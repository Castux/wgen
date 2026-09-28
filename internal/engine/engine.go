// Package engine regenerates the world in the background, as the config or
// the outline image change.
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

	pendingConf    *config.Config
	pendingImage   bool
	pendingOutline *Outline
	pendingRerun   bool
	pendingFresh   bool // don't start from the current world: another project
	wake           chan struct{}

	// Image file known to hold the current map (saved there): moving the
	// project to it is not a new image
	savedImage string

	watchSteps int    // show the simulation every that many steps, 0: don't
	progress   string // of the simulation being watched

	subscribers map[chan State]struct{}
}

// Outline is a map given in memory rather than by the image file: pixel
// colors, bottom row first.
type Outline struct {
	Width, Height int
	Pixels        []config.Color
}

// State is what the viewer is told about the engine.
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
	e.pendingConf = conf
	e.dirty = !fromFile
	e.mu.Unlock()
	e.signal()
}

// ReloadImage requests reloading the outline image.
func (e *Engine) ReloadImage() {
	e.mu.Lock()
	e.pendingImage = true
	e.mu.Unlock()
	e.signal()
}

// SetOutline requests a generation with a map given in memory, which
// replaces the image file's (until the file is reloaded).
func (e *Engine) SetOutline(o *Outline) {
	e.mu.Lock()
	e.pendingOutline = o
	e.pendingImage = false
	e.mu.Unlock()
	e.signal()
}

// Replace requests generating another project from scratch: a config, and
// its map in memory (nil: its image file). The config is unsaved until
// MarkSaved.
func (e *Engine) Replace(conf *config.Config, o *Outline) {
	e.mu.Lock()
	e.conf = conf
	e.pendingConf = conf
	e.pendingOutline = o
	e.pendingImage, e.pendingRerun = false, false
	e.pendingFresh = true
	e.dirty = true
	e.err = nil
	e.mu.Unlock()
	e.signal()
}

// SetSaved records that the project was saved: to conf's file, with the
// current map in conf's image file. Nothing is regenerated for the new
// paths.
func (e *Engine) SetSaved(conf *config.Config) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.savedImage = conf.ImagePath()
	e.conf = rebaseConfig(e.conf, conf)
	if e.pendingConf != nil {
		e.pendingConf = rebaseConfig(e.pendingConf, conf)
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
	n := c.Clone()
	n.ConfigPath, n.Image = paths.ConfigPath, paths.Image
	return n
}

// rebase returns a world whose config has the paths of another.
func rebase(w *gen.World, paths *config.Config) *gen.World {
	if w == nil || w.Conf == nil {
		return w
	}
	n := *w
	n.Conf = rebaseConfig(w.Conf, paths)
	return &n
}

// SetWatch chooses to show the simulation as it runs, every that many time
// steps (0: not).
func (e *Engine) SetWatch(steps int) {
	e.mu.Lock()
	e.watchSteps = steps
	e.mu.Unlock()
}

// Rerun requests generating again with the same config, to watch the
// simulation.
func (e *Engine) Rerun() {
	e.mu.Lock()
	e.pendingRerun = true
	e.mu.Unlock()
	e.signal()
}

// SetError reports an error that happened outside of generation (such as
// failing to parse the config file), keeping the current world.
func (e *Engine) SetError(err error) {
	e.mu.Lock()
	e.err = err
	e.publishLocked("")
	e.mu.Unlock()
}

// MarkSaved records that the current config was written to its file.
func (e *Engine) MarkSaved() {
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

// pendingLocked tells whether a request is waiting.
func (e *Engine) pendingLocked() bool {
	return e.pendingConf != nil || e.pendingImage || e.pendingOutline != nil || e.pendingRerun || e.pendingFresh
}

// Watched frames are shown for at least this long, so that fast phases can
// be seen
const minFrameTime = time.Second / 15

// step runs one pending request, returns false if there was none.
func (e *Engine) step() bool {
	e.mu.Lock()
	conf, image, outline, rerun, fresh := e.pendingConf, e.pendingImage, e.pendingOutline, e.pendingRerun, e.pendingFresh
	e.pendingConf, e.pendingImage, e.pendingOutline, e.pendingRerun, e.pendingFresh = nil, false, nil, false, false
	world := e.world
	latest := e.conf
	watchSteps := e.watchSteps
	if fresh {
		world = nil
	}
	// Moved to where the map was saved: the same map
	if world != nil && latest != nil && world.Conf.ImagePath() != latest.ImagePath() && latest.ImagePath() == e.savedImage {
		world = rebase(world, latest)
	}

	if conf == nil && !image && outline == nil && !rerun && !fresh {
		e.mu.Unlock()
		return false
	}

	e.busy = true
	e.publishLocked("")
	e.mu.Unlock()

	opts := gen.Options{
		// Newer requests supersede this one
		Canceled: func() bool {
			e.mu.Lock()
			defer e.mu.Unlock()
			return e.pendingLocked()
		},
		Preview: func(p *gen.World) {
			e.mu.Lock()
			defer e.mu.Unlock()
			e.display, e.preview = p, true
			e.version++
			e.publishLocked("")
		},
	}

	if watchSteps > 0 {
		var last time.Time
		opts.WatchSteps = watchSteps
		opts.Watch = func(p *gen.World, progress string) {
			e.mu.Lock()
			e.display, e.preview, e.progress = p, true, progress
			e.version++
			e.publishLocked("")
			e.mu.Unlock()

			if wait := minFrameTime - time.Since(last); wait > 0 {
				time.Sleep(wait)
			}
			last = time.Now()
		}
	}

	var next *gen.World
	var err error
	stage := gen.StageNone

	switch {
	case latest == nil:
		err = errors.New("no config loaded")

	case outline != nil:
		base := world
		if base == nil {
			base = &gen.World{}
		}
		next, stage, err = base.WithOutline(latest, outline.Width, outline.Height, outline.Pixels, opts)

	case rerun && conf == nil && !image && world != nil:
		next, stage, err = world.Rerun(opts)

	case world == nil:
		// Nothing generated yet (or the first generation failed): full run
		stage = gen.StageImage
		next, _, err = (&gen.World{}).UpdateWith(latest, opts)

	default:
		next = world
		if conf != nil {
			next, stage, err = next.UpdateWith(conf, opts)
		}
		if err == nil && image {
			var imageStage gen.Stage
			next, imageStage, err = next.ReloadImageWith(opts)
			stage = min(stage, imageStage)
		}
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	if errors.Is(err, gen.ErrCanceled) {
		// Redone with what superseded it
		if e.pendingConf == nil && conf != nil {
			e.pendingConf = conf
		}
		if e.pendingOutline == nil && outline != nil {
			e.pendingOutline = outline
		}
		e.pendingImage = e.pendingImage || image
		e.pendingRerun = e.pendingRerun || (rerun && !e.pendingLocked())
		e.pendingFresh = e.pendingFresh || fresh
		return true
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
		return true
	}

	e.world, e.display = next, next
	if stage < gen.StageNone || previewed {
		e.version++
	}
	e.publishLocked(stage.String())
	return true
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
