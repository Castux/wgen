package app

import (
	"math"
	"runtime"
	"sync"

	"github.com/Castux/wgen/internal/gen"
	"github.com/Castux/wgen/internal/geom"
)

// Finer ground around the eye, at eye level: the mesh is about a kilometer
// between points, flat facets up close. Two modes:
//
//   - smooth: grids of finer steps near the eye (4 m, 16 m, 64 m, 256 m,
//     each four times as wide as the next), their heights smoothly
//     interpolated in the heightmap, with fractal details
//   - blocks: 1 m cubes near the eye, their tops rounded from the smooth
//     ground, then the smooth grids
//
// Each layer is drawn in a hole of the coarser one, and meets it at its
// edges, where its heights blend into the coarser one's. They are built in
// the background, and rebuilt as the eye moves away from their center.

// Ground at eye level, as saved
const (
	groundFacets = "facets"
	groundSmooth = "smooth"
	groundBlocks = "blocks"
)

var groundModes = []string{groundFacets, groundSmooth, groundBlocks}

// Layers of finer ground
const (
	gridSize      = 257 // vertices on a side of a smooth grid
	gridBlend     = 6   // cells at the edges blending into the coarser ground
	holeMargin    = 2   // cells at the edges also covered by the coarser ground
	blocksSize    = 256 // blocks on a side
	blockMeters   = 1
	rebuildFactor = 0.2 // of a layer's size: the eye going that far from its center rebuilds it
)

// groundPatch is a layer of finer ground, built on the CPU: a grid of
// heights (smooth), or blocks, with its vertices.
type groundPatch struct {
	center geom.Vec2  // map position, what it was built around
	bounds [4]float64 // map positions: x0, y0, x1, y1
	size   float64    // map units, of a side

	// Smooth grids: the heights of the vertices, meters, to find the height
	// anywhere in it as drawn
	origin geom.Vec2
	step   float64 // map units
	n      int
	height []float64

	// Blocks: the top of each, meters
	tops []float64

	vertices []terrainVertex
	indices  []uint32
}

// hole is where the coarser ground isn't drawn: the patch, but for a margin
// where both are, blended into each other, so that no crack shows between
// them.
func (p *groundPatch) hole() [4]float64 {
	margin := holeMargin * p.step
	return [4]float64{p.bounds[0] + margin, p.bounds[1] + margin, p.bounds[2] - margin, p.bounds[3] - margin}
}

func (p *groundPatch) inside(position geom.Vec2) bool {
	return position.X >= p.bounds[0] && position.Y >= p.bounds[1] && position.X <= p.bounds[2] && position.Y <= p.bounds[3]
}

// heightAt is the height of the ground drawn at a position, in meters.
func (p *groundPatch) heightAt(position geom.Vec2) (float64, bool) {
	if p == nil || !p.inside(position) {
		return 0, false
	}
	u := (position.X - p.origin.X) / p.step
	v := (position.Y - p.origin.Y) / p.step
	if p.tops != nil {
		i, j := min(int(u), p.n-1), min(int(v), p.n-1)
		return p.tops[j*p.n+i], true
	}
	i, j := min(int(u), p.n-2), min(int(v), p.n-2)
	fu, fv := u-float64(i), v-float64(j)
	at := func(i, j int) float64 { return p.height[j*p.n+i] }

	// The triangles of the cell, as drawn: split along its diagonal
	if fu >= fv {
		return at(i, j) + fu*(at(i+1, j)-at(i, j)) + fv*(at(i+1, j+1)-at(i+1, j)), true
	}
	return at(i, j) + fv*(at(i, j+1)-at(i, j)) + fu*(at(i+1, j+1)-at(i, j+1)), true
}

// snap is the origin of a layer around a center, on multiples of step, so
// that vertices stay in place when it is rebuilt somewhere else.
func snap(center geom.Vec2, step float64, cells int) geom.Vec2 {
	half := float64(cells) / 2 * step
	return geom.Vec2{X: math.Round((center.X-half)/step) * step, Y: math.Round((center.Y-half)/step) * step}
}

// buildGrid builds a smooth grid around center, of step meters, with
// details down to finest meters, blending into parent at its edges.
func buildGrid(probe *gen.Probe, world *gen.World, center geom.Vec2, stepMeters, finest float64, parent func(geom.Vec2) float64) *groundPatch {
	step := stepMeters / world.MetersPerPixel
	n := gridSize
	origin := snap(center, step, n-1)
	p := &groundPatch{
		center: center, origin: origin, step: step, n: n, size: float64(n-1) * step,
		bounds: [4]float64{origin.X, origin.Y, origin.X + float64(n-1)*step, origin.Y + float64(n-1)*step},
		height: make([]float64, n*n), vertices: make([]terrainVertex, n*n),
	}

	parallelRows(n, func(j0, j1 int) {
		for j := j0; j < j1; j++ {
			for i := range n {
				position := geom.Vec2{X: origin.X + float64(i)*step, Y: origin.Y + float64(j)*step}
				fromEdge := min(i, j, n-1-i, n-1-j)
				outer := parent(position)
				z := probe.DetailedGround(position, finest)
				switch t := smoothstep(float64(fromEdge) / gridBlend); {
				case math.IsNaN(outer) && math.IsNaN(z):
					z = 0
				case math.IsNaN(outer):
				case math.IsNaN(z):
					z = outer
				default:
					z = geom.Lerp(outer, z, t)
				}
				p.height[j*n+i] = z
				p.vertices[j*n+i] = groundVertex(probe, position, z)
			}
		}
	})

	p.indices = make([]uint32, 0, 6*(n-1)*(n-1))
	for j := range n - 1 {
		for i := range n - 1 {
			a, b := uint32(j*n+i), uint32(j*n+i+1)
			c, d := b+uint32(n), a+uint32(n)
			p.indices = append(p.indices, a, b, c, a, c, d) // counterclockwise, y up
		}
	}
	p.addSkirt(stepMeters)
	return p
}

// addSkirt hangs a strip down from the edges of a grid, both sides drawn:
// where the coarser ground bends between two edge vertices (along its own
// facets' edges), it covers the crack in between.
func (p *groundPatch) addSkirt(depth float64) {
	n := p.n
	var edge []int // around the grid
	for i := range n {
		edge = append(edge, i) // bottom row
	}
	for j := 1; j < n; j++ {
		edge = append(edge, j*n+n-1) // right column
	}
	for i := n - 2; i >= 0; i-- {
		edge = append(edge, (n-1)*n+i) // top row
	}
	for j := n - 2; j >= 0; j-- {
		edge = append(edge, j*n) // left column
	}

	for k := 1; k < len(edge); k++ {
		a, b := p.vertices[edge[k-1]], p.vertices[edge[k]]
		base := uint32(len(p.vertices))
		lowA, lowB := a, b
		lowA.z -= float32(depth)
		lowB.z -= float32(depth)
		p.vertices = append(p.vertices, a, b, lowB, lowA)
		p.indices = append(p.indices, base, base+1, base+2, base, base+2, base+3, base, base+2, base+1, base, base+3, base+2)
	}
}

// groundVertex is a vertex of finer ground, in its terrain's color.
func groundVertex(probe *gen.Probe, position geom.Vec2, z float64) terrainVertex {
	v := terrainVertex{x: float32(position.X), y: float32(position.Y), z: float32(z)}
	if t := probe.TerrainAt(position); t != nil {
		v.r, v.g, v.b = t.Color[0], t.Color[1], t.Color[2]
		if t.IsWater() {
			v.a = 255
		}
	}
	return v
}

// buildBlocks builds 1 m blocks around center, their tops rounded from the
// ground below (the finest grid), water flat at its level.
func buildBlocks(probe *gen.Probe, world *gen.World, center geom.Vec2, below *groundPatch) *groundPatch {
	step := blockMeters / world.MetersPerPixel
	n := blocksSize
	origin := snap(center, step, n)
	p := &groundPatch{
		center: center, origin: origin, step: step, n: n, size: float64(n) * step,
		bounds: [4]float64{origin.X, origin.Y, origin.X + float64(n)*step, origin.Y + float64(n)*step},
		tops:   make([]float64, n*n),
	}

	// Tops and colors
	colors := make([]terrainVertex, n*n)
	parallelRows(n, func(j0, j1 int) {
		for j := j0; j < j1; j++ {
			for i := range n {
				position := geom.Vec2{X: origin.X + (float64(i)+0.5)*step, Y: origin.Y + (float64(j)+0.5)*step}
				ground, ok := below.heightAt(position)
				if !ok {
					ground = probe.DetailedGround(position, 2*blockMeters)
				}
				if math.IsNaN(ground) {
					ground = 0
				}
				top := math.Round(ground / blockMeters)
				vertex := groundVertex(probe, position, 0)
				if vertex.a == 255 {
					if point := probe.At(position); point.Water {
						top = math.Round(point.Surface / blockMeters)
					}
				}
				p.tops[j*n+i] = top * blockMeters

				// A little variation between blocks, as in the games
				shade := 0.93 + 0.14*float64(blockHash(origin, step, i, j)&0xff)/255
				vertex.r, vertex.g, vertex.b = scaleByte(vertex.r, shade), scaleByte(vertex.g, shade), scaleByte(vertex.b, shade)
				colors[j*n+i] = vertex
			}
		}
	})

	// Faces: the top of each block, and its sides down to lower neighbours
	// (at the edges, a little further, over the ground outside)
	topAt := func(i, j int) (float64, bool) {
		if i < 0 || j < 0 || i >= n || j >= n {
			return 0, false
		}
		return p.tops[j*n+i], true
	}
	quad := func(color terrainVertex, corners [4][3]float64) {
		base := uint32(len(p.vertices))
		for _, c := range corners {
			v := color
			v.x, v.y, v.z = float32(c[0]), float32(c[1]), float32(c[2])
			p.vertices = append(p.vertices, v)
		}
		p.indices = append(p.indices, base, base+1, base+2, base, base+2, base+3)
	}
	const edgeDepth = 3 * blockMeters
	for j := range n {
		for i := range n {
			top := p.tops[j*n+i]
			color := colors[j*n+i]
			x0, y0 := origin.X+float64(i)*step, origin.Y+float64(j)*step
			x1, y1 := x0+step, y0+step
			quad(color, [4][3]float64{{x0, y0, top}, {x1, y0, top}, {x1, y1, top}, {x0, y1, top}})

			side := color
			side.r, side.g, side.b = scaleByte(color.r, 0.8), scaleByte(color.g, 0.8), scaleByte(color.b, 0.8)
			for _, s := range []struct {
				di, dj  int
				corners [2][2]float64 // along the side, counterclockwise seen from outside
			}{
				{1, 0, [2][2]float64{{x1, y0}, {x1, y1}}},
				{-1, 0, [2][2]float64{{x0, y1}, {x0, y0}}},
				{0, 1, [2][2]float64{{x1, y1}, {x0, y1}}},
				{0, -1, [2][2]float64{{x0, y0}, {x1, y0}}},
			} {
				bottom, ok := topAt(i+s.di, j+s.dj)
				if !ok {
					bottom = top - edgeDepth
				}
				if bottom >= top {
					continue
				}
				a, b := s.corners[0], s.corners[1]
				quad(side, [4][3]float64{{a[0], a[1], bottom}, {b[0], b[1], bottom}, {b[0], b[1], top}, {a[0], a[1], top}})
			}
		}
	}
	return p
}

// blockHash is random bits for a block, from its position on the map.
func blockHash(origin geom.Vec2, step float64, i, j int) uint64 {
	h := uint64(int64(math.Round(origin.X/step))+int64(i))*0x9e3779b97f4a7c15 ^ uint64(int64(math.Round(origin.Y/step))+int64(j))*0xbf58476d1ce4e5b9
	h ^= h >> 31
	h *= 0x94d049bb133111eb
	return h ^ h>>29
}

func scaleByte(b uint8, factor float64) uint8 {
	return uint8(math.Round(math.Min(255, float64(b)*factor)))
}

func smoothstep(t float64) float64 {
	t = geom.Clamp(t, 0, 1)
	return t * t * (3 - 2*t)
}

// parallelRows calls f on bands of rows, in parallel.
func parallelRows(n int, f func(j0, j1 int)) {
	workers := min(runtime.GOMAXPROCS(0), n)
	var wg sync.WaitGroup
	for k := range workers {
		j0, j1 := n*k/workers, n*(k+1)/workers
		wg.Go(func() { f(j0, j1) })
	}
	wg.Wait()
}

// groundLayer is a layer of finer ground drawn by the terrain view.
type groundLayer struct {
	mesh   meshBuffers
	bounds [4]float64
}

// Steps of the smooth grids, meters, from the coarsest: each covers four
// times the one after
var gridSteps = []float64{256, 64, 16, 4}

// groundDetail builds the finer ground around the eye, in the background.
type groundDetail struct {
	mode  string
	world *gen.World

	// Built, from the coarsest: the grids, then blocks
	patches []*groundPatch
	meshes  []meshBuffers

	building bool
	results  chan groundBuild
}

type groundBuild struct {
	mode    string
	world   *gen.World
	patches []*groundPatch
	kept    int // the first patches were already shown
}

// updateGround keeps the finer ground around the eye: rebuilt in the
// background when the world, the mode changed, or the eye went away from
// its center; shown once built.
func (a *app) updateGround() {
	g := &a.ground
	if g.results == nil {
		g.results = make(chan groundBuild, 1)
	}
	select {
	case built := <-g.results:
		g.building = false
		a.showGround(built)
	default:
	}

	mode := a.settings.EyeGround
	if a.settings.View != viewEye || mode == groundFacets || a.world == nil || a.inspect.probe == nil {
		a.terrain.details = a.terrain.details[:0]
		return
	}
	a.terrain.details = a.terrain.details[:0]
	if g.world == a.world && g.mode == mode {
		for i, p := range g.patches {
			a.terrain.details = append(a.terrain.details, groundLayer{mesh: g.meshes[i], bounds: p.hole()})
		}
	}
	if g.building {
		return
	}

	// The layers still good, from the coarsest: the finer ones are rebuilt
	// from the first that isn't (they depend on it)
	v := a.terrain
	eye := geom.Vec2{X: v.eye.position.X() + v.width/2, Y: v.eye.position.Y() + v.height/2}
	layers := len(gridSteps)
	if mode == groundBlocks {
		layers++
	}
	kept := 0
	if g.world == a.world && g.mode == mode {
		for kept < min(len(g.patches), layers) && g.patches[kept].center.Dist(eye) <= rebuildFactor*g.patches[kept].size {
			kept++
		}
	}
	if kept == layers && len(g.patches) == layers {
		return
	}

	g.building = true
	probe, world := a.inspect.probe, a.world
	patches := append([]*groundPatch(nil), g.patches[:kept]...)
	go func() {
		defer a.wakeUp()
		for len(patches) < layers {
			k := len(patches)
			if k == len(gridSteps) {
				patches = append(patches, buildBlocks(probe, world, eye, patches[k-1]))
				continue
			}

			// Each grid meets the coarser ground at its edges
			parent := probe.Surface
			if k > 0 {
				coarser := patches[k-1]
				parent = func(p geom.Vec2) float64 {
					z, _ := coarser.heightAt(p)
					return z
				}
			}
			patches = append(patches, buildGrid(probe, world, eye, gridSteps[k], 2*gridSteps[k], parent))
		}
		g.results <- groundBuild{mode: mode, world: world, patches: patches, kept: kept}
	}()
}

// showGround uploads built layers.
func (a *app) showGround(built groundBuild) {
	g := &a.ground
	for len(g.meshes) < len(built.patches) {
		g.meshes = append(g.meshes, meshBuffers{})
	}
	for i := len(built.patches); i < len(g.meshes); i++ {
		g.meshes[i].delete()
	}
	g.meshes = g.meshes[:len(built.patches)]
	for i, p := range built.patches[built.kept:] {
		g.meshes[built.kept+i].upload(p.vertices, p.indices)
		p.vertices, p.indices = nil, nil // uploaded: only the heights are kept
	}
	g.patches, g.world, g.mode = built.patches, built.world, built.mode
	a.activity()
}

// groundHeight is the height of the ground drawn at a map position, in
// meters, at eye level: the finest layer there, else the mesh.
func (a *app) groundHeight(position geom.Vec2) float64 {
	g := &a.ground
	if a.settings.View == viewEye && a.settings.EyeGround != groundFacets && g.world == a.world {
		for i := len(g.patches) - 1; i >= 0; i-- {
			if z, ok := g.patches[i].heightAt(position); ok {
				return z
			}
		}
	}
	return a.inspect.probe.Surface(position)
}
