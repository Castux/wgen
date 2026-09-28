package gen

import (
	"math"
	"runtime"
	"sync"

	"github.com/Castux/wgen/internal/geom"
)

// parallelRows calls f on bands of rows [y0, y1) covering [0, n), in parallel.
func parallelRows(n int, f func(y0, y1 int)) {
	workers := min(runtime.GOMAXPROCS(0), n)
	var wg sync.WaitGroup
	for i := range workers {
		y0, y1 := n*i/workers, n*(i+1)/workers
		wg.Go(func() { f(y0, y1) })
	}
	wg.Wait()
}

// RasterizeTriangles calls pixel for every integer position (x, y) of the
// width * height grid covered by a mesh triangle, with the barycentric
// coordinates of the position. When triangles overlap on an edge, the last
// one wins. Rows are processed in parallel: pixel must only write to data for
// its own row.
func (w *World) RasterizeTriangles(width, height int, scale float64,
	pixel func(t int, x, y int, a, b, c float64)) {

	m := w.Mesh

	parallelRows(height, func(y0, y1 int) {
		for t, tri := range m.Triangles {
			p0 := m.Points[tri[0]].Scale(scale)
			p1 := m.Points[tri[1]].Scale(scale)
			p2 := m.Points[tri[2]].Scale(scale)

			minx := max(int(math.Floor(min(p0.X, p1.X, p2.X))), 0)
			maxx := min(int(math.Ceil(max(p0.X, p1.X, p2.X))), width-1)
			miny := max(int(math.Floor(min(p0.Y, p1.Y, p2.Y))), y0)
			maxy := min(int(math.Ceil(max(p0.Y, p1.Y, p2.Y))), y1-1)

			for y := miny; y <= maxy; y++ {
				for x := minx; x <= maxx; x++ {
					a, b, c := geom.Barycentric(p0, p1, p2, geom.Vec2{X: float64(x), Y: float64(y)})
					if a >= 0 && b >= 0 && c >= 0 {
						pixel(t, x, y, a, b, c)
					}
				}
			}
		}
	})
}

func (w *World) rasterize() {
	m := w.Mesh
	heightmap := nanSlice(w.Width * w.Height)
	water := nanSlice(w.Width * w.Height)

	// Water level is constant per triangle
	triangleWater := make([]float64, len(m.Triangles))
	for t, tri := range m.Triangles {
		triangleWater[t] = w.Lowest
		if w.IsWater(tri[0]) && w.IsWater(tri[1]) && w.IsWater(tri[2]) {
			triangleWater[t] = w.WaterLevel[tri[0]]
		}
	}

	w.RasterizeTriangles(w.Width, w.Height, 1, func(t, x, y int, a, b, c float64) {
		tri := m.Triangles[t]
		i := y*w.Width + x
		heightmap[i] = a*w.Z[tri[0]] + b*w.Z[tri[1]] + c*w.Z[tri[2]]
		water[i] = triangleWater[t]
	})

	w.Heightmap = heightmap
	w.WaterMap = water
}

// binomialCoefs returns the second half (center first) of the row of Pascal's
// triangle of the given order.
