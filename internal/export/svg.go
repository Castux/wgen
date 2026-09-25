package export

import (
	"bufio"
	"fmt"
	"math"

	"github.com/Castux/wgen/internal/gen"
	"github.com/Castux/wgen/internal/geom"
)

// SVG draws the Voronoi cells colored by elevation, and the rivers.
func SVG(w *gen.World, path string) error {
	return writeFile(path, func(out *bufio.Writer) {
		m := w.Mesh
		width, height := float64(w.Width), float64(w.Height)

		fmt.Fprintf(out, `<svg width='%f' height='%f'
		viewBox='%f %f %f %f'
		xmlns='http://www.w3.org/2000/svg'
		version='1.1'
		xmlns:xlink='http://www.w3.org/1999/xlink'>`+"\n",
			width, height, 0.0, -height, width, height)

		for v := range m.Points {
			fmt.Fprint(out, `<polygon points="`)
			for i, t := range m.VertexTriangles[v] {
				if i > 0 {
					fmt.Fprint(out, " ")
				}
				c := m.Circumcenters[t]
				fmt.Fprintf(out, "%f,%f", c.X, -c.Y)
			}

			col := cellColor(w, v)
			fmt.Fprintf(out, `" fill="%s" stroke="%s" />`+"\n", col, col)
		}

		for v, d := range w.Downhill {
			if d < 0 {
				continue
			}

			p, q := m.Points[v], m.Points[d]
			strokeWidth := math.Sqrt(float64(w.Flow[v])) * math.Pow(w.Conf.Resolution/30, 2)

			fmt.Fprintf(out, `<line x1="%f" y1="%f" x2="%f" y2="%f" stroke="#0E443D" stroke-width="%f" />`+"\n",
				p.X, -p.Y, q.X, -q.Y, strokeWidth)
		}

		fmt.Fprint(out, "</svg>\n")
	})
}

func cellColor(w *gen.World, v int) string {
	terrain := w.Terrain[v]
	z := w.Z[v]
	lerp := geom.Lerp

	switch {
	case terrain == nil:
		return "pink"

	case terrain.Name == "lake":
		return "#0E443D"

	case terrain.Name == "sea":
		f := z / w.Lowest
		return fmt.Sprintf("rgb(%.2f,%.2f,%.2f)", lerp(95, 0, f), lerp(132, 10, f), lerp(255, 100, f))
	}

	f := z / w.Highest
	return fmt.Sprintf("rgb(%.2f,%.2f,%.2f)", lerp(84, 255, f), lerp(169, 255, f), lerp(50, 255, f))
}
