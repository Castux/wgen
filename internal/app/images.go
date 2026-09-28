package app

import (
	"image"
	"log/slog"
	"sync"
	"time"

	"github.com/Castux/wgen/internal/gen"
	"github.com/Castux/wgen/internal/render"
)

// imageRequest is an image to render: a world version with render options.
type imageRequest struct {
	version int
	options render.Options
}

// imageSlot renders images of the world in the background, one at a time.
// Only the latest request matters: requests arriving while rendering replace
// each other, and results that are not the latest request are dropped.
type imageSlot struct {
	wake func() // called when a result is ready, from any goroutine

	mu      sync.Mutex
	latest  imageRequest // latest request, zero if none
	world   *gen.World   // of the latest request
	running bool
	result  *image.RGBA // ready, of the latest request
}

// request asks for an image, unless it is already the latest request.
func (s *imageSlot) request(world *gen.World, version int, options render.Options) {
	req := imageRequest{version, options}

	s.mu.Lock()
	defer s.mu.Unlock()

	if req == s.latest && s.world != nil {
		return
	}
	s.latest, s.world, s.result = req, world, nil
	if !s.running {
		s.running = true
		go s.run()
	}
}

func (s *imageSlot) run() {
	for {
		s.mu.Lock()
		req, world := s.latest, s.world
		s.mu.Unlock()

		start := time.Now()
		img := render.Render(world, req.options)
		slog.Debug("rendered", "options", req.options, "took", time.Since(start).Round(time.Millisecond))

		s.mu.Lock()
		if req == s.latest {
			s.result, s.running = img, false
			s.mu.Unlock()
			s.wake()
			return
		}
		s.mu.Unlock()
	}
}

// take returns the latest result, once.
func (s *imageSlot) take() *image.RGBA {
	s.mu.Lock()
	defer s.mu.Unlock()
	img := s.result
	s.result = nil
	return img
}

// busy tells whether an image is being rendered.
func (s *imageSlot) busy() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}
