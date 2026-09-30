// Package engine regenerates the world in the background, as the config or
// the map image change.
package engine

import (
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/Castux/wgen/internal/config"
	"github.com/Castux/wgen/internal/gen"
)

// Engine owns the current world and regenerates it in the background. Worlds
// are immutable, so readers just grab the current snapshot.
//
// Requests are coalesced: while a generation runs, only the latest requested
// config is kept, and generated next. A new request cancels the generation
// in progress, which is then redone with it. Long generations show a coarse
// preview first.
type Engine struct {
	mu sync.Mutex

	world   *gen.World     // last complete generation: the base of updates
	display *gen.World     // shown: world, or a preview of the generation in progress
	preview bool           // display is a preview
	conf    *config.Config // latest requested config, may not be generated yet
	version int            // of display
	err     error
	busy    bool
	dirty   bool // conf differs from the file

	pending request // not generated yet
	wake    chan struct{}

	// Image file known to hold the current map (saved there): moving the
	// project to it is not a new image
	savedImage string

	watchSteps int    // show the simulation every that many steps, 0: don't
	progress   string // of the simulation being watched

	subscribers map[chan State]struct{}
}

// Map is a map given in memory rather than by the image file: pixel
// colors, bottom row first.
type Map struct {
	Width, Height int
	Pixels        []config.Color
}

// State is what the app is told about the engine.
type State struct {
	Version  int    // incremented whenever the displayed world changes
	Busy     bool   // generating
	Preview  bool   // the displayed world is a preview, being refined
	Progress string // what the watched simulation is doing
	Error    string // last error, generating or loading the config
	Dirty    bool   // the config differs from its file
	Ready    bool   // a world was generated
	Stage    string // first stage rerun by the last generation
}

// request is what is asked of the engine: the requests that arrived since
// the last generation started, merged.
type request struct {
	conf       *config.Config // a new config
	image      bool           // reload the image file
	paintedMap *Map           // a map in memory
	fresh      bool           // don't start from the current world: another project
}

func (r request) empty() bool {
	return r.conf == nil && !r.image && r.paintedMap == nil && !r.fresh
}

func NewEngine() *Engine {
	e := &Engine{
		wake:        make(chan struct{}, 1),
		subscribers: map[chan State]struct{}{},
	}
	go e.loop()
	return e
}

// Snapshot returns the displayed world (nil if none was generated yet) and
// its version.
func (e *Engine) Snapshot() (*gen.World, int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.display, e.version
}

// Config returns the latest requested config (nil if none).
func (e *Engine) Config() *config.Config {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.conf
}

func (e *Engine) State() State {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.stateLocked("")
}

func (e *Engine) stateLocked(stage string) State {
	s := State{Version: e.version, Busy: e.busy, Preview: e.preview, Progress: e.progress, Dirty: e.dirty, Ready: e.display != nil, Stage: stage}
	if e.err != nil {
		s.Error = e.err.Error()
	}
	return s
}

// SetConfig requests a generation with a new config. fromFile tells whether it
// matches the config file (as opposed to an edit from the UI).
func (e *Engine) SetConfig(conf *config.Config, fromFile bool) {
	e.mu.Lock()
	e.conf = conf
	e.pending.conf = conf
	e.dirty = !fromFile
	e.mu.Unlock()
	e.signal()
}

// ReloadImage requests reloading the map image file.
func (e *Engine) ReloadImage() {
	e.mu.Lock()
	e.pending.image = true
	e.mu.Unlock()
	e.signal()
}

// SetMap requests a generation with a map given in memory, which
// replaces the image file's (until the file is reloaded).
func (e *Engine) SetMap(paintedMap *Map) {
	e.mu.Lock()
	e.pending.paintedMap = paintedMap
	e.pending.image = false
	e.mu.Unlock()
	e.signal()
}

// Replace requests generating another project from scratch: a config, and
// its map in memory (nil: its image file). The config is unsaved until
// MarkClean.
func (e *Engine) Replace(conf *config.Config, paintedMap *Map) {
	e.mu.Lock()
	e.conf = conf
	e.pending = request{conf: conf, paintedMap: paintedMap, fresh: true}
	e.dirty = true
	e.err = nil
	e.mu.Unlock()
	e.signal()
}

// SavedAs records that the project was saved: to conf's file, with the
// current map in conf's image file. Nothing is regenerated for the new
// paths.
func (e *Engine) SavedAs(conf *config.Config) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.savedImage = conf.ImagePath()
	e.conf = rebaseConfig(e.conf, conf)
	if e.pending.conf != nil {
		e.pending.conf = rebaseConfig(e.pending.conf, conf)
	}
	e.world = rebase(e.world, conf)
	if !e.preview {
		e.display = rebase(e.display, conf)
	}
	e.dirty = false
	e.publishLocked("")
}

// rebaseConfig returns a config with the paths of another.
func rebaseConfig(c, paths *config.Config) *config.Config {
	if c == nil {
		return nil
	}
	rebased := c.Clone()
	rebased.Path, rebased.Image = paths.Path, paths.Image
	return rebased
}

// rebase returns a world whose config has the paths of another.
func rebase(w *gen.World, paths *config.Config) *gen.World {
	if w == nil || w.Config == nil {
		return w
	}
	rebased := *w
	rebased.Config = rebaseConfig(w.Config, paths)
	return &rebased
}

// SetWatch chooses to show the simulation as it runs, every that many time
// steps (0: not).
func (e *Engine) SetWatch(steps int) {
	e.mu.Lock()
	e.watchSteps = steps
	e.mu.Unlock()
}

// SetError reports an error that happened outside of generation (such as
// failing to parse the config file), keeping the current world.
func (e *Engine) SetError(err error) {
	e.mu.Lock()
	e.err = err
	e.publishLocked("")
	e.mu.Unlock()
}

// MarkClean records that the current config was written to its file.
func (e *Engine) MarkClean() {
	e.mu.Lock()
	e.dirty = false
	e.publishLocked("")
	e.mu.Unlock()
}

func (e *Engine) signal() {
	select {
	case e.wake <- struct{}{}:
	default:
	}
}

func (e *Engine) loop() {
	for range e.wake {
		for e.step() {
		}
	}
}

// job is a request, with the state it applies to.
type job struct {
	request
	base       *gen.World     // the world to update, nil to generate from scratch
	latest     *config.Config // the latest requested config
	watchSteps int
}

// step runs one pending request, returns false if there was none.
func (e *Engine) step() bool {
	j, ok := e.takeRequest()
	if !ok {
		return false
	}
	next, stage, err := j.generate(e.options(j.watchSteps))
	e.finish(j.request, next, stage, err)
	return true
}

// takeRequest takes the pending request, if any, and marks the engine busy.
func (e *Engine) takeRequest() (job, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()

	j := job{request: e.pending, base: e.world, latest: e.conf, watchSteps: e.watchSteps}
	e.pending = request{}
	if j.empty() {
		return j, false
	}

	if j.fresh {
		j.base = nil
	}
	// Moved to where the map was saved: the same map
	if j.base != nil && j.latest != nil && j.base.Config.ImagePath() != j.latest.ImagePath() && j.latest.ImagePath() == e.savedImage {
		j.base = rebase(j.base, j.latest)
	}

	e.busy = true
	e.publishLocked("")
	return j, true
}

// Watched frames are shown for at least this long, so that fast phases can
// be seen
const minFrameTime = time.Second / 15

// options are the options of a generation: canceled by newer requests,
// showing its preview, and the simulation every watchSteps steps (0: not).
func (e *Engine) options(watchSteps int) gen.Options {
	options := gen.Options{
		// Newer requests supersede this one
		Canceled: func() bool {
			e.mu.Lock()
			defer e.mu.Unlock()
			return !e.pending.empty()
		},
		Preview: func(preview *gen.World) {
			e.mu.Lock()
			defer e.mu.Unlock()
			e.display, e.preview = preview, true
			e.version++
			e.publishLocked("")
		},
	}

	if watchSteps > 0 {
		var last time.Time
		options.WatchSteps = watchSteps
		options.Watch = func(watched *gen.World, progress string) {
			e.mu.Lock()
			e.display, e.preview, e.progress = watched, true, progress
			e.version++
			e.publishLocked("")
			e.mu.Unlock()

			if wait := minFrameTime - time.Since(last); wait > 0 {
				time.Sleep(wait)
			}
			last = time.Now()
		}
	}
	return options
}

// generate runs the job, and returns the new world and the first stage that
// was rerun.
func (j job) generate(options gen.Options) (next *gen.World, stage gen.Stage, err error) {
	stage = gen.StageNone

	switch {
	case j.latest == nil:
		err = errors.New("no config loaded")

	case j.paintedMap != nil:
		base := j.base
		if base == nil {
			base = &gen.World{}
		}
		next, stage, err = base.WithMap(j.latest, j.paintedMap.Width, j.paintedMap.Height, j.paintedMap.Pixels, options)

	case j.base == nil:
		// Nothing generated yet (or the first generation failed): full run
		stage = gen.StageImage
		next, _, err = (&gen.World{}).UpdateWith(j.latest, options)

	default:
		next = j.base
		if j.conf != nil {
			next, stage, err = next.UpdateWith(j.conf, options)
		}
		if err == nil && j.image {
			var imageStage gen.Stage
			next, imageStage, err = next.ReloadImageWith(options)
			stage = min(stage, imageStage)
		}
	}
	return next, stage, err
}

// finish records the result of a generation. A canceled one is requested
// again, merged with what superseded it.
func (e *Engine) finish(done request, next *gen.World, stage gen.Stage, err error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if errors.Is(err, gen.ErrCanceled) {
		if e.pending.conf == nil {
			e.pending.conf = done.conf
		}
		if e.pending.paintedMap == nil {
			e.pending.paintedMap = done.paintedMap
		}
		e.pending.image = e.pending.image || done.image
		e.pending.fresh = e.pending.fresh || done.fresh
		return
	}

	e.busy = false
	previewed := e.preview
	e.preview, e.progress = false, ""
	e.err = err
	if err != nil {
		slog.Error("generation failed", "err", err)
		if previewed {
			e.display = e.world
			e.version++
		}
		e.publishLocked("")
		return
	}

	e.world, e.display = next, next
	if stage < gen.StageNone || previewed {
		e.version++
	}
	e.publishLocked(stage.String())
}

// Subscribe returns a channel receiving state changes, starting with the
// current state. Slow subscribers miss intermediate states.
func (e *Engine) Subscribe() (<-chan State, func()) {
	ch := make(chan State, 4)

	e.mu.Lock()
	e.subscribers[ch] = struct{}{}
	ch <- e.stateLocked("")
	e.mu.Unlock()

	return ch, func() {
		e.mu.Lock()
		delete(e.subscribers, ch)
		e.mu.Unlock()
	}
}

func (e *Engine) publishLocked(stage string) {
	s := e.stateLocked(stage)
	for ch := range e.subscribers {
		select {
		case ch <- s:
		default:
			// Drop the oldest to make room for the latest
			select {
			case <-ch:
			default:
			}
			select {
			case ch <- s:
			default:
			}
		}
	}
}
