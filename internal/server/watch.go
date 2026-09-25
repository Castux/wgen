package server

import (
	"log/slog"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Watcher calls a function when one of the watched files changes. It watches
// directories rather than files, so that editors replacing files on save
// (write to a temporary file, then rename) are handled. Events are debounced.
type Watcher struct {
	fs       *fsnotify.Watcher
	mu       sync.Mutex
	files    map[string]func() // absolute path -> callback
	dirs     map[string]bool
	timers   map[string]*time.Timer
	debounce time.Duration
}

func NewWatcher() (*Watcher, error) {
	fs, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	w := &Watcher{
		fs:       fs,
		files:    map[string]func(){},
		dirs:     map[string]bool{},
		timers:   map[string]*time.Timer{},
		debounce: 150 * time.Millisecond,
	}
	go w.loop()
	return w, nil
}

// Watch calls f when path changes, replacing any previous callback for path.
func (w *Watcher) Watch(path string, f func()) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	w.files[abs] = f
	dir := filepath.Dir(abs)
	if !w.dirs[dir] {
		if err := w.fs.Add(dir); err != nil {
			return err
		}
		w.dirs[dir] = true
	}
	return nil
}

// Unwatch stops watching path. Its directory stays watched.
func (w *Watcher) Unwatch(path string) {
	abs, _ := filepath.Abs(path)
	w.mu.Lock()
	delete(w.files, abs)
	w.mu.Unlock()
}

func (w *Watcher) loop() {
	for {
		select {
		case event, ok := <-w.fs.Events:
			if !ok {
				return
			}
			if !event.Has(fsnotify.Write) && !event.Has(fsnotify.Create) && !event.Has(fsnotify.Rename) {
				continue
			}

			path := filepath.Clean(event.Name)
			w.mu.Lock()
			if f, ok := w.files[path]; ok {
				if t := w.timers[path]; t != nil {
					t.Stop()
				}
				w.timers[path] = time.AfterFunc(w.debounce, f)
			}
			w.mu.Unlock()

		case err, ok := <-w.fs.Errors:
			if !ok {
				return
			}
			slog.Warn("file watcher error", "err", err)
		}
	}
}

func (w *Watcher) Close() error { return w.fs.Close() }
