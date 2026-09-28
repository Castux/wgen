package export

import (
	"image"
	"log/slog"
	"math"

	"github.com/Castux/wgen/internal/gen"
	"github.com/Castux/wgen/internal/render"
)

// Heightmap writes the elevation as a 16 bits grayscale PNG: in meters above
// sea level (water and anything below 0 is 0), or normalized, from the
// lowest point (0) to the highest (65535).
func Heightmap(w *gen.World, path string, normalized bool) error {
	img := image.NewGray16(image.Rect(0, 0, w.Width, w.Height))
	low, span := 0.0, 1.0
	if normalized {
		low, span = w.Lowest, math.Max(w.Highest-w.Lowest, 1e-9)/65535
	}

	clamped := 0
	for y := range w.Height {
		for x := range w.Width {
			v := w.Heightmap[y*w.Width+x]
			if math.IsNaN(v) {
				v = low
			}
			v = math.Round((v - low) / span)
			if v > 65535 {
				clamped++
			}
			v = math.Max(0, math.Min(65535, v))

			// Flip vertically: images go down
			i := img.PixOffset(x, w.Height-1-y)
			img.Pix[i], img.Pix[i+1] = uint8(uint16(v)>>8), uint8(v)
		}
	}
	if clamped > 0 {
		slog.Warn("heightmap values above 65535 m were clamped", "pixels", clamped)
	}

	return writePNG(path, img)
}

// WaterMask writes a PNG, white where there is water (sea or lake), black on
// land.
func WaterMask(w *gen.World, path string) error {
	img := image.NewGray(image.Rect(0, 0, w.Width, w.Height))
	for y := range w.Height {
		for x := range w.Width {
			i := y*w.Width + x
			if level := w.WaterMap[i]; !math.IsNaN(level) && level > w.Heightmap[i] {
				img.Pix[img.PixOffset(x, w.Height-1-y)] = 255
			}
		}
	}
	return writePNG(path, img)
}

// Texture writes the map as seen in the app's map view: terrain colors,
// hillshading and rivers, at a scale of the map image.
func Texture(w *gen.World, path string, scale float64) error {
	if scale <= 0 {
		scale = 1
	}
	img := render.Render(w, render.Options{
		Scale: scale, Base: render.BaseTerrain, Shading: true,
		RiverPower: 0.5, RiverWidth: 4,
	})
	return writePNG(path, img)
}
