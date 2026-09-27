package gen

import (
	"image"
	"image/png"
	"math"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/Castux/wgen/internal/config"
	"github.com/Castux/wgen/internal/geom"
)

// setupUplift is the test island with the uplift model: 100 km wide, the
// mountain rising fast.
func setupUplift(t *testing.T) *config.Config {
	t.Helper()
	conf := setup(t)
	patched, err := conf.Patch([]byte(`{
		"elevationModel": "uplift", "mapWidth": 100, "resolution": 2, "levels": 2,
		"steps": 150, "refineSteps": 30, "upliftBlur": 3,
		"terrains": {
			"plains": {"uplift": 0.2, "detail": 1},
			"hills": {"uplift": 1},
			"mountains": {"uplift": 5}
		}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	return patched
}

func TestUpliftInvariants(t *testing.T) {
	conf := setupUplift(t)
	w := generate(t, conf)
	m := w.Mesh
	critical := math.Tan(conf.Uplift.CriticalSlope * math.Pi / 180)

	if w.MetersPerPixel != 500 {
		t.Errorf("meters per pixel %v", w.MetersPerPixel)
	}

	mean := map[string]float64{}
	count := map[string]float64{}
	for v := range int32(len(m.Points)) {
		if !w.IsLand(v) {
			continue
		}
		z := w.Z[v]
		if math.IsNaN(z) || z < 0 {
			t.Fatalf("land vertex %d at %v", v, z)
		}

		mean[w.Terrain[v].Name] += z
		count[w.Terrain[v].Name]++

		// Rivers flow down to the sea
		steps := 0
		for u := v; ; steps++ {
			d := w.Downhill[u]
			if d < 0 {
				if !w.IsWater(u) && !w.Shore[u] && !slices.ContainsFunc(m.Neighbours[u], w.IsSea) {
					t.Fatalf("river from %d ends at %d, on land", v, u)
				}
				break
			}
			// (Across lakes, rivers flow on the water, over their bed)
			if w.Z[d] >= w.Z[u] && w.IsLand(u) && w.IsLand(d) {
				t.Fatalf("river from %d goes up from %d to %d", v, u, d)
			}
			if steps > len(m.Points) {
				t.Fatalf("river from %d loops", v)
			}
			u = d
		}

		// Hillslopes are at most at the critical slope (the last erosion step
		// can make them a little steeper)
		if d := w.Downhill[v]; d >= 0 && w.IsLand(d) {
			slope := (z - w.Z[d]) / (m.Points[v].Dist(m.Points[d]) * w.MetersPerPixel)
			if slope > critical*1.5 {
				t.Errorf("slope %.2f from %d, critical %.2f", slope, v, critical)
			}
		}
	}

	// Heights in meters, the fast rising mountain much higher
	for name := range mean {
		mean[name] /= count[name]
	}
	if mean["mountains"] < 500 || mean["mountains"] < 2*mean["plains"] {
		t.Errorf("mean elevations: mountains %.0f m, plains %.0f m", mean["mountains"], mean["plains"])
	}

	// Drainage accumulates to about the land area
	land := 0.0
	for _, a := range CellAreas(m) {
		land += a
	}
	if maxDrainage := slices.Max(w.Drainage); maxDrainage <= 0 || maxDrainage > land {
		t.Errorf("drainage up to %v, total area %v", maxDrainage, land)
	}
}

// Faster uplift makes higher mountains.
func TestUpliftHeights(t *testing.T) {
	conf := setupUplift(t)
	slower, err := conf.Patch([]byte(`{"terrains": {"mountains": {"uplift": 2}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if a, b := generate(t, conf), generate(t, slower); !(a.Highest > b.Highest*1.2) {
		t.Errorf("highest %.0f m with uplift 5, %.0f m with uplift 2", a.Highest, b.Highest)
	}
}

func TestUpliftMeshes(t *testing.T) {
	w := generate(t, setupUplift(t))

	if len(w.levels) != 3 || w.Mesh != w.levels[2] {
		t.Fatalf("%d levels", len(w.levels))
	}

	// Each level has the points of the previous one, and more
	for k := 1; k < len(w.levels); k++ {
		fine := map[geom.Vec2]bool{}
		for _, p := range w.levels[k].Points {
			fine[p] = true
		}
		for _, p := range w.levels[k-1].Points {
			if !fine[p] {
				t.Fatalf("point %v of level %d missing from level %d", p, k-1, k)
			}
		}
		if len(w.levels[k].Points) <= len(w.levels[k-1].Points) {
			t.Errorf("level %d not finer", k)
		}
	}

	// Plains are refined once only: the mountain is denser
	density := func(name string) float64 {
		count := 0
		for v := range w.Mesh.Points {
			if t := w.Terrain[v]; t != nil && t.Name == name {
				count++
			}
		}
		return float64(count)
	}
	// Areas from the test image: plains ring 0.2..0.38 minus hills, mountain r = 0.07
	plainsArea := math.Pi * (0.38*0.38 - 0.2*0.2)
	mountainArea := math.Pi * 0.07 * 0.07
	if density("mountains")/mountainArea < 3*density("plains")/plainsArea {
		t.Errorf("mountain not refined more than plains: %v vs %v vertices", density("mountains"), density("plains"))
	}
}

func TestUpliftIncremental(t *testing.T) {
	conf := setupUplift(t)
	base := generate(t, conf)

	for _, tc := range []struct {
		patch string
		stage Stage
	}{
		{`{"terrains": {"mountains": {"uplift": 3}}}`, StageTerrain},
		{`{"erodibility": 4e-6, "criticalSlope": 25}`, StageTerrain},
		{`{"erosionFactor": 0.2}`, StageNone},
		{`{"terrains": {"hills": {"detail": 1}}}`, StageMesh},
		{`{"levels": 1}`, StageMesh},
		{`{"elevationModel": "slope"}`, StageMesh},
	} {
		patched, err := conf.Patch([]byte(tc.patch))
		if err != nil {
			t.Fatal(err)
		}

		incremental, stage, err := base.Update(patched)
		if err != nil {
			t.Fatal(err)
		}
		if stage != tc.stage {
			t.Errorf("%s: stage %v, expected %v", tc.patch, stage, tc.stage)
		}

		full := generate(t, patched)
		if !equalNaN(incremental.Z, full.Z) {
			t.Errorf("%s: incremental update differs from full generation", tc.patch)
		}
	}
}

func TestUpliftDeterministic(t *testing.T) {
	conf := setupUplift(t)
	a, b := generate(t, conf), generate(t, conf)
	if !equalNaN(a.Z, b.Z) {
		t.Error("two runs differ")
	}

	seeded, err := conf.Patch([]byte(`{"seed": 8}`))
	if err != nil {
		t.Fatal(err)
	}
	if c := generate(t, seeded); len(c.Z) == len(a.Z) && equalNaN(c.Z, a.Z) {
		t.Error("the seed changes nothing")
	}
}

func TestSlopeModelUnchanged(t *testing.T) {
	// The uplift parameters don't matter to the slope model
	conf := setup(t)
	patched, err := conf.Patch([]byte(`{"mapWidth": 50, "terrains": {"mountains": {"uplift": 3, "detail": 1}}}`))
	if err != nil {
		t.Fatal(err)
	}
	a, b := generate(t, conf), generate(t, patched)
	if !equalNaN(a.Z, b.Z) || a.MetersPerPixel != 1 {
		t.Error("uplift parameters change the slope model")
	}
	_ = config.ModelSlope
}

// Land at the edge of the image is simulated like the rest.
func TestUpliftMapEdge(t *testing.T) {
	conf := setupUplift(t)

	// Land on the left half, touching the image edges
	img := image.NewNRGBA(image.Rect(0, 0, 100, 100))
	for y := range 100 {
		for x := range 100 {
			c := sea
			if x < 50 {
				c = mountains
			}
			img.Set(x, y, c)
		}
	}
	f, err := os.Create("edge.png")
	if err != nil {
		t.Fatal(err)
	}
	png.Encode(f, img)
	f.Close()

	patched, err := conf.Patch([]byte(`{"path": "edge.png"}`))
	if err != nil {
		t.Fatal(err)
	}
	w := generate(t, patched)
	for v := range int32(len(w.Mesh.Points)) {
		if w.IsLand(v) && math.IsNaN(w.Z[v]) {
			t.Fatalf("land vertex %d at %v has no elevation", v, w.Mesh.Points[v])
		}
	}
}

// Terrains with a target height reach it: the high elevations of each are
// close to the target.
func TestUpliftCalibration(t *testing.T) {
	conf := setupUplift(t)
	patched, err := conf.Patch([]byte(`{"upliftBlur": 5, "terrains": {
		"plains": {"height": 300}, "hills": {"height": 1000}, "mountains": {"height": 3000}
	}}`))
	if err != nil {
		t.Fatal(err)
	}
	w := generate(t, patched)

	for _, terrain := range patched.Terrains {
		if !terrain.Calibrated() {
			continue
		}
		var zs []float64
		for v, tv := range w.Terrain {
			if tv != nil && tv.Name == terrain.Name {
				zs = append(zs, w.Z[v])
			}
		}
		slices.Sort(zs)
		summit := zs[int(0.95*float64(len(zs)-1))]
		if math.Abs(summit/terrain.Height-1) > 0.3 {
			t.Errorf("%s: summits at %.0f m, target %.0f m", terrain.Name, summit, terrain.Height)
		}
	}
}

// The uplift model shows a coarse preview, and can be canceled.
func TestUpliftPreviewCancel(t *testing.T) {
	conf := setupUplift(t)

	var previews []*World
	full, _, err := (&World{}).UpdateWith(conf, Options{Preview: func(p *World) { previews = append(previews, p) }})
	if err != nil {
		t.Fatal(err)
	}
	if len(previews) != 1 {
		t.Fatalf("%d previews", len(previews))
	}
	p := previews[0]
	if p.Mesh != full.levels[0] || len(p.Z) != len(p.Mesh.Points) || len(p.Heightmap) != p.Width*p.Height {
		t.Errorf("preview not of the coarse mesh")
	}
	if p.Highest <= 0 || p.Highest > 2*full.Highest {
		t.Errorf("preview highest %v, final %v", p.Highest, full.Highest)
	}

	// Canceled once previewed
	previewed := false
	_, _, err = (&World{}).UpdateWith(conf, Options{
		Preview:  func(*World) { previewed = true },
		Canceled: func() bool { return previewed },
	})
	if err != ErrCanceled {
		t.Errorf("canceled generation: %v", err)
	}
}

// An outline given in memory gives the same world as from the file.
func TestWithOutline(t *testing.T) {
	for _, conf := range []*config.Config{setup(t), setupUplift(t)} {
		fromFile := generate(t, conf)

		outline := slices.Clone(fromFile.Outline)
		fromMemory, _, err := (&World{}).WithOutline(conf, fromFile.Width, fromFile.Height, outline, Options{})
		if err != nil {
			t.Fatal(err)
		}
		if !equalNaN(fromFile.Z, fromMemory.Z) {
			t.Errorf("%s: outline in memory differs from the file", conf.Uplift.Model)
		}

		// Painting mountains in the sea changes the world, as a full generation
		for i := range 200 {
			outline[i*fromFile.Width+i/2] = config.Color{101, 72, 31}
		}
		painted, _, err := fromFile.WithOutline(conf, fromFile.Width, fromFile.Height, outline, Options{})
		if err != nil {
			t.Fatal(err)
		}
		full, _, err := (&World{}).WithOutline(conf, fromFile.Width, fromFile.Height, outline, Options{})
		if err != nil {
			t.Fatal(err)
		}
		if !equalNaN(painted.Z, full.Z) || equalNaN(painted.Z, fromFile.Z) {
			t.Errorf("%s: painted outline not regenerated right", conf.Uplift.Model)
		}
	}
}

// Watching the simulation shows its steps, without changing its result.
func TestUpliftWatch(t *testing.T) {
	conf := setupUplift(t)
	calibrated, err := conf.Patch([]byte(`{"terrains": {"mountains": {"height": 2000}}}`))
	if err != nil {
		t.Fatal(err)
	}

	for _, c := range []*config.Config{conf, calibrated} {
		plain := generate(t, c)

		var frames []*World
		var phases []string
		watched, _, err := (&World{}).UpdateWith(c, Options{
			WatchSteps: 20,
			Watch: func(w *World, progress string) {
				frames = append(frames, w)
				phases = append(phases, progress)
			},
		})
		if err != nil {
			t.Fatal(err)
		}

		if !equalNaN(plain.Z, watched.Z) {
			t.Error("watching changes the result")
		}
		if len(frames) < 10 {
			t.Fatalf("%d frames", len(frames))
		}
		// From the coarse mesh to the finest
		if frames[0].Mesh != watched.levels[0] || frames[len(frames)-1].Mesh != watched.Mesh {
			t.Error("frames not from the coarse to the final mesh")
		}
		for i, f := range frames {
			if len(f.Z) != len(f.Mesh.Points) || len(f.Heightmap) != f.Width*f.Height {
				t.Fatalf("frame %d incomplete", i)
			}
		}
		if c.Terrain("mountains").Calibrated() && !slices.ContainsFunc(phases, func(p string) bool { return strings.Contains(p, "Calibrating") }) {
			t.Errorf("no calibration phase in %v", phases[:3])
		}
	}
}

func TestRerun(t *testing.T) {
	w := generate(t, setupUplift(t))
	again, stage, err := w.Rerun(Options{})
	if err != nil || stage != StageTerrain || !equalNaN(w.Z, again.Z) {
		t.Errorf("rerun: stage %v, err %v, same %v", stage, err, equalNaN(w.Z, again.Z))
	}
}
