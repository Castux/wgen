package engine

import (
	"errors"
	"log/slog"
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
	imagePath string // currently watched
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
