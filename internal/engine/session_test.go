package engine

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Castux/wgen/internal/config"
	"github.com/Castux/wgen/internal/export"
)

const testConfig = `{
	"image": "island.png",
	"mapWidth": 50, "resolution": 1000, "levels": 1,
	"terrains": {
		"sea": { "color": "#42427d" },
		"land": { "color": "#87a851", "height": 500 }
	},
	"simulation": { "steps": 60, "refineSteps": 10 }
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

func newHarness(t *testing.T, s *Session) *harness {
	t.Cleanup(func() { s.Close() })
	states, cancel := s.Engine.Subscribe()
	t.Cleanup(cancel)
	return &harness{t: t, s: s, states: states}
}

func setup(t *testing.T) *harness {
	t.Chdir(t.TempDir())
	writeIsland(t, 30)
	os.WriteFile("config.json", []byte(testConfig), 0o644)

	s, err := Open("config.json")
	if err != nil {
		t.Fatal(err)
	}
	h := newHarness(t, s)
	h.waitFor(func(st State) bool { return st.Ready && !st.Busy })
	return h
}

// waitFor waits for a state matching f.
func (h *harness) waitFor(f func(State) bool) State {
	h.t.Helper()
	timeout := time.After(20 * time.Second)
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

// paintedMap returns the map of the displayed world.
func (h *harness) paintedMap() *Map {
	w, _ := h.s.Engine.Snapshot()
	return &Map{Width: w.Width, Height: w.Height, Pixels: slices.Clone(w.Map)}
}

func TestPatchSaveExport(t *testing.T) {
	h := setup(t)
	if st := h.s.Engine.State(); st.Dirty {
		t.Errorf("loaded project dirty: %+v", st)
	}
	v0 := h.s.Engine.State().Version

	if err := h.s.Patch([]byte(`{"resolution": -1}`)); err == nil {
		t.Error("invalid patch accepted")
	}

	if err := h.s.Patch([]byte(`{"simulation": {"criticalSlope": 25}}`)); err != nil {
		t.Fatal(err)
	}
	st := h.waitFor(func(st State) bool { return st.Version > v0 && !st.Busy })
	if !st.Dirty || st.Stage != "terrain" {
		t.Errorf("after patch: %+v", st)
	}

	// Save writes the files, which the watcher ignores: nothing regenerated
	v1 := st.Version
	if _, err := h.s.Save(h.paintedMap()); err != nil {
		t.Fatal(err)
	}
	saved, _ := os.ReadFile("config.json")
	if !bytes.Contains(saved, []byte(`"criticalSlope": 25`)) || !bytes.Contains(saved, []byte(`"image": "island.png"`)) {
		t.Errorf("saved project: %s", saved)
	}
	time.Sleep(500 * time.Millisecond)
	if st := h.s.Engine.State(); st.Dirty || st.Busy || st.Version != v1 {
		t.Errorf("after save: %+v", st)
	}

	files, err := h.s.Export("out/island", export.Options{Heightmap: true, WaterMask: true, Texture: true, OBJ: true, SVG: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"out/island-height.png", "out/island-water.png", "out/island-texture.png", "out/island.obj", "out/island.svg"} {
		if !slices.Contains(files, f) {
			t.Errorf("%s not exported (%v)", f, files)
		}
		if _, err := os.Stat(f); err != nil {
			t.Error(err)
		}
	}
}

func TestHotReload(t *testing.T) {
	h := setup(t)
	v0 := h.s.Engine.State().Version

	// Broken project: error reported, world kept
	os.WriteFile("config.json", []byte(`{"image": `), 0o644)
	st := h.waitFor(func(st State) bool { return st.Error != "" })
	if !st.Ready || st.Version != v0 {
		t.Errorf("after broken project: %+v", st)
	}

	// Fixed: regenerated, error cleared
	os.WriteFile("config.json", []byte(strings.Replace(testConfig, `"resolution": 1000`, `"resolution": 1500`, 1)), 0o644)
	st = h.waitFor(func(st State) bool { return st.Version > v0 && !st.Busy })
	if st.Error != "" || st.Stage != "mesh" {
		t.Errorf("after fixed project: %+v", st)
	}

	// New image: the mesh follows the terrains
	writeIsland(t, 20)
	st = h.waitFor(func(st State) bool { return st.Version > v0+1 && !st.Busy })
	if st.Stage != "mesh" {
		t.Errorf("after image change: %+v", st)
	}
}

func TestPaintedMap(t *testing.T) {
	h := setup(t)
	world, _ := h.s.Engine.Snapshot()
	v0 := h.s.Engine.State().Version

	// Paint the island away: all sea but a corner
	o := h.paintedMap()
	sea, land := o.Pixels[0], o.Pixels[40*o.Width+50]
	for i := range o.Pixels {
		o.Pixels[i] = sea
	}
	for y := range 20 {
		for x := range 20 {
			o.Pixels[y*o.Width+x] = land
		}
	}

	h.s.SetMap(o)
	h.waitFor(func(st State) bool { return st.Version > v0 && !st.Busy })
	painted, v1 := h.s.Engine.Snapshot()
	if painted.Map[40*o.Width+50] != sea {
		t.Errorf("painted map not generated")
	}

	// Saved to the image file, which reloads as is: nothing regenerated
	if _, err := h.s.Save(o); err != nil {
		t.Fatal(err)
	}
	time.Sleep(500 * time.Millisecond)
	if st := h.s.Engine.State(); st.Version != v1 || st.Busy {
		t.Errorf("own save reloaded: %+v", st)
	}

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

// A new project lives in memory until saved as, which doesn't regenerate.
func TestNewSaveAs(t *testing.T) {
	t.Chdir(t.TempDir())
	s, err := NewSession()
	if err != nil {
		t.Fatal(err)
	}
	h := newHarness(t, s)

	conf, _, err := config.Parse([]byte(testConfig))
	if err != nil {
		t.Fatal(err)
	}
	sea, land := conf.Terrain("sea").Color, conf.Terrain("land").Color
	o := &Map{Width: 64, Height: 64, Pixels: make([]config.Color, 64*64)}
	for i := range o.Pixels {
		o.Pixels[i] = sea
		if x, y := i%64, i/64; math.Hypot(float64(x-32), float64(y-32)) < 20 {
			o.Pixels[i] = land
		}
	}

	s.New(conf, o)
	st := h.waitFor(func(st State) bool { return st.Ready && !st.Busy })
	if !st.Dirty || s.ProjectPath() != "" || st.Error != "" {
		t.Errorf("new project: %+v, path %q", st, s.ProjectPath())
	}
	if _, err := s.Save(o); err == nil {
		t.Error("saved a project without a file")
	}

	path := filepath.Join("maps", "new")
	if err := s.SaveAs(path, o); err != nil {
		t.Fatal(err)
	}
	if s.ProjectPath() != path+".json" {
		t.Errorf("project path %q", s.ProjectPath())
	}
	loaded, _, err := config.Load(path + ".json")
	if err != nil || loaded.Image != "new.png" {
		t.Fatalf("saved project: %v %+v", err, loaded)
	}
	if _, err := os.Stat(filepath.Join("maps", "new.png")); err != nil {
		t.Error(err)
	}

	time.Sleep(300 * time.Millisecond)
	v := s.Engine.State().Version
	if st := s.Engine.State(); st.Dirty || st.Busy {
		t.Errorf("after save as: %+v", st)
	}

	// Editing after saving doesn't reload the image
	if err := s.Patch([]byte(`{"terrains": {"land": {"height": 800}}}`)); err != nil {
		t.Fatal(err)
	}
	st = h.waitFor(func(st State) bool { return st.Version > v && !st.Busy })
	if st.Stage != "terrain" || st.Error != "" {
		t.Errorf("after edit: %+v", st)
	}

	// Loading it again, from the file
	s2, err := Open(path + ".json")
	if err != nil {
		t.Fatal(err)
	}
	h2 := newHarness(t, s2)
	st = h2.waitFor(func(st State) bool { return st.Ready && !st.Busy })
	if st.Error != "" || st.Dirty {
		t.Errorf("reopened: %+v", st)
	}
}

func TestPreviewAndSupersede(t *testing.T) {
	h := setup(t)
	if err := h.s.Patch([]byte(`{"resolution": 500, "levels": 2, "terrains": {"land": {"height": 1000}}}`)); err != nil {
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
		return !st.Busy && !st.Preview && world.Config.Terrain("land").Height == 2000
	})
	if !previewed {
		t.Error("no preview shown")
	}
	if st := h.s.Engine.State(); st.Error != "" {
		t.Errorf("error: %s", st.Error)
	}
}

// Watching, a change of the simulation shows it in steps
func TestWatch(t *testing.T) {
	h := setup(t)
	h.s.Engine.SetWatch(20)
	v0 := h.s.Engine.State().Version
	if err := h.s.Patch([]byte(`{"simulation": {"erodibility": 3e-6}}`)); err != nil {
		t.Fatal(err)
	}

	var progress []string
	st := h.waitFor(func(st State) bool {
		if st.Progress != "" {
			progress = append(progress, st.Progress)
		}
		return !st.Busy && st.Version > v0 && st.Progress == ""
	})
	if len(progress) < 3 || !strings.Contains(progress[0], "step") {
		t.Errorf("progress: %v", progress)
	}
	if st.Error != "" || st.Preview {
		t.Errorf("after watching: %+v", st)
	}
}
