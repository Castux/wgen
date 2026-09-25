package render

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Castux/wgen/internal/config"
	"github.com/Castux/wgen/internal/gen"
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

func island(t *testing.T) *gen.World {
	t.Chdir(t.TempDir())

	img := image.NewNRGBA(image.Rect(0, 0, 100, 80))
	for y := range 80 {
		for x := range 100 {
			c := color.NRGBA{66, 66, 125, 255}
			if math.Hypot(float64(x-50), float64(y-40)) < 30 {
				c = color.NRGBA{135, 168, 81, 255}
			}
			img.Set(x, y, c)
		}
	}
	f, _ := os.Create("island.png")
	png.Encode(f, img)
	f.Close()

	conf, _, err := config.Parse([]byte(testConfig))
	if err != nil {
		t.Fatal(err)
	}
	w, err := gen.New(conf)
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func TestRender(t *testing.T) {
	w := island(t)

	img := Render(w, Options{Scale: 2, Base: BaseTerrain, Shading: true})
	if img.Rect.Dx() != 200 || img.Rect.Dy() != 160 {
		t.Fatalf("size %v", img.Rect)
	}

	// Top left is sea, center is land (shaded, so only roughly)
	sea := img.RGBAAt(2, 2)
	land := img.RGBAAt(100, 80)
	if sea.B <= sea.G || land.G <= land.B {
		t.Errorf("sea %v, land %v", sea, land)
	}

	// Overlay: white, with some river pixels
	overlay := Render(w, Options{Scale: 2, Base: BaseNone, RiverPower: 0.5, RiverWidth: 4})
	if overlay.RGBAAt(2, 2) != (color.RGBA{255, 255, 255, 255}) {
		t.Errorf("overlay background %v", overlay.RGBAAt(2, 2))
	}
	rivers := 0
	for i := 0; i < len(overlay.Pix); i += 4 {
		if overlay.Pix[i] < 128 {
			rivers++
		}
	}
	if rivers == 0 {
		t.Errorf("no rivers drawn")
	}

	// Contours and grid darken some pixels
	lines := Render(w, Options{Scale: 1, Base: BaseNone, Contours: 5, Grid: 10})
	if lines.RGBAAt(0, 0) == (color.RGBA{255, 255, 255, 255}) {
		t.Errorf("no grid line at the origin")
	}

	height := Render(w, Options{Scale: 1, Base: BaseHeight})
	if height.RGBAAt(50, 40).R <= height.RGBAAt(2, 2).R {
		t.Errorf("center should be higher than the sea")
	}
}

func TestMaxSize(t *testing.T) {
	w := island(t)
	width, height, scale := Size(w, 1000)
	if width != MaxSize || height != int(math.Round(MaxSize*0.8)) || scale != MaxSize/100.0 {
		t.Errorf("got %dx%d at %f", width, height, scale)
	}
}

// Timing on a real config: WGEN_BENCH=path/to/config.json go test ./internal/render -run Timing -v
func TestTiming(t *testing.T) {
	path := os.Getenv("WGEN_BENCH")
	if path == "" {
		t.Skip("WGEN_BENCH not set")
	}

	path, _ = filepath.Abs(path)
	t.Chdir(filepath.Dir(path))
	conf, _, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	w, err := gen.New(conf)
	if err != nil {
		t.Fatal(err)
	}

	for _, o := range []Options{
		{Scale: 1, Base: BaseTerrain, Shading: true, RiverPower: 0.5, RiverWidth: 10, Contours: 20},
		{Scale: 2, Base: BaseNone, RiverPower: 0.5, RiverWidth: 10},
		{Scale: 2, Base: BaseNone, RiverPower: 0.5, RiverWidth: 10, Contours: 20, Grid: 100},
	} {
		start := time.Now()
		img := Render(w, o)
		rendered := time.Since(start)

		var buf bytes.Buffer
		enc := png.Encoder{CompressionLevel: png.BestSpeed}
		enc.Encode(&buf, img)
		t.Logf("%+v: %v render, %v encode, %d KB", o, rendered, time.Since(start)-rendered, buf.Len()/1024)
	}
}
