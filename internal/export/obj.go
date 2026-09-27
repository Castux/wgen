// Package export writes a generated world to OBJ, SVG and PNG files.
package export

import (
	"bufio"
	"fmt"
	"math"
	"os"

	"github.com/Castux/wgen/internal/gen"
)

// OBJ writes the mesh, with UVs mapping to the outline image. Triangles
// outside the map are skipped. Elevations are converted to pixels, the
// horizontal unit, so that the mesh has the true proportions.
func OBJ(w *gen.World, path string) error {
	return writeFile(path, func(out *bufio.Writer) {
		m := w.Mesh
		width, height := float64(w.Width), float64(w.Height)
		scale := 1.0
		if w.MetersPerPixel > 0 {
			scale = 1 / w.MetersPerPixel
		}

		for v, p := range m.Points {
			z := w.Z[v]
			if math.IsNaN(z) || math.IsInf(z, 0) {
				z = 0
			}

			fmt.Fprintf(out, "v %.6f %.6f %.6f\n", p.X, z*scale, -p.Y)
			fmt.Fprintf(out, "vt %.6f %.6f\n", p.X/width, 1-p.Y/height)
		}

		for _, tri := range m.Triangles {
			if w.Terrain[tri[0]] == nil || w.Terrain[tri[1]] == nil || w.Terrain[tri[2]] == nil {
				continue
			}

			// Reversed, since y is flipped
			a, b, c := tri[2]+1, tri[1]+1, tri[0]+1
			fmt.Fprintf(out, "f %d/%d %d/%d %d/%d\n", a, a, b, b, c, c)
		}
	})
}

func writeFile(path string, f func(*bufio.Writer)) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}

	out := bufio.NewWriter(file)
	f(out)

	if err := out.Flush(); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}
