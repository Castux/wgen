package gen

import (
	"container/heap"
	"fmt"
	"image"
	"image/color"
	"log/slog"
	"math"
	"math/rand/v2"
	"os"
	"slices"

	_ "image/jpeg"
	_ "image/png"

	"github.com/Castux/wgen/internal/config"
	"github.com/Castux/wgen/internal/geom"
	"github.com/Castux/wgen/internal/mesh"
)

// Random streams, so that each stage's randomness only depends on the seed
const (
	streamMesh = iota + 1
	streamSmoothing
)

func (w *World) rng(stream uint64) *rand.Rand {
	return rand.New(rand.NewPCG(w.Conf.Seed, stream))
}

func uniform(r *rand.Rand, a, b float64) float64 {
	return a + (b-a)*r.Float64()
}

func (w *World) loadOutline() error {
	f, err := os.Open(w.Conf.Path)
	if err != nil {
		return fmt.Errorf("could not load outline: %w", err)
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return fmt.Errorf("could not load outline %s: %w", w.Conf.Path, err)
	}

	b := img.Bounds()
	w.Width, w.Height = b.Dx(), b.Dy()
	w.Outline = make([]config.Color, w.Width*w.Height)

	// Flip vertically: world y goes up
	for y := range w.Height {
		row := w.Height - 1 - y
		for x := range w.Width {
			c := color.NRGBAModel.Convert(img.At(b.Min.X+x, b.Min.Y+y)).(color.NRGBA)
			w.Outline[row*w.Width+x] = config.Color{c.R, c.G, c.B}
		}
	}

	return nil
}

// Test hook to replace the generated points
var pointsHook func(w *World) []geom.Vec2

func (w *World) generatePoints() []geom.Vec2 {
	if pointsHook != nil {
		return pointsHook(w)
	}

	var points []geom.Vec2
	res := w.Conf.Resolution
	jitter := w.Conf.Jitter
	margin := w.margin()
	r := w.rng(streamMesh)

	jittered := func(x, y float64) geom.Vec2 {
		return geom.Vec2{
			X: x + uniform(r, -res, res)*jitter,
			Y: y + uniform(r, -res, res)*jitter,
		}
	}

	switch w.Conf.Grid {
	case config.GridSquare:
		for x := -margin; x < float64(w.Width)+margin; x += res {
			for y := -margin; y < float64(w.Height)+margin; y += res {
				points = append(points, jittered(x, y))
			}
		}

	case config.GridHex:
		for x := -margin; x < float64(w.Width)+margin; x += res {
			row := 0
			for y := -margin; y < float64(w.Height)+margin; y += res * math.Sqrt(3) / 2 {
				row++
				points = append(points, jittered(x+float64(row%2)*0.5*res, y))
			}
		}
	}

	return points
}

func (w *World) generateMesh() error {
	m, err := mesh.Build(w.generatePoints())
	if err != nil {
		return err
	}

	if w.Conf.Relax {
		m, err = m.Relax(func(p geom.Vec2) bool { return !w.inBoundsPlusHalfMargin(p) })
		if err != nil {
			return err
		}
	}

	w.Mesh = m
	return nil
}

func (w *World) assignTerrainTypes() error {
	numVertices := len(w.Mesh.Points)
	w.Terrain = make([]*config.Terrain, numVertices)
	w.Gradient = nanSlice(numVertices)
	w.Shore = make([]bool, numVertices)
	w.Shores = nil

	terrains := w.Conf.TerrainsByColor()
	r := w.rng(streamSmoothing)
	radius := w.Conf.SmoothingRadius
	numSamples := int(math.Ceil(math.Pow(radius/w.Conf.Resolution, 2)))

	badPixels := map[config.Color]int{}

	for v, p := range w.Mesh.Points {
		if !w.inBoundsPlusHalfMargin(p) {
			continue
		}

		pixel := w.pixel(p)
		terrain := terrains[pixel]
		if terrain == nil {
			badPixels[pixel]++
			continue
		}
		w.Terrain[v] = terrain

		if !terrain.Smoothing || radius <= 0 {
			w.Gradient[v] = terrain.Gradient
			continue
		}

		// Average the gradient over random samples around the vertex. Only
		// consider gradients of the same sign, so that shores don't move.

		sum := terrain.Gradient
		count := 1

		for range numSamples {
			dx := uniform(r, -radius, radius)
			dy := uniform(r, -radius, radius)
			sample := p.Add(geom.Vec2{X: dx, Y: dy})

			if !w.inBounds(sample) {
				continue
			}

			other := terrains[w.pixel(sample)]
			if other != nil && other.Gradient*terrain.Gradient > 0 {
				sum += other.Gradient
				count++
			}
		}

		w.Gradient[v] = sum / float64(count)
	}

	for pixel, count := range badPixels {
		slog.Warn("pixel color matches no terrain", "color", pixel.String(), "vertices", count)
	}

	for v := range w.Mesh.Points {
		v := int32(v)
		if w.IsWater(v) && slices.ContainsFunc(w.Mesh.Neighbours[v], w.IsLand) {
			w.Shore[v] = true
			w.Shores = append(w.Shores, v)
		}
	}

	if !slices.ContainsFunc(w.Shores, w.IsSea) {
		return fmt.Errorf("no sea defined: at least one terrain with a fixedShore must touch land")
	}

	return nil
}

// computeElevation builds elevation up from the sea shores: each vertex is at
// the lowest elevation reachable by climbing from the sea, with the slope
// given by its gradient. It is a shortest path problem, solved with Dijkstra.
//
// In the erosion pass, the slope is reduced along rivers big enough.
func (w *World) computeElevation(erosion bool) {
	m := w.Mesh
	z := nanSlice(len(m.Points))
	q := &vertexQueue{z: z}

	for _, s := range w.Shores {
		if w.IsSea(s) {
			z[s] = w.Terrain[s].FixedShore
			q.push(s)
		}
	}

	for q.Len() > 0 {
		c, cz := q.pop()
		if cz != z[c] {
			continue // stale entry
		}

		for _, n := range m.Neighbours[c] {
			if w.Terrain[n] == nil || w.IsSea(n) {
				continue
			}

			gradient := w.Gradient[n]
			if w.IsLake(n) {
				gradient = 0.00001
			}

			if erosion && w.Terrain[n].Erosion && w.Downhill[n] == c && int(w.Flow[n]) > w.Conf.ErosionMinFlow {
				gradient *= w.Conf.ErosionFactor
			}

			newZ := cz + gradient*m.Points[c].Dist(m.Points[n])
			if math.IsNaN(z[n]) || newZ < z[n] {
				z[n] = newZ
				q.push(n)
			}
		}
	}

	w.Z = z
}

// computeRiverFlow finds the steepest downhill neighbour of every vertex, and
// the flow of each vertex: 1 for itself plus the flow of everything uphill.
func (w *World) computeRiverFlow() {
	m := w.Mesh
	z := w.Z
	w.Downhill = make([]int32, len(m.Points))
	w.Flow = make([]int32, len(m.Points))

	var order []int32

	for v := range m.Points {
		w.Downhill[v] = -1
		w.Flow[v] = 1

		if math.IsNaN(z[v]) {
			continue
		}
		order = append(order, int32(v))

		// It needs to be actually downhill (avoid rivers along shores)
		lowest := int32(-1)
		for _, n := range m.Neighbours[v] {
			if z[n] < z[v] && (lowest < 0 || z[n] < z[lowest]) {
				lowest = n
			}
		}
		w.Downhill[v] = lowest
	}

	// Accumulate from the top down. Downhill is strictly lower, so every
	// vertex is done before the one it flows into.

	slices.SortFunc(order, func(a, b int32) int {
		switch {
		case z[a] > z[b]:
			return -1
		case z[a] < z[b]:
			return 1
		}
		return 0
	})

	for _, v := range order {
		if d := w.Downhill[v]; d >= 0 {
			w.Flow[d] += w.Flow[v]
		}
	}
}

// computeWaterDepth builds the depth of water bodies down from their shores,
// the same way as elevation. Lakes get the water level of their shore.
func (w *World) computeWaterDepth() {
	m := w.Mesh
	z := slices.Clone(w.Z)
	water := nanSlice(len(m.Points))

	for v := range m.Points {
		if w.IsWater(int32(v)) && !w.Shore[v] {
			z[v] = math.NaN()
		}
	}

	// Water gradients are negative: explore the highest vertices first
	neg := func(x float64) float64 { return -x }
	q := &vertexQueue{z: z, key: neg}

	for _, s := range w.Shores {
		if !math.IsNaN(z[s]) {
			q.push(s)
		}
	}

	for q.Len() > 0 {
		c, cz := q.pop()
		if cz != z[c] {
			continue
		}

		if w.IsSea(c) {
			water[c] = w.Terrain[c].FixedShore
		} else if w.IsLake(c) && w.Shore[c] {
			water[c] = cz
		}

		for _, n := range m.Neighbours[c] {
			if w.Terrain[n] == nil || w.IsLand(n) {
				continue
			}

			newZ := cz + w.Gradient[n]*m.Points[c].Dist(m.Points[n])
			if math.IsNaN(z[n]) || newZ > z[n] {
				z[n] = newZ
				if w.IsLake(n) {
					water[n] = water[c]
				}
				q.push(n)
			}
		}
	}

	w.Z = z
	w.WaterLevel = water
}

func (w *World) finalizeElevation() {
	w.Lowest = math.Inf(1)
	w.Highest = math.Inf(-1)

	for v, z := range w.Z {
		if w.Terrain[v] == nil || math.IsNaN(z) {
			continue
		}
		w.Lowest = math.Min(w.Lowest, z)
		w.Highest = math.Max(w.Highest, z)
	}

	if max := w.Conf.MaxHeight; max != 0 {
		for v := range w.Z {
			w.Z[v] = w.Z[v] / w.Highest * max
		}
		w.Lowest = w.Lowest / w.Highest * max
		w.Highest = max
	}
}

// vertexQueue is a min priority queue of vertices, keyed on their elevation
// at the time they were pushed (possibly transformed by key).
type vertexQueue struct {
	z     []float64
	key   func(float64) float64
	items []queueItem
}

type queueItem struct {
	v   int32
	z   float64
	key float64
}

func (q *vertexQueue) Len() int           { return len(q.items) }
func (q *vertexQueue) Less(i, j int) bool { return q.items[i].key < q.items[j].key }
func (q *vertexQueue) Swap(i, j int)      { q.items[i], q.items[j] = q.items[j], q.items[i] }
func (q *vertexQueue) Push(x any)         { q.items = append(q.items, x.(queueItem)) }

func (q *vertexQueue) Pop() any {
	last := q.items[len(q.items)-1]
	q.items = q.items[:len(q.items)-1]
	return last
}

func (q *vertexQueue) push(v int32) {
	z := q.z[v]
	key := z
	if q.key != nil {
		key = q.key(z)
	}
	heap.Push(q, queueItem{v, z, key})
}

func (q *vertexQueue) pop() (int32, float64) {
	item := heap.Pop(q).(queueItem)
	return item.v, item.z
}
