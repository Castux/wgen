package server

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
// config is kept, and generated next.
type Engine struct {
	mu sync.Mutex

	world   *gen.World
	conf    *config.Config // latest requested config, may not be generated yet
	version int
	err     error
	busy    bool
	dirty   bool // conf differs from the file

	pendingConf  *config.Config
	pendingImage bool
	wake         chan struct{}

	subscribers map[chan State]struct{}
}

// State is what clients are told about the engine.
type State struct {
	Version int    `json:"version"`
	Busy    bool   `json:"busy"`
	Error   string `json:"error,omitempty"`
	Dirty   bool   `json:"dirty"`
	Ready   bool   `json:"ready"`
	Stage   string `json:"stage,omitempty"`
}

func NewEngine() *Engine {
	e := &Engine{
		wake:        make(chan struct{}, 1),
		subscribers: map[chan State]struct{}{},
	}
	go e.loop()
	return e
}

// Snapshot returns the current world (nil if none was generated yet) and its
// version.
func (e *Engine) Snapshot() (*gen.World, int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.world, e.version
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
	s := State{Version: e.version, Busy: e.busy, Dirty: e.dirty, Ready: e.world != nil, Stage: stage}
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

// step runs one pending request, returns false if there was none.
func (e *Engine) step() bool {
	e.mu.Lock()
	conf, image := e.pendingConf, e.pendingImage
	e.pendingConf, e.pendingImage = nil, false
	world := e.world

	if conf == nil && !image {
		e.mu.Unlock()
		return false
	}

	e.busy = true
	e.publishLocked("")
	e.mu.Unlock()

	var next *gen.World
	var err error
	stage := gen.StageNone

	switch {
	case world == nil:
		// Nothing generated yet (or the first generation failed): full run
		if conf == nil {
			conf = e.Config()
		}
		if conf == nil {
			err = errors.New("no config loaded")
			break
		}
		stage = gen.StageImage
		next, err = gen.New(conf)

	default:
		next = world
		if conf != nil {
			next, stage, err = next.Update(conf)
		}
		if err == nil && image {
			var imageStage gen.Stage
			next, imageStage, err = next.ReloadImage()
			stage = min(stage, imageStage)
		}
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	e.busy = false
	e.err = err
	if err != nil {
		slog.Error("generation failed", "err", err)
		e.publishLocked("")
		return true
	}

	e.world = next
	if stage < gen.StageNone {
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
