package export

import (
	"image"
	"image/png"
	"log/slog"
	"math"
	"os"

	"github.com/Castux/wgen/internal/gen"
)

// Heightmaps writes the elevation and water level rasters as grayscale PNGs.
// Values are written as is (not normalized), clamped to the pixel range: use
// maxHeight in the config to control the scale.
func Heightmaps(w *gen.World, heightmapPath, waterLevelPath string) error {
	if err := grayPNG(w, w.Heightmap, heightmapPath); err != nil {
		return err
	}
	return grayPNG(w, w.WaterMap, waterLevelPath)
}

func grayPNG(w *gen.World, data []float64, path string) error {
	maxValue := 255.0
	var img interface {
		image.Image
		setValue(x, y int, v uint16)
	}

	if w.Conf.PNG16 {
		maxValue = 65535
		img = gray16{image.NewGray16(image.Rect(0, 0, w.Width, w.Height))}
	} else {
		img = gray8{image.NewGray(image.Rect(0, 0, w.Width, w.Height))}
	}

	outOfBounds := 0
	low, high := math.Inf(1), math.Inf(-1)

	for y := range w.Height {
		for x := range w.Width {
			v := data[y*w.Width+x]
			if math.IsNaN(v) {
				outOfBounds++
				v = 0
			}

			v = math.Floor(v)
			if v < 0 || v > maxValue {
				outOfBounds++
				low, high = math.Min(low, v), math.Max(high, v)
			}

			// Flip vertically: images go down
			img.setValue(x, w.Height-1-y, uint16(max(0, min(maxValue, v))))
		}
	}

	if outOfBounds > 0 {
		slog.Warn("some heightmap values are out of the pixel range and were clamped",
			"path", path, "pixels", outOfBounds, "low", low, "high", high, "max", maxValue)
	}

	slog.Info("exporting heightmap", "path", path, "bits", map[bool]int{false: 8, true: 16}[w.Conf.PNG16])

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

type gray8 struct{ *image.Gray }

func (g gray8) setValue(x, y int, v uint16) { g.Pix[g.PixOffset(x, y)] = uint8(v) }

type gray16 struct{ *image.Gray16 }

func (g gray16) setValue(x, y int, v uint16) {
	i := g.PixOffset(x, y)
	g.Pix[i] = uint8(v >> 8)
	g.Pix[i+1] = uint8(v)
}
