package gen

import (
	"math"
	"runtime"
	"sync"

	"github.com/Castux/wgen/internal/geom"
)

// parallelRanges calls f on ranges [start, end) covering [0, n), in parallel.
func parallelRanges(n int, f func(start, end int)) {
	workers := min(runtime.GOMAXPROCS(0), n)
	var wg sync.WaitGroup
	for i := range workers {
		start, end := n*i/workers, n*(i+1)/workers
		wg.Go(func() { f(start, end) })
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

	points := w.Mesh.Points
	parallelRanges(height, func(y0, y1 int) {
		for t, tri := range w.Mesh.Triangles {
			p0 := points[tri[0]].Scale(scale)
			p1 := points[tri[1]].Scale(scale)
			p2 := points[tri[2]].Scale(scale)

			minX := max(int(math.Floor(min(p0.X, p1.X, p2.X))), 0)
			maxX := min(int(math.Ceil(max(p0.X, p1.X, p2.X))), width-1)
			minY := max(int(math.Floor(min(p0.Y, p1.Y, p2.Y))), y0)
			maxY := min(int(math.Ceil(max(p0.Y, p1.Y, p2.Y))), y1-1)

			for y := minY; y <= maxY; y++ {
				for x := minX; x <= maxX; x++ {
					a, b, c := geom.Barycentric(p0, p1, p2, geom.Vec2{X: float64(x), Y: float64(y)})
					if a >= 0 && b >= 0 && c >= 0 {
						pixel(t, x, y, a, b, c)
					}
				}
			}
		}
	})
}

// rasterize interpolates the elevation of the mesh into the heightmap, and
// the water level into the water map.
func (w *World) rasterize() {
	triangles := w.Mesh.Triangles
	heightmap := nanSlice(w.Width * w.Height)
	water := nanSlice(w.Width * w.Height)

	// Water level is constant per triangle
	triangleWater := make([]float64, len(triangles))
	for t, tri := range triangles {
		triangleWater[t] = w.Lowest
		if w.IsWater(tri[0]) && w.IsWater(tri[1]) && w.IsWater(tri[2]) {
			triangleWater[t] = w.WaterLevel[tri[0]]
		}
	}

	w.RasterizeTriangles(w.Width, w.Height, 1, func(t, x, y int, a, b, c float64) {
		tri := triangles[t]
		i := y*w.Width + x
		heightmap[i] = a*w.Z[tri[0]] + b*w.Z[tri[1]] + c*w.Z[tri[2]]
		water[i] = triangleWater[t]
	})

	w.Heightmap = heightmap
	w.WaterMap = water
}
