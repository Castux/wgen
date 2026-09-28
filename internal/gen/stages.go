package gen

import (
	"container/heap"
	"fmt"
	"image"
	"image/color"
	"log/slog"
	"math"
	"os"
	"slices"

	_ "image/jpeg"
	_ "image/png"

	"github.com/Castux/wgen/internal/config"
)

// The stages of the pipeline, but for the meshes (meshes.go), the simulation
// (simulation.go) and the rasterization (raster.go).

// loadOutline loads the map image.
func (w *World) loadOutline() error {
	path := w.Conf.ImagePath()
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("could not load the map: %w", err)
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return fmt.Errorf("could not load the map %s: %w", path, err)
	}

	w.Width, w.Height, w.Outline = Outline(img)
	return nil
}

// Outline converts an image to a map: its colors, bottom row first.
func Outline(img image.Image) (width, height int, outline []config.Color) {
	b := img.Bounds()
	width, height = b.Dx(), b.Dy()
	outline = make([]config.Color, width*height)

	// Flip vertically: world y goes up
	for y := range height {
		row := height - 1 - y
		for x := range width {
			c := color.NRGBAModel.Convert(img.At(b.Min.X+x, b.Min.Y+y)).(color.NRGBA)
			outline[row*width+x] = config.Color{c.R, c.G, c.B}
		}
	}
	return width, height, outline
}

// assignTerrains gives each vertex the terrain of its pixel, and finds the
// shores.
func (w *World) assignTerrains() error {
	numVertices := len(w.Mesh.Points)
	w.Terrain = make([]*config.Terrain, numVertices)
	w.Shore = make([]bool, numVertices)
	w.Shores = nil

	terrains := w.Conf.TerrainsByColor()
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
		return fmt.Errorf("no coast: the map needs some sea next to land")
	}

	return nil
}

// computeWaterDepth gives the sea and lakes their level and depth. The sea is
// at level 0, each lake at the level of its lowest shore (where it spills).
// Their floors go down from their shores, with the floor slope.
func (w *World) computeWaterDepth() {
	level := w.waterLevels()
	w.Z = w.waterFloors(level)
	w.WaterLevel = level
}

// waterLevels returns the level of the water at each vertex, NaN on land.
func (w *World) waterLevels() []float64 {
	m := w.Mesh
	level := nanSlice(len(m.Points))

	// Each lake, found by flood fill
	visited := make([]bool, len(m.Points))
	for start := range int32(len(m.Points)) {
		if !w.IsLake(start) || visited[start] {
			continue
		}
		lake := []int32{start}
		visited[start] = true
		lakeLevel := math.Inf(1)
		for i := 0; i < len(lake); i++ {
			v := lake[i]
			if w.Shore[v] && !math.IsNaN(w.Z[v]) {
				lakeLevel = math.Min(lakeLevel, w.Z[v])
			}
			for _, n := range m.Neighbours[v] {
				if w.IsLake(n) && !visited[n] {
					visited[n] = true
					lake = append(lake, n)
				}
			}
		}
		if math.IsInf(lakeLevel, 1) {
			lakeLevel = 0
		}
		for _, v := range lake {
			level[v] = lakeLevel
		}
	}

	for v := range int32(len(m.Points)) {
		if w.IsSea(v) {
			level[v] = 0
		}
	}
	return level
}

// waterFloors returns the elevations with the floors of the sea and lakes:
// from the shores down, the highest first.
func (w *World) waterFloors(level []float64) []float64 {
	m := w.Mesh
	elevation := slices.Clone(w.Z)
	floorSlope := w.Conf.Simulation.FloorSlope * w.MetersPerPixel // meters per pixel

	for v := range int32(len(m.Points)) {
		if w.IsWater(v) {
			elevation[v] = math.NaN()
		}
	}
	for _, v := range w.Shores {
		elevation[v] = level[v]
	}

	negate := func(x float64) float64 { return -x }
	queue := &vertexQueue{elevation: elevation, key: negate}
	for _, v := range w.Shores {
		queue.push(v)
	}

	for queue.Len() > 0 {
		v, z := queue.pop()
		if z != elevation[v] {
			// Raised since it was pushed
			continue
		}
		for _, n := range m.Neighbours[v] {
			if !w.IsWater(n) || (w.IsSea(v) != w.IsSea(n)) {
				continue
			}
			floor := z - floorSlope*m.Points[v].Dist(m.Points[n])
			if math.IsNaN(elevation[n]) || floor > elevation[n] {
				elevation[n] = floor
				queue.push(n)
			}
		}
	}

	// Water not reached from a shore: at its level
	for v := range int32(len(m.Points)) {
		if w.IsWater(v) && math.IsNaN(elevation[v]) {
			elevation[v] = level[v]
		}
	}
	return elevation
}

// computeElevationRange sets the lowest and highest elevations of the map.
func (w *World) computeElevationRange() {
	w.Lowest = math.Inf(1)
	w.Highest = math.Inf(-1)

	for v, z := range w.Z {
		if w.Terrain[v] == nil || math.IsNaN(z) {
			continue
		}
		w.Lowest = math.Min(w.Lowest, z)
		w.Highest = math.Max(w.Highest, z)
	}
	if math.IsInf(w.Lowest, 1) {
		w.Lowest, w.Highest = 0, 0
	}
}

// vertexQueue is a min priority queue of vertices, keyed on their elevation
// at the time they were pushed (possibly transformed by key).
type vertexQueue struct {
	elevation []float64
	key       func(float64) float64
	items     []queueItem
}

type queueItem struct {
	v         int32
	elevation float64 // when pushed
	key       float64
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
	elevation := q.elevation[v]
	key := elevation
	if q.key != nil {
		key = q.key(elevation)
	}
	heap.Push(q, queueItem{v, elevation, key})
}

// pop returns the vertex with the lowest key, and its elevation when pushed.
func (q *vertexQueue) pop() (int32, float64) {
	item := heap.Pop(q).(queueItem)
	return item.v, item.elevation
}
