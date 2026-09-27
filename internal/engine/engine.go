// Package engine regenerates the world in the background, as the config or
// the outline image change.
package engine

import (
	"errors"
	"log/slog"
	"sync"

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
	wake           chan struct{}

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
	Version int    // incremented whenever the displayed world changes
	Busy    bool   // generating
	Preview bool   // the displayed world is a preview, being refined
	Error   string // last error, generating or loading the config
	Dirty   bool   // the config differs from its file
	Ready   bool   // a world was generated
	Stage   string // first stage rerun by the last generation
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
	s := State{Version: e.version, Busy: e.busy, Preview: e.preview, Dirty: e.dirty, Ready: e.display != nil, Stage: stage}
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
	return e.pendingConf != nil || e.pendingImage || e.pendingOutline != nil
}

// step runs one pending request, returns false if there was none.
func (e *Engine) step() bool {
	e.mu.Lock()
	conf, image, outline := e.pendingConf, e.pendingImage, e.pendingOutline
	e.pendingConf, e.pendingImage, e.pendingOutline = nil, false, nil
	world := e.world
	latest := e.conf

	if conf == nil && !image && outline == nil {
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
		return true
	}

	e.busy = false
	previewed := e.preview
	e.preview = false
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
