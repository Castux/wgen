package engine

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"slices"
	"strings"
	"testing"
	"time"
)

const testConfig = `{
	"path": "island.png",
	"resolution": 4, "grid": "hex", "jitter": 0.5, "relax": false,
	"smoothingRadius": 0, "erosionMinFlow": 5, "erosionFactor": 0.5,
	"terrains": {
		"sea": { "r": 66, "g": 66, "b": 125, "gradient": -0.1, "fixedShore": 0.0 },
		"land": { "r": 135, "g": 168, "b": 81, "gradient": 0.5 }
	}
}`

func writeIsland(t *testing.T, radius float64) {
	img := image.NewNRGBA(image.Rect(0, 0, 100, 80))
	for y := range 80 {
		for x := range 100 {
			c := color.NRGBA{66, 66, 125, 255}
			if math.Hypot(float64(x-50), float64(y-40)) < radius {
				c = color.NRGBA{135, 168, 81, 255}
			}
			img.Set(x, y, c)
		}
	}

	var buf bytes.Buffer
	png.Encode(&buf, img)
	if err := os.WriteFile("island.png", buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

type harness struct {
	t      *testing.T
	s      *Session
	states <-chan State
}

func setup(t *testing.T) *harness {
	t.Chdir(t.TempDir())
	writeIsland(t, 30)
	os.WriteFile("config.json", []byte(testConfig), 0o644)

	s, err := Open("config.json")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })

	states, cancel := s.Engine.Subscribe()
	t.Cleanup(cancel)

	h := &harness{t: t, s: s, states: states}
	h.waitFor(func(st State) bool { return st.Ready && !st.Busy })
	return h
}

// waitFor waits for a state matching f.
func (h *harness) waitFor(f func(State) bool) State {
	h.t.Helper()
	timeout := time.After(10 * time.Second)
	for {
		select {
		case st := <-h.states:
			if f(st) {
				return st
			}
		case <-timeout:
			h.t.Fatalf("timeout, state %+v", h.s.Engine.State())
		}
	}
}

func TestPatchSaveExport(t *testing.T) {
	h := setup(t)
	v0 := h.s.Engine.State().Version

	if err := h.s.Patch([]byte(`{"resolution": -1}`)); err == nil {
		t.Error("invalid patch accepted")
	}

	if err := h.s.Patch([]byte(`{"erosionFactor": 0.2}`)); err != nil {
		t.Fatal(err)
	}
	st := h.waitFor(func(st State) bool { return st.Version > v0 && !st.Busy })
	if !st.Dirty || st.Stage != "erosion" {
		t.Errorf("after patch: %+v", st)
	}

	// Save writes the file, which reloads as is: nothing regenerated
	if _, err := h.s.Save(); err != nil {
		t.Fatal(err)
	}
	saved, _ := os.ReadFile("config.json")
	if !bytes.Contains(saved, []byte(`"erosionFactor": 0.2`)) {
		t.Errorf("saved config: %s", saved)
	}

	time.Sleep(500 * time.Millisecond)
	if st := h.s.Engine.State(); st.Dirty || st.Version != v0+1 {
		t.Errorf("after save: %+v", st)
	}

	// Nothing enabled but heightmaps, by default
	files, err := h.s.Export()
	if err != nil || !slices.Contains(files, "config.json-w.png") {
		t.Errorf("export: %v %v", files, err)
	}
}

func TestHotReload(t *testing.T) {
	h := setup(t)
	v0 := h.s.Engine.State().Version

	// Broken config: error reported, world kept
	os.WriteFile("config.json", []byte(`{"path": `), 0o644)
	st := h.waitFor(func(st State) bool { return st.Error != "" })
	if !st.Ready || st.Version != v0 {
		t.Errorf("after broken config: %+v", st)
	}

	// Fixed config: regenerated, error cleared
	os.WriteFile("config.json", []byte(strings.Replace(testConfig, `"resolution": 4`, `"resolution": 5`, 1)), 0o644)
	st = h.waitFor(func(st State) bool { return st.Version > v0 && !st.Busy })
	if st.Error != "" || st.Stage != "mesh" {
		t.Errorf("after fixed config: %+v", st)
	}

	// New image, same size: from terrain
	writeIsland(t, 20)
	st = h.waitFor(func(st State) bool { return st.Version > v0+1 && !st.Busy })
	if st.Stage != "terrain" {
		t.Errorf("after image change: %+v", st)
	}
}
