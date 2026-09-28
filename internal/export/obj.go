package export

import (
	"bufio"
	"fmt"
	"math"

	"github.com/Castux/wgen/internal/gen"
)

// OBJ writes the mesh, with UVs mapping to the map image. Triangles outside
// the map are skipped. Elevations are converted to pixels, the horizontal
// unit, so that the mesh has the true proportions.
func OBJ(w *gen.World, path string) error {
	return writeFile(path, func(out *bufio.Writer) {
		mesh := w.Mesh
		width, height := float64(w.Width), float64(w.Height)
		scale := 1.0
		if w.MetersPerPixel > 0 {
			scale = 1 / w.MetersPerPixel
		}

		for v, p := range mesh.Points {
			z := w.Elevation[v]
			if math.IsNaN(z) || math.IsInf(z, 0) {
				z = 0
			}

			fmt.Fprintf(out, "v %.6f %.6f %.6f\n", p.X, z*scale, -p.Y)
			fmt.Fprintf(out, "vt %.6f %.6f\n", p.X/width, 1-p.Y/height)
		}

		for _, triangle := range mesh.Triangles {
			if w.Terrain[triangle[0]] == nil || w.Terrain[triangle[1]] == nil || w.Terrain[triangle[2]] == nil {
				continue
			}

			// Reversed, since y is flipped. OBJ indices start at 1.
			a, b, c := triangle[2]+1, triangle[1]+1, triangle[0]+1
			fmt.Fprintf(out, "f %d/%d %d/%d %d/%d\n", a, a, b, b, c, c)
		}
	})
}
