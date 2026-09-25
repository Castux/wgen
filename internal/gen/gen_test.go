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
	"testing"

	"github.com/Castux/wgen/internal/config"
)

var update = flag.Bool("update", false, "update golden files")

var (
	sea       = color.NRGBA{66, 66, 125, 255}
	plains    = color.NRGBA{135, 168, 81, 255}
	hills     = color.NRGBA{209, 184, 134, 255}
	mountains = color.NRGBA{101, 72, 31, 255}
	lake      = color.NRGBA{109, 148, 194, 255}
)

const testConfig = `{
	"path": "island.png",
	"resolution": 6,
	"grid": "hex",
	"jitter": 0.5,
	"relax": true,
	"seed": 7,
	"smoothingRadius": 10,
	"erosionMinFlow": 5,
	"erosionFactor": 0.5,
	"blurRadius": 1,
	"terrains": {
		"sea": { "r": 66, "g": 66, "b": 125, "gradient": -0.1, "fixedShore": 0.0 },
		"plains": { "r": 135, "g": 168, "b": 81, "gradient": 0.2 },
		"hills": { "r": 209, "g": 184, "b": 134, "gradient": 0.8 },
		"mountains": { "r": 101, "g": 72, "b": 31, "gradient": 1.2 },
		"lake": { "r": 109, "g": 148, "b": 194, "gradient": -0.05, "smoothing": false, "erosion": false }
	}
}`

// writeIsland writes a test outline: a round island with hills, a mountain
// and a lake.
func writeIsland(t *testing.T, path string, size int) {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	s := float64(size)

	for y := range size {
		for x := range size {
			fx, fy := float64(x)/s, float64(y)/s
			d := math.Hypot(fx-0.5, fy-0.5)

			c := sea
			switch {
			case math.Hypot(fx-0.35, fy-0.6) < 0.06:
				c = lake
			case math.Hypot(fx-0.6, fy-0.4) < 0.07:
				c = mountains
			case d < 0.2:
				c = hills
			case d < 0.38:
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
	t.Chdir(t.TempDir())
	writeIsland(t, "island.png", 200)

	conf, _, err := config.Parse([]byte(testConfig))
	if err != nil {
		t.Fatal(err)
	}
	return conf
}

func generate(t *testing.T, conf *config.Config) *World {
	w, err := New(conf)
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func TestInvariants(t *testing.T) {
	w := generate(t, setup(t))

	var land, water, lakes int
	for v := range w.Mesh.Points {
		v := int32(v)
		switch {
		case w.Terrain[v] == nil:
			continue
		case w.IsSea(v) && w.Shore[v]:
			if w.Z[v] != 0 {
				t.Fatalf("sea shore %d at %f", v, w.Z[v])
			}
		case w.IsLand(v):
			land++
			if !(w.Z[v] > 0) {
				t.Fatalf("land vertex %d at %f", v, w.Z[v])
			}
		case w.IsLake(v):
			lakes++
			if !(w.WaterLevel[v] > 0) || !(w.Z[v] <= w.WaterLevel[v]) {
				t.Fatalf("lake vertex %d: z %f, water level %f", v, w.Z[v], w.WaterLevel[v])
			}
		case w.IsSea(v):
			water++
			if !(w.Z[v] < 0) || w.WaterLevel[v] != 0 {
				t.Fatalf("sea vertex %d: z %f, water level %f", v, w.Z[v], w.WaterLevel[v])
			}
		}
	}

	if land == 0 || water == 0 || lakes == 0 {
		t.Fatalf("land %d, sea %d, lake %d", land, water, lakes)
	}

	// Each vertex's flow is itself plus everything flowing into it
	inflow := make([]int32, len(w.Flow))
	for v, d := range w.Downhill {
		if d >= 0 {
			inflow[d] += w.Flow[v]
		}
	}
	for v := range w.Flow {
		if w.Flow[v] != inflow[v]+1 {
			t.Fatalf("flow of %d: %d, inflow %d", v, w.Flow[v], inflow[v])
		}
	}

	for i, h := range w.Heightmap {
		if math.IsNaN(h) || h < w.Lowest-1e-9 || h > w.Highest+1e-9 {
			t.Fatalf("heightmap pixel %d: %f", i, h)
		}
	}
}

// Downhill is computed before erosion and water depth: without erosion, it
// must be lower, at least for land flowing into land or sea (lake elevations
// are rewritten by water depth).
func TestDownhill(t *testing.T) {
	conf := setup(t)
	conf.ErosionMinFlow = math.MaxInt
	w := generate(t, conf)

	rivers := 0
	for v, d := range w.Downhill {
		if d >= 0 && w.IsLand(int32(v)) && !w.IsLake(d) {
			rivers++
			if !(w.Z[d] < w.Z[v]) {
				t.Fatalf("downhill of %d (%s, z %f) is not lower: %d (%s, shore %v, z %f)", v, w.Terrain[v].Name, w.Z[v], d, w.Terrain[d].Name, w.Shore[d], w.Z[d])
			}
		}
	}
	if rivers == 0 {
		t.Fatal("no rivers")
	}
}

func TestDeterministic(t *testing.T) {
	conf := setup(t)
	a, b := generate(t, conf), generate(t, conf)

	if !slices.Equal(a.Heightmap, b.Heightmap) || !equalNaN(a.Z, b.Z) {
		t.Errorf("same seed, different results")
	}

	conf2 := conf.Clone()
	conf2.Seed++
	c := generate(t, conf2)
	if slices.Equal(a.Heightmap, c.Heightmap) {
		t.Errorf("different seeds, same results")
	}
}

// Incremental updates must give the same result as generating from scratch.
func TestIncremental(t *testing.T) {
	conf := setup(t)
	base := generate(t, conf)
	baseHeightmap := slices.Clone(base.Heightmap)

	for _, tc := range []struct {
		patch string
		stage Stage
	}{
		{`{"blurRadius": 3}`, StageRaster},
		{`{"blurRadius": 0}`, StageRaster},
		{`{"erosionFactor": 0.2}`, StageErosion},
		{`{"erosionMinFlow": 50}`, StageErosion},
		{`{"terrains": {"hills": {"gradient": 1.5}}}`, StageTerrain},
		{`{"terrains": {"plains": {"erosion": false}}}`, StageTerrain},
		{`{"smoothingRadius": 0}`, StageTerrain},
		{`{"maxHeight": 1000}`, StageTerrain},
		{`{"resolution": 8}`, StageMesh},
		{`{"grid": "square", "relax": false}`, StageMesh},
		{`{"seed": 3}`, StageMesh},
		{`{"exportOBJ": true}`, StageNone},
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
		if !slices.Equal(incremental.Heightmap, full.Heightmap) || !equalNaN(incremental.Z, full.Z) {
			t.Errorf("%s: incremental update differs from full generation", tc.patch)
		}
	}

	if !slices.Equal(base.Heightmap, baseHeightmap) {
		t.Errorf("updates modified the original world")
	}
}

func TestReloadImage(t *testing.T) {
	conf := setup(t)
	w := generate(t, conf)

	// Same size: mesh is kept
	writeIsland(t, "island.png", 200)
	r, stage, err := w.ReloadImage()
	if err != nil || stage != StageTerrain || r.Mesh != w.Mesh {
		t.Errorf("same size reload: stage %v, err %v", stage, err)
	}

	writeIsland(t, "island.png", 150)
	r, stage, err = w.ReloadImage()
	if err != nil || stage != StageMesh || r.Width != 150 {
		t.Errorf("new size reload: stage %v, err %v", stage, err)
	}

	os.WriteFile("island.png", []byte("garbage"), 0o644)
	if _, _, err := w.ReloadImage(); err == nil {
		t.Errorf("reloading a broken image should fail")
	}
}

func TestNoSea(t *testing.T) {
	conf := setup(t)
	conf.Terrain("sea").FixedShore = math.NaN()
	if _, err := New(conf); err == nil {
		t.Errorf("expected an error without sea")
	}
}

// The original implementation computed elevation with a FIFO queue, revisiting
// vertices until no improvement. Dijkstra must give the same result.
func TestElevationMatchesFIFO(t *testing.T) {
	w := generate(t, setup(t))

	for _, erosion := range []bool{false, true} {
		fifo := nanSlice(len(w.Mesh.Points))
		var queue []int32
		for _, s := range w.Shores {
			if w.IsSea(s) {
				fifo[s] = w.Terrain[s].FixedShore
				queue = append(queue, s)
			}
		}

		for len(queue) > 0 {
			c := queue[0]
			queue = queue[1:]

			for _, n := range w.Mesh.Neighbours[c] {
				if w.Terrain[n] == nil || w.IsSea(n) {
					continue
				}
				gradient := w.Gradient[n]
				if w.IsLake(n) {
					gradient = 0.00001
				}
				if erosion && w.Terrain[n].Erosion && w.Downhill[n] == c && int(w.Flow[n]) > w.Conf.ErosionMinFlow {
					gradient *= w.Conf.ErosionFactor
				}
				newZ := fifo[c] + gradient*w.Mesh.Points[c].Dist(w.Mesh.Points[n])
				if math.IsNaN(fifo[n]) || newZ < fifo[n] {
					fifo[n] = newZ
					queue = append(queue, n)
				}
			}
		}

		c := *w
		c.computeElevation(erosion)
		for v := range fifo {
			if !(fifo[v] == c.Z[v] || math.IsNaN(fifo[v]) && math.IsNaN(c.Z[v]) || math.Abs(fifo[v]-c.Z[v]) < 1e-9) {
				t.Fatalf("erosion %v, vertex %d: FIFO %f, Dijkstra %f", erosion, v, fifo[v], c.Z[v])
			}
		}
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

	s := snapshot{Vertices: len(w.Mesh.Points), Lowest: w.Lowest, Highest: w.Highest}
	for i := range n {
		for j := range n {
			y, x := (2*i+1)*w.Height/(2*n), (2*j+1)*w.Width/(2*n)
			s.Samples[i][j] = math.Round(w.Heightmap[y*w.Width+x]*1e6) / 1e6
		}
	}

	if *update {
		data, _ := json.MarshalIndent(s, "", "\t")
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

	close := func(a, b float64) bool { return math.Abs(a-b) <= 1e-4*math.Max(1, math.Abs(b)) }

	if s.Vertices != want.Vertices || !close(s.Lowest, want.Lowest) || !close(s.Highest, want.Highest) {
		t.Errorf("got %d vertices, range %f..%f, want %d, %f..%f",
			s.Vertices, s.Lowest, s.Highest, want.Vertices, want.Lowest, want.Highest)
	}
	for i := range n {
		for j := range n {
			if !close(s.Samples[i][j], want.Samples[i][j]) {
				t.Errorf("sample %d,%d: got %f, want %f", i, j, s.Samples[i][j], want.Samples[i][j])
			}
		}
	}
}

func equalNaN(a, b []float64) bool {
	return slices.EqualFunc(a, b, func(x, y float64) bool {
		return x == y || math.IsNaN(x) && math.IsNaN(y)
	})
}
