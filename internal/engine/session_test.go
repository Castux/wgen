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

func TestOutline(t *testing.T) {
	h := setup(t)
	world, _ := h.s.Engine.Snapshot()
	v0 := h.s.Engine.State().Version

	// Paint the island away: all sea but a corner
	o := &Outline{Width: world.Width, Height: world.Height, Pixels: slices.Clone(world.Outline)}
	sea, land := o.Pixels[0], o.Pixels[40*o.Width+50]
	for i := range o.Pixels {
		o.Pixels[i] = sea
	}
	for y := range 20 {
		for x := range 20 {
			o.Pixels[y*o.Width+x] = land
		}
	}

	h.s.SetOutline(o)
	h.waitFor(func(st State) bool { return st.Version > v0 && !st.Busy })
	painted, v1 := h.s.Engine.Snapshot()
	if painted.Outline[40*o.Width+50] != sea || painted.Highest >= world.Highest {
		t.Errorf("painted outline not generated: highest %v, was %v", painted.Highest, world.Highest)
	}

	// Saved to the image file, which reloads as is: nothing regenerated
	path, err := h.s.SaveOutline(o)
	if err != nil || path != "island.png" {
		t.Fatalf("save: %q %v", path, err)
	}
	time.Sleep(500 * time.Millisecond)
	if st := h.s.Engine.State(); st.Version != v1 || st.Busy {
		t.Errorf("own save reloaded: %+v", st)
	}

	// The saved file is the map
	f, _ := os.Open("island.png")
	img, err := png.Decode(f)
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	if r, g, b, _ := img.At(5, world.Height-1-5).RGBA(); [3]uint8{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8)} != [3]uint8(land) {
		t.Errorf("saved image: land corner is %v", img.At(5, world.Height-6))
	}
}

func TestPreviewAndSupersede(t *testing.T) {
	h := setup(t)
	if err := h.s.Patch([]byte(`{"elevationModel": "uplift", "mapWidth": 50, "resolution": 1, "levels": 2,
		"terrains": {"land": {"height": 1000}}}`)); err != nil {
		t.Fatal(err)
	}
	// Right away, superseding it
	if err := h.s.Patch([]byte(`{"terrains": {"land": {"height": 2000}}}`)); err != nil {
		t.Fatal(err)
	}

	previewed := false
	h.waitFor(func(st State) bool {
		previewed = previewed || st.Preview
		world, _ := h.s.Engine.Snapshot()
		return !st.Busy && !st.Preview && world.Conf.Terrain("land").Height == 2000
	})
	if !previewed {
		t.Error("no preview shown")
	}
	if st := h.s.Engine.State(); st.Error != "" {
		t.Errorf("error: %s", st.Error)
	}
}
