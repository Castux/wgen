// Package export writes a generated world to image and mesh files.
package export

import (
	"image/png"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/Castux/wgen/internal/gen"
	"github.com/Castux/wgen/internal/render"
)

// Options chooses the files to write.
type Options struct {
	Heightmap  bool `json:"heightmap"`  // 16 bits grayscale PNG, <base>-height.png
	Normalized bool `json:"normalized"` // heightmap from the lowest to the highest point, instead of meters above sea level
	WaterMask  bool `json:"waterMask"`  // 8 bits PNG, white where there is water, <base>-water.png
	Texture    bool `json:"texture"`    // the map with terrain colors, hillshading and rivers, <base>-texture.png
	OBJ        bool `json:"obj"`        // mesh, <base>.obj
	SVG        bool `json:"svg"`        // terrain cells and rivers, <base>.svg

	TextureScale float64 `json:"textureScale"` // relative to the map image, 1 if 0
}

// All writes the files chosen by the options, named from base (a path
// without extension), and returns the files written.
func All(w *gen.World, base string, o Options) ([]string, error) {
	if err := os.MkdirAll(filepath.Dir(base), 0o755); err != nil {
		return nil, err
	}
	var files []string
	write := func(path string, f func(string) error) error {
		slog.Info("exporting", "path", path)
		if err := f(path); err != nil {
			return err
		}
		files = append(files, path)
		return nil
	}

	type output struct {
		enabled bool
		path    string
		f       func(string) error
	}
	for _, out := range []output{
		{o.Heightmap, base + "-height.png", func(p string) error { return Heightmap(w, p, o.Normalized) }},
		{o.WaterMask, base + "-water.png", func(p string) error { return WaterMask(w, p) }},
		{o.Texture, base + "-texture.png", func(p string) error { return Texture(w, p, o.TextureScale) }},
		{o.OBJ, base + ".obj", func(p string) error { return OBJ(w, p) }},
		{o.SVG, base + ".svg", func(p string) error { return SVG(w, p) }},
	} {
		if !out.enabled {
			continue
		}
		if err := write(out.path, out.f); err != nil {
			return files, err
		}
	}
	return files, nil
}

// Texture writes the map as seen in the viewer's 2D view: terrain colors,
// hillshading and rivers.
func Texture(w *gen.World, path string, scale float64) error {
	if scale <= 0 {
		scale = 1
	}
	img := render.Render(w, render.Options{
		Scale: scale, Base: render.BaseTerrain, Shading: true,
		RiverPower: 0.5, RiverWidth: 4,
	})
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
