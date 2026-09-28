package gen

import (
	"encoding/json"
	"flag"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Castux/wgen/internal/config"
	"github.com/Castux/wgen/internal/geom"
)

var update = flag.Bool("update", false, "update golden files")

var (
	sea       = color.NRGBA{66, 66, 125, 255}
	plains    = color.NRGBA{135, 168, 81, 255}
	hills     = color.NRGBA{209, 184, 134, 255}
	mountains = color.NRGBA{101, 72, 31, 255}
	lake      = color.NRGBA{109, 148, 194, 255}
)

// The test island, 100 km wide
const testConfig = `{
	"image": "island.png",
	"mapWidth": 100, "resolution": 2, "levels": 2, "seed": 7,
	"terrains": {
		"sea": { "color": "#42427d" },
		"lake": { "color": "#6d94c2" },
		"plains": { "color": "#87a851", "height": 300, "detail": 1 },
		"hills": { "color": "#d1b886", "height": 1000 },
		"mountains": { "color": "#65481f", "height": 3000 }
	},
	"simulation": { "steps": 150, "refineSteps": 30, "upliftBlur": 5 }
}`

// writeIsland writes a test map: a round island with hills, a mountain and
// a lake.
func writeIsland(t *testing.T, path string, size int) {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	for y := range size {
		for x := range size {
			fx, fy := float64(x)/float64(size), float64(y)/float64(size)
			fromCenter := math.Hypot(fx-0.5, fy-0.5)

			c := sea
			switch {
			case math.Hypot(fx-0.35, fy-0.6) < 0.06:
				c = lake
			case math.Hypot(fx-0.6, fy-0.4) < 0.07:
				c = mountains
			case fromCenter < 0.2:
				c = hills
			case fromCenter < 0.38:
				c = plains
			}
			img.Set(x, y, c)
		}
	}

	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

func setup(t *testing.T) *config.Config {
	t.Helper()
	t.Chdir(t.TempDir())
	writeIsland(t, "island.png", 200)

	conf, _, err := config.Parse([]byte(testConfig))
	if err != nil {
		t.Fatal(err)
	}
	return conf
}

func generate(t *testing.T, conf *config.Config) *World {
	t.Helper()
	w, err := New(conf)
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func patch(t *testing.T, c *config.Config, p string) *config.Config {
	t.Helper()
	patched, err := c.Patch([]byte(p))
	if err != nil {
		t.Fatal(err)
	}
	return patched
}

func equalNaN(a, b []float64) bool {
	return slices.EqualFunc(a, b, func(x, y float64) bool {
		return x == y || math.IsNaN(x) && math.IsNaN(y)
	})
}

func TestInvariants(t *testing.T) {
	conf := setup(t)
	w := generate(t, conf)
	m := w.Mesh
	critical := math.Tan(conf.Simulation.CriticalSlope * math.Pi / 180)

	if w.MetersPerPixel != 500 {
		t.Errorf("meters per pixel %v", w.MetersPerPixel)
	}

	for v := range int32(len(m.Points)) {
		if !w.IsLand(v) {
			continue
		}
		z := w.Z[v]
		if math.IsNaN(z) || z < 0 {
			t.Fatalf("land vertex %d at %v", v, z)
		}

		// Rivers flow down to the sea
		steps := 0
		for u := v; ; steps++ {
			next := w.Downhill[u]
			if next < 0 {
				if !w.IsWater(u) && !slices.ContainsFunc(m.Neighbours[u], w.IsWater) {
					t.Fatalf("river from %d ends at %d, on land", v, u)
				}
				break
			}
			// (Across lakes, rivers flow on the water, over their bed)
			if w.Z[next] >= w.Z[u] && w.IsLand(u) && w.IsLand(next) {
				t.Fatalf("river from %d goes up from %d to %d", v, u, next)
			}
			if steps > len(m.Points) {
				t.Fatalf("river from %d loops", v)
			}
			u = next
		}

		// Hillslopes are at most at the critical slope (the last erosion step
		// can make them a little steeper)
		if next := w.Downhill[v]; next >= 0 && w.IsLand(next) {
			slope := (z - w.Z[next]) / (m.Points[v].Dist(m.Points[next]) * w.MetersPerPixel)
			if slope > critical*1.5 {
				t.Errorf("slope %.2f from %d, critical %.2f", slope, v, critical)
			}
		}
	}

	// Water: the sea at 0, below it; each lake flat, its floor below
	for v := range int32(len(m.Points)) {
		switch {
		case w.IsSea(v):
			if w.WaterLevel[v] != 0 || w.Z[v] > 0 {
				t.Fatalf("sea vertex %d: level %v, floor %v", v, w.WaterLevel[v], w.Z[v])
			}
		case w.IsLake(v):
			if w.Z[v] > w.WaterLevel[v] || w.WaterLevel[v] <= 0 {
				t.Fatalf("lake vertex %d: level %v, floor %v", v, w.WaterLevel[v], w.Z[v])
			}
		}
	}

	// Drainage accumulates to at most the whole area
	total := 0.0
	for _, a := range CellAreas(m) {
		total += a
	}
	if maxDrainage := slices.Max(w.Drainage); maxDrainage <= 0 || maxDrainage > total {
		t.Errorf("drainage up to %v, total area %v", maxDrainage, total)
	}
}

// Terrains reach their target heights: the high elevations of each are close
// to the target, and higher targets make higher terrain.
func TestCalibration(t *testing.T) {
	conf := setup(t)
	w := generate(t, conf)

	summits := map[string]float64{}
	for _, terrain := range conf.Land() {
		var zs []float64
		for v, tv := range w.Terrain {
			if tv != nil && tv.Name == terrain.Name {
				zs = append(zs, w.Z[v])
			}
		}
		slices.Sort(zs)
		summits[terrain.Name] = zs[int(0.95*float64(len(zs)-1))]
		if math.Abs(summits[terrain.Name]/terrain.Height-1) > 0.3 {
			t.Errorf("%s: summits at %.0f m, target %.0f m", terrain.Name, summits[terrain.Name], terrain.Height)
		}
	}
	if !(summits["plains"] < summits["hills"] && summits["hills"] < summits["mountains"]) {
		t.Errorf("summits %v", summits)
	}
}

func TestMeshes(t *testing.T) {
	w := generate(t, setup(t))

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
	count := func(name string) float64 {
		n := 0
		for v := range w.Mesh.Points {
			if t := w.Terrain[v]; t != nil && t.Name == name {
				n++
			}
		}
		return float64(n)
	}
	plainsArea := math.Pi * (0.38*0.38 - 0.2*0.2)
	mountainArea := math.Pi * 0.07 * 0.07
	if count("mountains")/mountainArea < 3*count("plains")/plainsArea {
		t.Errorf("mountain not refined more than plains: %v vs %v vertices", count("mountains"), count("plains"))
	}
}

func TestIncremental(t *testing.T) {
	conf := setup(t)
	base := generate(t, conf)

	for _, tc := range []struct {
		patch string
		stage Stage
	}{
		{`{"terrains": {"mountains": {"height": 2000}}}`, StageTerrain},
		{`{"simulation": {"erodibility": 4e-6, "criticalSlope": 25}}`, StageTerrain},
		{`{"mapWidth": 80}`, StageTerrain},
		{`{"terrains": {"hills": {"detail": 1}}}`, StageMesh},
		{`{"levels": 1}`, StageMesh},
		{`{"seed": 3}`, StageMesh},
		{`{"image": "island.png"}`, StageNone},
	} {
		patched := patch(t, conf, tc.patch)

		incremental, stage, err := base.Update(patched)
		if err != nil {
			t.Fatal(err)
		}
		if stage != tc.stage {
			t.Errorf("%s: stage %v, expected %v", tc.patch, stage, tc.stage)
		}

		full := generate(t, patched)
		if !equalNaN(incremental.Z, full.Z) || !equalNaN(incremental.Heightmap, full.Heightmap) {
			t.Errorf("%s: incremental update differs from full generation", tc.patch)
		}
	}

	if !equalNaN(base.Z, generate(t, conf).Z) {
		t.Error("updates modified the original world")
	}
}

func TestDeterministic(t *testing.T) {
	conf := setup(t)
	a, b := generate(t, conf), generate(t, conf)
	if !equalNaN(a.Z, b.Z) {
		t.Error("two runs differ")
	}
	if c := generate(t, patch(t, conf, `{"seed": 8}`)); len(c.Z) == len(a.Z) && equalNaN(c.Z, a.Z) {
		t.Error("the seed changes nothing")
	}
}

func TestReloadImage(t *testing.T) {
	conf := setup(t)
	w := generate(t, conf)

	// Smaller island, same size
	img := image.NewNRGBA(image.Rect(0, 0, 200, 200))
	for y := range 200 {
		for x := range 200 {
			c := sea
			if math.Hypot(float64(x-100), float64(y-100)) < 40 {
				c = hills
			}
			img.Set(x, y, c)
		}
	}
	f, _ := os.Create("island.png")
	png.Encode(f, img)
	f.Close()

	reloaded, _, err := w.ReloadImage()
	if err != nil {
		t.Fatal(err)
	}
	if !equalNaN(reloaded.Z, generate(t, conf).Z) {
		t.Error("reloaded image differs from a full generation")
	}
}

// Land at the edge of the image is simulated like the rest.
func TestMapEdge(t *testing.T) {
	conf := setup(t)

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
	f, _ := os.Create("edge.png")
	png.Encode(f, img)
	f.Close()

	w := generate(t, patch(t, conf, `{"image": "edge.png"}`))
	for v := range int32(len(w.Mesh.Points)) {
		if w.IsLand(v) && math.IsNaN(w.Z[v]) {
			t.Fatalf("land vertex %d at %v has no elevation", v, w.Mesh.Points[v])
		}
	}
}

func TestNoCoast(t *testing.T) {
	conf := setup(t)
	img := image.NewNRGBA(image.Rect(0, 0, 50, 50))
	for i := range img.Pix {
		img.Pix[i] = []uint8{plains.R, plains.G, plains.B, 255}[i%4]
	}
	f, _ := os.Create("land.png")
	png.Encode(f, img)
	f.Close()

	if _, err := New(patch(t, conf, `{"image": "land.png"}`)); err == nil || !strings.Contains(err.Error(), "coast") {
		t.Errorf("all land: %v", err)
	}
}

// The simulation shows a coarse preview, and can be canceled.
func TestPreviewCancel(t *testing.T) {
	conf := setup(t)

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

	previewed := false
	_, _, err = (&World{}).UpdateWith(conf, Options{
		Preview:  func(*World) { previewed = true },
		Canceled: func() bool { return previewed },
	})
	if err != ErrCanceled {
		t.Errorf("canceled generation: %v", err)
	}
}

// Watching the simulation shows its steps, without changing its result.
func TestWatch(t *testing.T) {
	conf := setup(t)
	plain := generate(t, conf)

	var frames []*World
	var phases []string
	watched, _, err := (&World{}).UpdateWith(conf, Options{
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
	if frames[0].Mesh != watched.levels[0] || frames[len(frames)-1].Mesh != watched.Mesh {
		t.Error("frames not from the coarse to the final mesh")
	}
	if !slices.ContainsFunc(phases, func(p string) bool { return strings.Contains(p, "Calibrating") }) {
		t.Errorf("no calibration phase in %v", phases[:3])
	}
}

func TestRerun(t *testing.T) {
	w := generate(t, setup(t))
	again, stage, err := w.Rerun(Options{})
	if err != nil || stage != StageTerrain || !equalNaN(w.Z, again.Z) {
		t.Errorf("rerun: stage %v, err %v", stage, err)
	}
}

// A map given in memory gives the same world as from the file.
func TestWithOutline(t *testing.T) {
	conf := setup(t)
	fromFile := generate(t, conf)

	outline := slices.Clone(fromFile.Outline)
	fromMemory, _, err := (&World{}).WithOutline(conf, fromFile.Width, fromFile.Height, outline, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !equalNaN(fromFile.Z, fromMemory.Z) {
		t.Error("map in memory differs from the file")
	}

	// Painting mountains in the sea
	for i := range 200 {
		outline[i*fromFile.Width+i/2] = config.Color{101, 72, 31}
	}
	painted, _, err := fromFile.WithOutline(conf, fromFile.Width, fromFile.Height, outline, Options{})
	if err != nil {
		t.Fatal(err)
	}
	full, _, _ := (&World{}).WithOutline(conf, fromFile.Width, fromFile.Height, outline, Options{})
	if !equalNaN(painted.Z, full.Z) || equalNaN(painted.Z, fromFile.Z) {
		t.Error("painted map not regenerated right")
	}
}

// Golden snapshot of a downsampled heightmap, to catch unintended changes.
// Run with -update to regenerate after an intended change.
func TestGolden(t *testing.T) {
	golden, _ := filepath.Abs(filepath.Join("testdata", "golden.json"))
	w := generate(t, setup(t))

	const n = 16
	type snapshot struct {
		Vertices        int
		Lowest, Highest float64
		Samples         [n][n]float64
	}

	got := snapshot{Vertices: len(w.Mesh.Points), Lowest: w.Lowest, Highest: w.Highest}
	for i := range n {
		for j := range n {
			y, x := (2*i+1)*w.Height/(2*n), (2*j+1)*w.Width/(2*n)
			got.Samples[i][j] = math.Round(w.Heightmap[y*w.Width+x]*1e3) / 1e3
		}
	}

	if *update {
		data, _ := json.MarshalIndent(got, "", "\t")
		os.MkdirAll(filepath.Dir(golden), 0o755)
		if err := os.WriteFile(golden, data, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}

	data, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err, "(run with -update to create)")
	}
	var want snapshot
	json.Unmarshal(data, &want)

	near := func(a, b float64) bool { return math.Abs(a-b) <= 1e-3*math.Max(1, math.Abs(b)) }

	if got.Vertices != want.Vertices || !near(got.Lowest, want.Lowest) || !near(got.Highest, want.Highest) {
		t.Errorf("got %d vertices, range %f..%f, want %d, %f..%f",
			got.Vertices, got.Lowest, got.Highest, want.Vertices, want.Lowest, want.Highest)
	}
	for i := range n {
		for j := range n {
			if !near(got.Samples[i][j], want.Samples[i][j]) {
				t.Errorf("sample %d,%d: got %f, want %f", i, j, got.Samples[i][j], want.Samples[i][j])
			}
		}
	}
}
