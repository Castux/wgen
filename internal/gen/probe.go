package gen

import (
	"math"

	"github.com/Castux/wgen/internal/config"
	"github.com/Castux/wgen/internal/geom"
)

// Probe tells what is at points of a world, and along paths: for the
// information under the cursor, and altitude profiles. It indexes the
// vertices on a grid, for the closest one.
type Probe struct {
	w        *World
	terrains map[config.Color]*config.Terrain

	cellSize      float64
	columns, rows int
	cells         [][]int32 // vertices by cell, row by row
}

// Point is what is at a position of the map.
type Point struct {
	Position geom.Vec2 // map pixels, y up
	Inside   bool      // in the map; nothing else is set otherwise

	Terrain *config.Terrain // nil if the color matches no terrain
	Ground  float64         // meters: the elevation, the bed under water
	Water   bool
	Surface float64 // meters, of the water, if any
	Depth   float64 // meters, of the water, if any

	// Area draining through the closest vertex, square meters: the size of
	// the river there
	Drainage float64
}

// NewProbe indexes a world.
func NewProbe(w *World) *Probe {
	p := &Probe{w: w, terrains: w.Config.TerrainsByColor()}
	if w.Mesh == nil || len(w.Mesh.Points) == 0 {
		return p
	}

	// About two vertices per cell
	area := float64(w.Width) * float64(w.Height)
	p.cellSize = math.Max(1, math.Sqrt(2*area/float64(len(w.Mesh.Points))))
	p.columns = int(math.Ceil(float64(w.Width)/p.cellSize)) + 1
	p.rows = int(math.Ceil(float64(w.Height)/p.cellSize)) + 1
	p.cells = make([][]int32, p.columns*p.rows)
	for v, point := range w.Mesh.Points {
		if cell, ok := p.cell(point); ok {
			p.cells[cell] = append(p.cells[cell], int32(v))
		}
	}
	return p
}

func (p *Probe) cell(position geom.Vec2) (int, bool) {
	column, row := int(position.X/p.cellSize), int(position.Y/p.cellSize)
	if position.X < 0 || position.Y < 0 || column >= p.columns || row >= p.rows {
		return 0, false
	}
	return row*p.columns + column, true
}

// Inside tells whether a position is in the map.
func (p *Probe) Inside(position geom.Vec2) bool {
	return position.X >= 0 && position.Y >= 0 && position.X <= float64(p.w.Width) && position.Y <= float64(p.w.Height)
}

// At is what is at a position, in map pixels (y up).
func (p *Probe) At(position geom.Vec2) Point {
	point := Point{Position: position, Inside: p.Inside(position)}
	if !point.Inside {
		return point
	}
	w := p.w

	x := min(int(position.X), w.Width-1)
	y := min(int(position.Y), w.Height-1)
	if len(w.Map) == w.Width*w.Height {
		point.Terrain = p.terrains[w.Map[y*w.Width+x]]
	}

	point.Ground = p.Ground(position)
	closest := p.closest(position)
	if closest >= 0 && closest < int32(len(w.Drainage)) {
		point.Drainage = w.Drainage[closest] * w.MetersPerPixel * w.MetersPerPixel
	}

	// Water where there is some above the ground. Triangles along the
	// shore, partly land, have no water level (the lowest elevation, in the
	// water map): the closest vertex has it
	if point.Terrain == nil || !point.Terrain.IsWater() || len(w.WaterMap) != w.Width*w.Height {
		return point
	}
	surface := w.WaterMap[y*w.Width+x]
	if !(surface > point.Ground) && closest >= 0 && w.IsWater(closest) {
		surface = w.WaterLevel[closest]
	}
	if surface > point.Ground {
		point.Water = true
		point.Surface = surface
		point.Depth = surface - point.Ground
	}
	return point
}

// Ground is the elevation at a position (the bed under water), in meters,
// interpolated in the heightmap. NaN outside of the map.
func (p *Probe) Ground(position geom.Vec2) float64 {
	w := p.w
	if !p.Inside(position) || len(w.Heightmap) != w.Width*w.Height {
		return math.NaN()
	}
	x := geom.Clamp(position.X, 0, float64(w.Width-1))
	y := geom.Clamp(position.Y, 0, float64(w.Height-1))
	x0, y0 := int(x), int(y)
	x1, y1 := min(x0+1, w.Width-1), min(y0+1, w.Height-1)
	fx, fy := x-float64(x0), y-float64(y0)

	at := func(x, y int) float64 {
		if z := w.Heightmap[y*w.Width+x]; !math.IsNaN(z) {
			return z
		}
		return w.Lowest
	}
	return geom.Lerp(geom.Lerp(at(x0, y0), at(x1, y0), fx), geom.Lerp(at(x0, y1), at(x1, y1), fx), fy)
}

// closest is the vertex closest to a position, among those of its cell and
// the neighbouring ones: -1 if none.
func (p *Probe) closest(position geom.Vec2) int32 {
	if p.cells == nil {
		return -1
	}
	best, bestDistance := int32(-1), math.Inf(1)
	column, row := int(position.X/p.cellSize), int(position.Y/p.cellSize)
	for r := max(row-1, 0); r <= min(row+1, p.rows-1); r++ {
		for c := max(column-1, 0); c <= min(column+1, p.columns-1); c++ {
			for _, v := range p.cells[r*p.columns+c] {
				if d := p.w.Mesh.Points[v].Dist(position); d < bestDistance {
					best, bestDistance = v, d
				}
			}
		}
	}
	return best
}

// Profile is the altitude profile along a path.
type Profile struct {
	Samples []ProfileSample
	Length  float64 // meters

	Lowest, Highest float64 // meters, of the ground and water surface
	Ascent, Descent float64 // meters, summed along the path, on the surface
	HasWater        bool
	Start, End      float64 // meters, surface elevations at both ends
}

// ProfileSample is a point of a profile.
type ProfileSample struct {
	Distance float64 // meters from the start
	Position geom.Vec2
	Ground   float64 // meters
	Surface  float64 // meters: the water surface, or the ground
	Water    bool
}

// Most samples of a profile: about one per map pixel, up to this
const maxProfileSamples = 2000

// Profile samples the elevation along a path (map pixels, y up), clipped to
// the map.
func (p *Probe) Profile(path []geom.Vec2) Profile {
	var profile Profile
	if len(path) < 2 {
		return profile
	}

	lengths := make([]float64, len(path)-1)
	total := 0.0
	for i := range lengths {
		lengths[i] = path[i+1].Dist(path[i])
		total += lengths[i]
	}
	metersPerPixel := p.w.MetersPerPixel
	profile.Length = total * metersPerPixel
	if total == 0 {
		return profile
	}

	count := int(math.Min(maxProfileSamples, math.Max(2, math.Ceil(total)+1)))
	step := total / float64(count-1)
	profile.Lowest, profile.Highest = math.Inf(1), math.Inf(-1)

	segment, segmentStart := 0, 0.0
	for i := range count {
		distance := float64(i) * step
		for segment < len(lengths)-1 && distance > segmentStart+lengths[segment] {
			segmentStart += lengths[segment]
			segment++
		}
		t := 0.0
		if lengths[segment] > 0 {
			t = math.Min(1, (distance-segmentStart)/lengths[segment])
		}
		a, b := path[segment], path[segment+1]
		position := geom.Vec2{X: geom.Lerp(a.X, b.X, t), Y: geom.Lerp(a.Y, b.Y, t)}

		point := p.At(position)
		if !point.Inside {
			continue
		}
		sample := ProfileSample{Distance: distance * metersPerPixel, Position: position, Ground: point.Ground, Surface: point.Ground}
		if point.Water {
			sample.Water, sample.Surface = true, point.Surface
			profile.HasWater = true
		}
		profile.Lowest = math.Min(profile.Lowest, sample.Ground)
		profile.Highest = math.Max(profile.Highest, sample.Surface)
		if n := len(profile.Samples); n > 0 {
			if change := sample.Surface - profile.Samples[n-1].Surface; change > 0 {
				profile.Ascent += change
			} else {
				profile.Descent -= change
			}
		}
		profile.Samples = append(profile.Samples, sample)
	}

	if n := len(profile.Samples); n > 0 {
		profile.Start, profile.End = profile.Samples[0].Surface, profile.Samples[n-1].Surface
	} else {
		profile.Lowest, profile.Highest = 0, 0
	}
	return profile
}
