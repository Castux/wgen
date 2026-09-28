package export

import (
	"bufio"
	"fmt"
	"math"

	"github.com/Castux/wgen/internal/config"
	"github.com/Castux/wgen/internal/gen"
	"github.com/Castux/wgen/internal/geom"
)

// SVG draws the Voronoi cells colored by elevation, and the rivers.
func SVG(w *gen.World, path string) error {
	return writeFile(path, func(out *bufio.Writer) {
		mesh := w.Mesh
		width, height := float64(w.Width), float64(w.Height)

		fmt.Fprintf(out, `<svg width='%f' height='%f'
		viewBox='%f %f %f %f'
		xmlns='http://www.w3.org/2000/svg'
		version='1.1'
		xmlns:xlink='http://www.w3.org/1999/xlink'>`+"\n",
			width, height, 0.0, -height, width, height)

		for v := range mesh.Points {
			fmt.Fprint(out, `<polygon points="`)
			for i, t := range mesh.VertexTriangles[v] {
				if i > 0 {
					fmt.Fprint(out, " ")
				}
				c := mesh.Circumcenters[t]
				fmt.Fprintf(out, "%f,%f", c.X, -c.Y)
			}

			color := cellColor(w, v)
			fmt.Fprintf(out, `" fill="%s" stroke="%s" />`+"\n", color, color)
		}

		// River widths grow like the square root of the drainage, in vertices
		// of the finest mesh (hexagonal cells, the resolution apart)
		resolution := w.Config.Resolution
		vertexArea := resolution * resolution * math.Sqrt(3) / 2
		for v, d := range w.Downhill {
			if d < 0 {
				continue
			}

			p, q := mesh.Points[v], mesh.Points[d]
			strokeWidth := math.Sqrt(w.Drainage[v]/vertexArea) * math.Pow(resolution/30, 2)

			fmt.Fprintf(out, `<line x1="%f" y1="%f" x2="%f" y2="%f" stroke="#0E443D" stroke-width="%f" />`+"\n",
				p.X, -p.Y, q.X, -q.Y, strokeWidth)
		}

		fmt.Fprint(out, "</svg>\n")
	})
}

// cellColor is the color of the cell of a vertex: blue in the sea, darker
// when deeper, and from green to white on land.
func cellColor(w *gen.World, v int) string {
	terrain := w.Terrain[v]
	z := w.Elevation[v]
	lerp := geom.Lerp

	switch {
	case terrain == nil:
		return "pink"

	case terrain.Name == config.LakeName:
		return "#0E443D"

	case terrain.Name == config.SeaName:
		f := ratio(z, w.Lowest)
		return fmt.Sprintf("rgb(%.2f,%.2f,%.2f)", lerp(95, 0, f), lerp(132, 10, f), lerp(255, 100, f))
	}

	f := ratio(z, w.Highest)
	return fmt.Sprintf("rgb(%.2f,%.2f,%.2f)", lerp(84, 255, f), lerp(169, 255, f), lerp(50, 255, f))
}

// ratio is z / extreme, 0 if the extreme is 0 (a flat world).
func ratio(z, extreme float64) float64 {
	if extreme == 0 {
		return 0
	}
	return z / extreme
}
