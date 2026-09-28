package gen

import (
	"log/slog"
	"math"

	"github.com/Castux/wgen/internal/config"
	"github.com/Castux/wgen/internal/geom"
	"github.com/Castux/wgen/internal/mesh"
)

// The meshes of the simulation, one per level of refinement: coarse, where
// the landscape settles, then finer ones that add detail. Each level's
// points are on a hex lattice of half the spacing of the previous one, which
// contains the previous level's points, plus new ones in between where the
// terrains ask for more detail.

// levelSpacing is the mesh spacing of a level, in pixels. The last level has
// the configured resolution.
func (w *World) levelSpacing(level int) float64 {
	return w.Conf.Resolution * math.Pow(2, float64(w.Conf.Levels-level))
}

// detailAt is the number of refinement levels wanted at a position.
func (w *World) detailAt(p geom.Vec2, terrains map[config.Color]*config.Terrain) int {
	if !w.inBounds(p) {
		return 0
	}
	terrain := terrains[w.pixel(p)]
	switch {
	case terrain == nil:
		return 0
	case terrain.Detail != config.DetailAuto:
		return terrain.Detail
	case terrain.IsWater():
		return 0
	}
	return w.Conf.Levels
}

// maxDetailAround is the highest detail within radius of p (sampled), so
// that refined regions extend a bit beyond their terrain.
func (w *World) maxDetailAround(p geom.Vec2, radius float64, terrains map[config.Color]*config.Terrain) int {
	detail := w.detailAt(p, terrains)
	for i := range 8 {
		angle := float64(i) * math.Pi / 4
		sample := geom.Vec2{X: p.X + radius*math.Cos(angle), Y: p.Y + radius*math.Sin(angle)}
		detail = max(detail, w.detailAt(sample, terrains))
	}
	return detail
}

// meshPoint is a point of the meshes, and the first level that has it.
type meshPoint struct {
	position geom.Vec2
	level    int
}

// generateMeshes builds the meshes of all levels.
func (w *World) generateMeshes() error {
	points := w.meshPoints()

	w.levels = make([]*mesh.Mesh, w.Conf.Levels+1)
	for k := range w.levels {
		var positions []geom.Vec2
		for _, p := range points {
			if p.level <= k {
				positions = append(positions, p.position)
			}
		}
		levelMesh, err := mesh.Build(positions)
		if err != nil {
			return err
		}
		w.levels[k] = levelMesh
		slog.Debug("level mesh", "level", k, "vertices", len(levelMesh.Points))
	}

	w.Mesh = w.levels[len(w.levels)-1]
	return nil
}

// meshPoints returns the points of all levels. A point is added at the first
// level where it is on the lattice, if the terrain wants that much detail,
// and kept at the finer levels. It is jittered once, when added, so that it
// is at the same position at every level.
func (w *World) meshPoints() []meshPoint {
	terrains := w.Conf.TerrainsByColor()
	margin := w.margin()
	seed := w.rng(streamMesh).Uint64()

	var points []meshPoint
	for k := 0; k <= w.Conf.Levels; k++ {
		spacing := w.levelSpacing(k)
		rowSpacing := spacing * math.Sqrt(3) / 2
		jitter := 0.35 * spacing

		for j := 0; -margin+float64(j)*rowSpacing < float64(w.Height)+margin; j++ {
			y := -margin + float64(j)*rowSpacing
			for i := 0; ; i++ {
				x := -margin + float64(i)*spacing + float64(j%2)*spacing/2
				if x >= float64(w.Width)+margin {
					break
				}

				// Point (i', j') of the coarser lattice is (2i' + j'%2, 2j')
				// on this one
				if k > 0 && j%2 == 0 && (i-(j/2)%2)%2 == 0 {
					continue
				}

				bits := hash2(seed, int64(k)<<32|int64(i), int64(j))
				p := geom.Vec2{
					X: x + (unit(bits)*2-1)*jitter,
					Y: y + (unit(hash2(bits, 1, 0))*2-1)*jitter,
				}
				if k > 0 && w.maxDetailAround(p, w.levelSpacing(k-1), terrains) < k {
					continue
				}
				points = append(points, meshPoint{p, k})
			}
		}
	}
	return points
}

// CellAreas returns the area of the Voronoi cell of each vertex, in square
// pixels. Cells on the hull are open, and those next to it can reach far
// out: they are limited to a few times the area of a hexagon of their
// average neighbour distance.
func CellAreas(m *mesh.Mesh) []float64 {
	areas := make([]float64, len(m.Points))
	for v, p := range m.Points {
		sum := 0.0
		for _, n := range m.Neighbours[v] {
			sum += p.Dist(m.Points[n])
		}
		spacing := sum / float64(max(1, len(m.Neighbours[v])))
		hexagon := math.Sqrt(3) / 2 * spacing * spacing

		triangles := m.VertexTriangles[v]
		if len(triangles) < 3 || len(triangles) != len(m.Neighbours[v]) {
			areas[v] = hexagon
			continue
		}

		// Shoelace formula on the circumcenters of the triangles around v
		twiceArea := 0.0
		for i, t := range triangles {
			c0, c1 := m.Circumcenters[t], m.Circumcenters[triangles[(i+1)%len(triangles)]]
			twiceArea += c0.X*c1.Y - c1.X*c0.Y
		}
		areas[v] = math.Min(math.Abs(twiceArea)/2, 3*hexagon)
	}
	return areas
}
