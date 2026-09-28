package gen

import (
	"fmt"
	"log/slog"
	"math"
	"slices"

	"github.com/Castux/wgen/internal/config"
	"github.com/Castux/wgen/internal/geom"
)

// The uplift field, and its calibration.
//
// Every connected region of a land terrain has its own uplift rate. Within a
// region, uplift ramps up from its border with lower terrains (or water)
// over the upliftBlur distance: ranges rise more in their core, without
// raising their neighbours.
//
// Terrains with a target height get the rate that makes the summits of each
// of their regions reach it: the height a region reaches also depends on its
// size and surroundings. The coarse level is simulated, the summit height of
// every region measured, and its rate scaled toward the target, a few times.
//
// Finer meshes then make higher summits: on the coarse mesh, every vertex
// drains a large area and erodes, while finer ones have ridges, which barely
// erode and rise up to the critical slope. So the coarse level is calibrated
// again, aiming at the target divided by how much the refinement raised each
// region, and refined again.

const (
	calibrationIterations = 8
	calibrationTolerance  = 0.05 // relative error of the summit height
	calibrationDamping    = 0.8  // exponent of the correction factors

	// Summit height of a region: this quantile of its elevations
	summitQuantile = 0.95

	// Regions with fewer coarse vertices are too small to be measured
	minRegionVertices = 10

	// Uplift limits, mm per year: regions too small to reach their height
	// don't get absurd rates
	minUplift, maxUplift = 0.001, 30

	// Uplift at the border of a region, relative to its core
	rampFloor = 0.3
)

// upliftField holds the regions of land terrains on a grid, and each cell's
// uplift factor, from its distance to the border of its region with lower
// terrains.
type upliftField struct {
	cellSize      float64           // pixels
	width, height int               // cells
	region        []int32           // of each cell, -1 for water or none
	ramp          []float64         // uplift factor of each cell, rampFloor..1
	regionTerrain []*config.Terrain // of each region
}

// rank orders terrains for the ramps: by target height.
func rank(t *config.Terrain) float64 { return t.Height }

func (w *World) newUpliftField() *upliftField {
	cellSize := w.levelSpacing(0) / 2
	f := &upliftField{cellSize: cellSize}
	f.width, f.height = int(math.Ceil(float64(w.Width)/cellSize)), int(math.Ceil(float64(w.Height)/cellSize))

	terrainAt := w.landTerrainGrid(f)
	f.findRegions(terrainAt)
	radius := w.Conf.Simulation.UpliftBlur * 1000 / w.MetersPerPixel / cellSize // cells
	f.computeRamps(terrainAt, radius)
	return f
}

// landTerrainGrid returns the land terrain at the center of each cell of the
// field, nil for water.
func (w *World) landTerrainGrid(f *upliftField) []*config.Terrain {
	terrains := w.Conf.TerrainsByColor()
	terrainAt := make([]*config.Terrain, f.width*f.height)
	for y := range f.height {
		for x := range f.width {
			p := geom.Vec2{X: (float64(x) + 0.5) * f.cellSize, Y: (float64(y) + 0.5) * f.cellSize}
			if t := terrains[w.pixel(p)]; t != nil && !t.IsWater() {
				terrainAt[y*f.width+x] = t
			}
		}
	}
	return terrainAt
}

// findRegions labels the connected regions of the same terrain,
// 4-connected.
func (f *upliftField) findRegions(terrainAt []*config.Terrain) {
	f.region = make([]int32, len(terrainAt))
	for i := range f.region {
		f.region[i] = -1
	}
	var stack []int
	for start, t := range terrainAt {
		if t == nil || f.region[start] >= 0 {
			continue
		}
		id := int32(len(f.regionTerrain))
		f.regionTerrain = append(f.regionTerrain, t)
		f.region[start] = id
		stack = append(stack[:0], start)

		for len(stack) > 0 {
			i := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			for _, j := range f.neighbours4(i) {
				if terrainAt[j] == t && f.region[j] < 0 {
					f.region[j] = id
					stack = append(stack, j)
				}
			}
		}
	}
}

// computeRamps sets the uplift factor of each land cell, from its distance to
// lower terrain or water, per rank: rampFloor at the border, 1 beyond radius
// (cells).
func (f *upliftField) computeRamps(terrainAt []*config.Terrain, radius float64) {
	f.ramp = make([]float64, len(terrainAt))
	ranks := map[float64]bool{}
	for _, t := range f.regionTerrain {
		ranks[rank(t)] = true
	}
	for r := range ranks {
		dist := make([]float64, len(terrainAt))
		for i, t := range terrainAt {
			if t == nil || rank(t) < r {
				dist[i] = 0
			} else {
				dist[i] = math.Inf(1)
			}
		}
		chamfer(dist, f.width, f.height)

		for i, t := range terrainAt {
			if t == nil || rank(t) != r {
				continue
			}
			x := 1.0
			if radius > 0 {
				x = math.Min(1, dist[i]/radius)
			}
			x = x * x * (3 - 2*x) // smoothstep
			f.ramp[i] = rampFloor + (1-rampFloor)*x
		}
	}
}

func (f *upliftField) neighbours4(i int) []int {
	x, y := i%f.width, i/f.width
	neighbours := make([]int, 0, 4)
	if x > 0 {
		neighbours = append(neighbours, i-1)
	}
	if x < f.width-1 {
		neighbours = append(neighbours, i+1)
	}
	if y > 0 {
		neighbours = append(neighbours, i-f.width)
	}
	if y < f.height-1 {
		neighbours = append(neighbours, i+f.width)
	}
	return neighbours
}

// chamfer turns a grid of 0 (sources) and infinity into distances to the
// nearest source, in cells (two pass chamfer, with diagonal steps of √2:
// about euclidean).
func chamfer(dist []float64, width, height int) {
	const straight, diagonal = 1, math.Sqrt2
	at := func(x, y int) float64 {
		if x < 0 || y < 0 || x >= width || y >= height {
			return math.Inf(1)
		}
		return dist[y*width+x]
	}
	for y := range height {
		for x := range width {
			i := y*width + x
			dist[i] = min(dist[i], at(x-1, y)+straight, at(x, y-1)+straight, at(x-1, y-1)+diagonal, at(x+1, y-1)+diagonal)
		}
	}
	for y := height - 1; y >= 0; y-- {
		for x := width - 1; x >= 0; x-- {
			i := y*width + x
			dist[i] = min(dist[i], at(x+1, y)+straight, at(x, y+1)+straight, at(x+1, y+1)+diagonal, at(x-1, y+1)+diagonal)
		}
	}
}

// regionAt is the region at a position, -1 if none.
func (f *upliftField) regionAt(p geom.Vec2) int32 {
	x := min(max(int(p.X/f.cellSize), 0), f.width-1)
	y := min(max(int(p.Y/f.cellSize), 0), f.height-1)
	return f.region[y*f.width+x]
}

// initialRates are the uplift rates of the regions before calibration: a
// first guess from their target height (summits at about 1500 m per mm/yr),
// 0 for terrains without height.
func (f *upliftField) initialRates() []float64 {
	rates := make([]float64, len(f.regionTerrain))
	for region, t := range f.regionTerrain {
		if t.Calibrated() {
			rates[region] = geom.Clamp(t.Height/1500, minUplift, maxUplift)
		}
	}
	return rates
}

// sampler returns the uplift (mm per year) at a position, for the given
// rates of the regions (copied).
func (f *upliftField) sampler(rates []float64) func(geom.Vec2) float64 {
	rates = slices.Clone(rates)
	cellUplift := func(x, y int) float64 {
		x, y = min(max(x, 0), f.width-1), min(max(y, 0), f.height-1)
		i := y*f.width + x
		if f.region[i] < 0 {
			return 0
		}
		return rates[f.region[i]] * f.ramp[i]
	}

	return func(p geom.Vec2) float64 {
		// Bilinear, but not across the border of the region: the others'
		// values would leak in
		region := f.regionAt(p)
		if region < 0 {
			return 0
		}
		x, y := p.X/f.cellSize-0.5, p.Y/f.cellSize-0.5
		x0, y0 := int(math.Floor(x)), int(math.Floor(y))
		fx, fy := x-float64(x0), y-float64(y0)

		sum, weight := 0.0, 0.0
		// Corners: x and y offsets, and weight
		for _, corner := range [4][3]float64{{0, 0, (1 - fx) * (1 - fy)}, {1, 0, fx * (1 - fy)}, {0, 1, (1 - fx) * fy}, {1, 1, fx * fy}} {
			cx, cy := min(max(x0+int(corner[0]), 0), f.width-1), min(max(y0+int(corner[1]), 0), f.height-1)
			if f.region[cy*f.width+cx] == region {
				sum += corner[2] * cellUplift(cx, cy)
				weight += corner[2]
			}
		}
		if weight == 0 {
			return rates[region] * rampFloor
		}
		return sum / weight
	}
}

// calibrated tells whether some terrain is calibrated.
func (f *upliftField) calibrated() bool {
	return slices.ContainsFunc(f.regionTerrain, (*config.Terrain).Calibrated)
}

// targets are the target heights of the regions (0 for uncalibrated ones).
func (f *upliftField) targets() []float64 {
	targets := make([]float64, len(f.regionTerrain))
	for region, t := range f.regionTerrain {
		if t.Calibrated() {
			targets[region] = t.Height
		}
	}
	return targets
}

// summits measures the summit height of each region in a simulation: NaN
// for regions too small to measure.
func (f *upliftField) summits(s *simulation) []float64 {
	elevations := make([][]float64, len(f.regionTerrain))
	for v, p := range s.mesh.Points {
		if region := f.regionAt(p); region >= 0 && s.active[v] {
			elevations[region] = append(elevations[region], s.elevation[v])
		}
	}

	summits := make([]float64, len(f.regionTerrain))
	for region, zs := range elevations {
		if len(zs) < minRegionVertices {
			summits[region] = math.NaN()
			continue
		}
		slices.Sort(zs)
		summits[region] = math.Max(1, zs[int(summitQuantile*float64(len(zs)-1))])
	}
	return summits
}

// calibrate adjusts the rates of the regions with a target, and returns the
// simulation of the coarse level with the final rates. It starts from a flat
// land, or from a previous simulation. Each next iteration continues from
// the previous one, for a third of the steps: the landscape is already
// close to settled.
func (r *simulator) calibrate(rates, targets []float64, from *simulation, phase string) *simulation {
	sim := from
	for iteration := range calibrationIterations {
		previous := sim
		sim = r.w.newSimulation(r.w.levels[0], r.field.sampler(rates), r.erodibilityNoise)
		label := fmt.Sprintf("%s, coarse level (%d vertices): iteration %d", phase, len(sim.mesh.Points), iteration+1)
		if previous == nil {
			sim.startFlat(r.w.rng(streamNoise))
			r.run(sim, r.params.Steps, label)
		} else {
			sim.elevation = slices.Clone(previous.elevation)
			r.run(sim, max(1, r.params.Steps/3), label)
		}

		factors, worst := corrections(r.field.summits(sim), rates, targets)
		slog.Debug("calibrating", "iteration", iteration, "regions", len(rates), "worst error", math.Round(worst*1000)/1000)
		if worst < calibrationTolerance || iteration == calibrationIterations-1 {
			break
		}
		for region := range rates {
			rates[region] = geom.Clamp(rates[region]*factors[region], minUplift, maxUplift)
		}
	}

	return sim
}

// corrections returns the factors to scale each region's rate by to reach
// its target from its measured summit (1 for those without target or
// measure), and the worst relative error of the summits.
func corrections(summits, rates, targets []float64) (factors []float64, worst float64) {
	factors = make([]float64, len(rates))
	for region, summit := range summits {
		factors[region] = 1
		if targets[region] <= 0 || math.IsNaN(summit) {
			continue
		}
		factors[region] = math.Pow(geom.Clamp(targets[region]/summit, 1.0/3, 3), calibrationDamping)

		// Regions at the uplift limits can't do better
		limited := (factors[region] > 1 && rates[region] >= maxUplift) || (factors[region] < 1 && rates[region] <= minUplift)
		if !limited {
			worst = math.Max(worst, math.Abs(summit/targets[region]-1))
		}
	}
	return factors, worst
}
