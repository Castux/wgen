package engine

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Castux/wgen/internal/config"
	"github.com/Castux/wgen/internal/export"
)

// Session is the project being worked on: a config (with its file, if saved)
// and its map. It loads and saves projects, reloads the project file and the
// map image when they change on disk, and applies the user's edits.
type Session struct {
	Engine *Engine

	watcher *Watcher

	mu         sync.Mutex
	configPath string              // watched, "" if not saved yet
	imagePath  string              // watched
	written    map[string][32]byte // hash of what the session wrote last, per file
}

var errNoConfig = errors.New("no project loaded")

// NewSession returns a session without a project.
func NewSession() (*Session, error) {
	watcher, err := NewWatcher()
	if err != nil {
		return nil, err
	}
	return &Session{Engine: NewEngine(), watcher: watcher, written: map[string][32]byte{}}, nil
}

// Open returns a session with a project file loaded. A broken project file
// is not an error: it is reported in the engine state, and loaded again
// when fixed.
func Open(configPath string) (*Session, error) {
	s, err := NewSession()
	if err != nil {
		return nil, err
	}
	if err := s.Load(configPath); err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}

// Close stops watching the files.
func (s *Session) Close() error { return s.watcher.Close() }

// ConfigPath is the path of the project file, "" if not saved yet.
func (s *Session) ConfigPath() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.configPath
}

// Load switches to a project file, and generates it.
func (s *Session) Load(configPath string) error {
	if _, err := os.Stat(configPath); err != nil {
		return err
	}
	s.watchConfig(configPath)
	s.loadConfig(true)
	return nil
}

// New switches to a new project, not saved yet: a config and its map.
func (s *Session) New(conf *config.Config, outline *Outline) {
	conf = conf.Clone()
	conf.ConfigPath = ""
	s.watchConfig("")
	s.watchImage("")
	s.Engine.Replace(conf, outline)
}

func (s *Session) watchConfig(path string) {
	s.mu.Lock()
	old := s.configPath
	s.configPath = path
	s.mu.Unlock()

	if old != "" && old != path {
		s.watcher.Unwatch(old)
	}
	if path != "" && path != old {
		if err := s.watcher.Watch(path, func() {
			if !s.ownWrite(path) {
				s.loadConfig(false)
			}
		}); err != nil {
			slog.Warn("cannot watch project file", "path", path, "err", err)
		}
	}
}

// loadConfig (re)loads the project file. On failure, the current world is
// kept and the error reported in the engine state. fresh starts from
// nothing: another project was shown.
func (s *Session) loadConfig(fresh bool) {
	path := s.ConfigPath()
	if path == "" {
		return
	}
	conf, warnings, err := config.Load(path)
	for _, w := range warnings {
		slog.Warn(w, "project", path)
	}
	if err != nil {
		slog.Error("could not load project", "err", err)
		s.Engine.SetError(err)
		return
	}

	slog.Info("loaded project", "path", path)
	s.watchImage(conf.ImagePath())
	if fresh {
		s.Engine.Replace(conf, nil)
		s.Engine.MarkSaved()
	} else {
		s.Engine.SetConfig(conf, true)
	}
}

func (s *Session) watchImage(path string) {
	s.mu.Lock()
	old := s.imagePath
	s.imagePath = path
	s.mu.Unlock()

	if path == old {
		return
	}
	if old != "" {
		s.watcher.Unwatch(old)
	}
	if path == "" {
		return
	}
	err := s.watcher.Watch(path, func() {
		if s.ownWrite(path) {
			return
		}
		slog.Info("map image changed", "path", path)
		s.Engine.ReloadImage()
	})
	if err != nil {
		slog.Warn("cannot watch image", "path", path, "err", err)
	}
}

// ownWrite tells whether a file is as the session last wrote it: changes
// seen by the watcher that are only our own saves.
func (s *Session) ownWrite(path string) bool {
	data, err := os.ReadFile(path)
	s.mu.Lock()
	defer s.mu.Unlock()
	hash, ok := s.written[path]
	return err == nil && ok && sha256.Sum256(data) == hash
}

// write writes a file, remembering it as our own.
func (s *Session) write(path string, data []byte) error {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	s.mu.Lock()
	s.written[path] = sha256.Sum256(data)
	s.mu.Unlock()
	return os.WriteFile(path, data, 0o644)
}

// Patch applies a partial config, in the project file format, and
// regenerates. The patched config is validated first: on error, nothing
// changes.
func (s *Session) Patch(patch []byte) error {
	conf := s.Engine.Config()
	if conf == nil {
		return errNoConfig
	}
	patched, err := conf.Patch(patch)
	if err != nil {
		return err
	}
	return s.SetConfig(patched)
}

// SetConfig replaces the config (such as edited terrains), and regenerates.
func (s *Session) SetConfig(conf *config.Config) error {
	if err := conf.Validate(); err != nil {
		return err
	}
	if s.ConfigPath() != "" {
		s.watchImage(conf.ImagePath())
	}
	s.Engine.SetConfig(conf, false)
	return nil
}

// SetOutline regenerates with a map given in memory (the editor's), instead
// of the image file.
func (s *Session) SetOutline(outline *Outline) {
	s.Engine.SetOutline(outline)
}

// Save writes the project file and its map image, and returns the project
// file path. The project must have a file already (see SaveAs).
func (s *Session) Save(outline *Outline) (string, error) {
	path := s.ConfigPath()
	if path == "" {
		return "", errors.New("the project has no file yet: save it as")
	}
	return path, s.SaveAs(path, outline)
}

// SaveAs writes the project to a file, and its map image next to it (keeping
// its name, or <project>.png for new projects), and makes it the project's
// file.
func (s *Session) SaveAs(configPath string, outline *Outline) error {
	conf := s.Engine.Config()
	if conf == nil {
		return errNoConfig
	}
	conf = conf.Clone()

	if !strings.EqualFold(filepath.Ext(configPath), ".json") {
		configPath += ".json"
	}
	if conf.Image == "" || s.ConfigPath() != configPath {
		conf.Image = strings.TrimSuffix(filepath.Base(configPath), filepath.Ext(configPath)) + ".png"
	}
	conf.ConfigPath = configPath

	data, err := EncodeOutline(outline)
	if err != nil {
		return err
	}
	if err := s.write(conf.ImagePath(), data); err != nil {
		return err
	}
	if err := s.write(configPath, conf.Marshal()); err != nil {
		return err
	}

	s.watchConfig(configPath)
	s.watchImage(conf.ImagePath())
	s.Engine.SetSaved(conf)
	slog.Info("saved project", "path", configPath, "image", conf.ImagePath())
	return nil
}

// EncodeOutline encodes a map as a PNG image.
func EncodeOutline(o *Outline) ([]byte, error) {
	if o == nil || len(o.Pixels) != o.Width*o.Height {
		return nil, fmt.Errorf("no map to save")
	}
	img := image.NewNRGBA(image.Rect(0, 0, o.Width, o.Height))
	for y := range o.Height {
		row := o.Pixels[(o.Height-1-y)*o.Width:] // bottom row first
		for x := range o.Width {
			c := row[x]
			img.SetNRGBA(x, y, color.NRGBA{c[0], c[1], c[2], 255})
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Export writes the chosen outputs of the current world, named from base (a
// path without extension), and returns the files written.
func (s *Session) Export(base string, options export.Options) ([]string, error) {
	world, _ := s.Engine.Snapshot()
	if world == nil {
		return nil, errors.New("nothing generated yet")
	}
	return export.All(world, base, options)
}
