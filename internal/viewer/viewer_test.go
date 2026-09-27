package viewer

import (
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Castux/wgen/internal/config"
	"github.com/Castux/wgen/internal/gen"
	"github.com/Castux/wgen/internal/render"
)

func TestSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wgen", "viewer.json")

	if s := loadSettings(path); s != defaultSettings {
		t.Errorf("without file: %+v", s)
	}

	s := defaultSettings
	s.View, s.Wireframe, s.Contours = "map", true, 20
	saveSettings(path, s)
	if loaded := loadSettings(path); loaded != s {
		t.Errorf("loaded %+v, saved %+v", loaded, s)
	}

	// Invalid values get their default, the others are kept
	os.WriteFile(path, []byte(`{"view": "sideways", "grid": 50}`), 0o644)
	if loaded := loadSettings(path); loaded.View != "orbit" || loaded.Grid != 50 {
		t.Errorf("loaded %+v", loaded)
	}

	os.WriteFile(path, []byte(`{"view": `), 0o644)
	if loaded := loadSettings(path); loaded != defaultSettings {
		t.Errorf("broken file: %+v", loaded)
	}

	if cycle(views, "map") != "orbit" || cycle(views, "orbit") != "top" {
		t.Error("cycle")
	}
}

func TestRoundToStep(t *testing.T) {
	for _, test := range []struct{ v, step, want float64 }{
		{0.123456, 0.001, 0.123},
		{-0.0996, 0.001, -0.1},
		{31.74, 0.5, 31.5},
		{0.7, 0.1, 0.7},
		{17, 10, 20},
		{3.3, 1, 3},
	} {
		if got := roundToStep(test.v, test.step); got != test.want {
			t.Errorf("roundToStep(%v, %v) = %v, expected %v", test.v, test.step, got, test.want)
		}
	}

	if stepFormat(0.01) != "%.2f" || stepFormat(1) != "%.0f" || stepFormat(0.5) != "%.1f" {
		t.Error("stepFormat")
	}
}

func island(t *testing.T) *gen.World {
	t.Chdir(t.TempDir())

	img := image.NewNRGBA(image.Rect(0, 0, 60, 40))
	for y := range 40 {
		for x := range 60 {
			c := color.NRGBA{66, 66, 125, 255}
			if math.Hypot(float64(x-30), float64(y-20)) < 15 {
				c = color.NRGBA{135, 168, 81, 255}
			}
			img.Set(x, y, c)
		}
	}
	f, _ := os.Create("island.png")
	png.Encode(f, img)
	f.Close()

	conf, _, err := config.Parse([]byte(`{
		"path": "island.png",
		"resolution": 4, "grid": "hex", "jitter": 0.5, "relax": false,
		"smoothingRadius": 0, "erosionMinFlow": 5, "erosionFactor": 0.5,
		"terrains": {
			"sea": { "r": 66, "g": 66, "b": 125, "gradient": -0.1, "fixedShore": 0.0 },
			"land": { "r": 135, "g": 168, "b": 81, "gradient": 0.5 }
		}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	w, err := gen.New(conf)
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func TestImageSlot(t *testing.T) {
	w := island(t)

	var mu sync.Mutex
	wakes := 0
	s := imageSlot{wake: func() { mu.Lock(); wakes++; mu.Unlock() }}

	wait := func() *image.RGBA {
		t.Helper()
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			if img := s.take(); img != nil {
				return img
			}
			time.Sleep(time.Millisecond)
		}
		t.Fatal("no image rendered")
		return nil
	}

	// Only the latest of several requests is delivered
	for scale := 1.0; scale <= 4; scale++ {
		s.request(w, 1, render.Options{Scale: scale, Base: render.BaseTerrain})
	}
	if img := wait(); img.Rect.Dx() != 240 {
		t.Errorf("image width %d, expected the latest request's", img.Rect.Dx())
	}
	if s.busy() || s.take() != nil {
		t.Error("slot still busy, or result delivered twice")
	}

	// The same request again does nothing
	s.request(w, 1, render.Options{Scale: 4, Base: render.BaseTerrain})
	if s.busy() {
		t.Error("rendering the same image again")
	}

	// A new version does
	s.request(w, 2, render.Options{Scale: 4, Base: render.BaseTerrain})
	wait()

	mu.Lock()
	defer mu.Unlock()
	if wakes != 2 {
		t.Errorf("%d wake ups, expected 2", wakes)
	}
}
