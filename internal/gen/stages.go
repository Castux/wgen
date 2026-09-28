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
)

// Random streams, so that each stage's randomness only depends on the seed
const (
	streamMesh = iota + 1
	streamNoise
)

func (w *World) rng(stream uint64) *rand.Rand {
	return rand.New(rand.NewPCG(w.Conf.Seed, stream))
}

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

func (w *World) assignTerrainTypes() error {
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
	m := w.Mesh
	z := slices.Clone(w.Z)
	water := nanSlice(len(m.Points))
	slope := w.Conf.Simulation.FloorSlope * w.MetersPerPixel // meters per pixel

	// Lake levels, per lake
	visited := make([]bool, len(m.Points))
	for start := range int32(len(m.Points)) {
		if !w.IsLake(start) || visited[start] {
			continue
		}
		lake := []int32{start}
		visited[start] = true
		level := math.Inf(1)
		for i := 0; i < len(lake); i++ {
			v := lake[i]
			if w.Shore[v] && !math.IsNaN(w.Z[v]) {
				level = math.Min(level, w.Z[v])
			}
			for _, n := range m.Neighbours[v] {
				if w.IsLake(n) && !visited[n] {
					visited[n] = true
					lake = append(lake, n)
				}
			}
		}
		if math.IsInf(level, 1) {
			level = 0
		}
		for _, v := range lake {
			water[v] = level
		}
	}

	// Floors: from the shores down, the highest first
	for v := range m.Points {
		switch {
		case w.IsSea(int32(v)):
			water[v] = 0
			z[v] = math.NaN()
		case w.IsLake(int32(v)):
			z[v] = math.NaN()
		}
	}
	for _, s := range w.Shores {
		z[s] = water[s]
	}

	neg := func(x float64) float64 { return -x }
	q := &vertexQueue{z: z, key: neg}
	for _, s := range w.Shores {
		q.push(s)
	}

	for q.Len() > 0 {
		c, cz := q.pop()
		if cz != z[c] {
			continue
		}
		for _, n := range m.Neighbours[c] {
			if !w.IsWater(n) || (w.IsSea(c) != w.IsSea(n)) {
				continue
			}
			newZ := cz - slope*m.Points[c].Dist(m.Points[n])
			if math.IsNaN(z[n]) || newZ > z[n] {
				z[n] = newZ
				q.push(n)
			}
		}
	}

	// Water not reached from a shore: at its level
	for v := range m.Points {
		if w.IsWater(int32(v)) && math.IsNaN(z[v]) {
			z[v] = water[v]
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
	if math.IsInf(w.Lowest, 1) {
		w.Lowest, w.Highest = 0, 0
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
