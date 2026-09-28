// Package export writes a generated world to image, mesh and vector files.
package export

import (
	"bufio"
	"image"
	"image/png"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/Castux/wgen/internal/gen"
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
func All(w *gen.World, base string, options Options) ([]string, error) {
	if err := os.MkdirAll(filepath.Dir(base), 0o755); err != nil {
		return nil, err
	}

	type output struct {
		enabled bool
		path    string
		write   func(path string) error
	}
	var files []string
	for _, out := range []output{
		{options.Heightmap, base + "-height.png", func(path string) error { return Heightmap(w, path, options.Normalized) }},
		{options.WaterMask, base + "-water.png", func(path string) error { return WaterMask(w, path) }},
		{options.Texture, base + "-texture.png", func(path string) error { return Texture(w, path, options.TextureScale) }},
		{options.OBJ, base + ".obj", func(path string) error { return OBJ(w, path) }},
		{options.SVG, base + ".svg", func(path string) error { return SVG(w, path) }},
	} {
		if !out.enabled {
			continue
		}
		slog.Info("exporting", "path", out.path)
		if err := out.write(out.path); err != nil {
			return files, err
		}
		files = append(files, out.path)
	}
	return files, nil
}

// writeFile writes a text file, through a buffer.
func writeFile(path string, write func(*bufio.Writer)) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}

	out := bufio.NewWriter(file)
	write(out)

	if err := out.Flush(); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}

func writePNG(path string, img image.Image) error {
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
