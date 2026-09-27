package engine

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"image"
	"image/color"
	"image/png"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/Castux/wgen/internal/config"
	"github.com/Castux/wgen/internal/export"
)

// Session ties an engine to a config file: it loads the file, reloads it and
// the outline image when they change, and applies the user's edits.
type Session struct {
	Engine *Engine

	watcher    *Watcher
	configPath string

	mu        sync.Mutex
	imagePath string   // currently watched
	written   [32]byte // hash of the last image written by SaveOutline
}

var errNoConfig = errors.New("no config loaded")

// Open loads a config file and starts generating. A broken config file is
// not an error: it is reported in the engine state, and loaded again when
// fixed.
func Open(configPath string) (*Session, error) {
	watcher, err := NewWatcher()
	if err != nil {
		return nil, err
	}

	s := &Session{
		Engine:     NewEngine(),
		watcher:    watcher,
		configPath: configPath,
	}

	if err := watcher.Watch(configPath, s.loadConfig); err != nil {
		watcher.Close()
		return nil, err
	}
	s.loadConfig()

	return s, nil
}

// Close stops watching the files.
func (s *Session) Close() error { return s.watcher.Close() }

// ConfigPath is the path of the config file, as given to Open.
func (s *Session) ConfigPath() string { return s.configPath }

// loadConfig (re)loads the config file. On failure, the current world is kept
// and the error reported in the engine state.
func (s *Session) loadConfig() {
	conf, warnings, err := config.Load(s.configPath)
	for _, w := range warnings {
		slog.Warn(w, "config", s.configPath)
	}
	if err != nil {
		slog.Error("could not load config", "err", err)
		s.Engine.SetError(err)
		return
	}

	slog.Info("loaded config", "path", s.configPath)
	s.watchImage(conf.Path)
	s.Engine.SetConfig(conf, true)
}

func (s *Session) watchImage(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if path == s.imagePath {
		return
	}
	if s.imagePath != "" {
		s.watcher.Unwatch(s.imagePath)
	}

	s.imagePath = path
	err := s.watcher.Watch(path, func() {
		// Not for our own writes
		data, err := os.ReadFile(path)
		s.mu.Lock()
		own := err == nil && sha256.Sum256(data) == s.written
		s.mu.Unlock()
		if own {
			return
		}

		slog.Info("outline image changed", "path", path)
		s.Engine.ReloadImage()
	})
	if err != nil {
		slog.Warn("cannot watch image", "path", path, "err", err)
	}
}

// Patch applies a partial config, in the config file format, and regenerates.
// The patched config is validated first: on error, nothing changes.
func (s *Session) Patch(patch []byte) error {
	conf := s.Engine.Config()
	if conf == nil {
		return errNoConfig
	}

	patched, err := conf.Patch(patch)
	if err != nil {
		return err
	}

	if patched.Path != conf.Path {
		s.watchImage(patched.Path)
	}

	s.Engine.SetConfig(patched, false)
	return nil
}

// Save writes the current config to its file, and returns the file path.
func (s *Session) Save() (string, error) {
	conf := s.Engine.Config()
	if conf == nil {
		return "", errNoConfig
	}

	if err := conf.Save(); err != nil {
		return "", err
	}

	slog.Info("saved config", "path", conf.ConfigPath)
	s.Engine.MarkSaved()
	return conf.ConfigPath, nil
}

// Export writes the exports enabled in the config of the current world, and
// returns the files written.
func (s *Session) Export() ([]string, error) {
	world, _ := s.Engine.Snapshot()
	if world == nil {
		return nil, errors.New("nothing generated yet")
	}
	return export.All(world)
}

// SetOutline regenerates with a map given in memory (the editor's), instead
// of the image file.
func (s *Session) SetOutline(o *Outline) {
	s.Engine.SetOutline(o)
}

// SaveOutline writes a map to the image file of the config, and returns the
// file path.
func (s *Session) SaveOutline(o *Outline) (string, error) {
	conf := s.Engine.Config()
	if conf == nil {
		return "", errNoConfig
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
		return "", err
	}

	s.mu.Lock()
	s.written = sha256.Sum256(buf.Bytes())
	s.mu.Unlock()

	path := conf.Path
	if dir := filepath.Dir(path); dir != "" {
		os.MkdirAll(dir, 0o755)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		return "", err
	}
	slog.Info("saved map", "path", path)
	return path, nil
}
