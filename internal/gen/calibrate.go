package gen

import (
	"fmt"
	"log/slog"
	"math"
	"slices"

	"github.com/Castux/wgen/internal/config"
	"github.com/Castux/wgen/internal/geom"
)

// The uplift field of the uplift model, and its calibration.
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
// distance to the border of its region with lower terrains.
type upliftField struct {
	cell    float64 // pixels
	gw, gh  int
	label   []int32           // region of each cell, -1 for water or none
	terrain []*config.Terrain // of each region
	ramp    []float64         // uplift factor of each cell, rampFloor..1
}

// rank orders terrains for the ramps: by target height, or by uplift.
func rank(t *config.Terrain) float64 {
	if t.Calibrated() {
		return t.Height
	}
	return t.Uplift * 1500
}

func (w *World) newUpliftField() *upliftField {
	terrains := w.Conf.TerrainsByColor()
	cell := w.upliftSpacing(0) / 2
	f := &upliftField{cell: cell}
	f.gw, f.gh = int(math.Ceil(float64(w.Width)/cell)), int(math.Ceil(float64(w.Height)/cell))
	n := f.gw * f.gh

	terrainAt := make([]*config.Terrain, n)
	for y := range f.gh {
		for x := range f.gw {
			p := geom.Vec2{X: (float64(x) + 0.5) * cell, Y: (float64(y) + 0.5) * cell}
			if t := terrains[w.pixel(p)]; t != nil && t.Gradient >= 0 {
				terrainAt[y*f.gw+x] = t
			}
		}
	}

	// Connected regions of the same terrain, 4-connected
	f.label = make([]int32, n)
	for i := range f.label {
		f.label[i] = -1
	}
	var stack []int
	for start, t := range terrainAt {
		if t == nil || f.label[start] >= 0 {
			continue
		}
		id := int32(len(f.terrain))
		f.terrain = append(f.terrain, t)
		f.label[start] = id
		stack = append(stack[:0], start)

		for len(stack) > 0 {
			i := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			for _, j := range f.neighbours4(i) {
				if terrainAt[j] == t && f.label[j] < 0 {
					f.label[j] = id
					stack = append(stack, j)
				}
			}
		}
	}

	// Distance to lower terrain or water, per rank (chamfer distance)
	f.ramp = make([]float64, n)
	radius := w.Conf.Uplift.UpliftBlur * 1000 / w.MetersPerPixel / cell // cells
	ranks := map[float64]bool{}
	for _, t := range f.terrain {
		ranks[rank(t)] = true
	}
	for r := range ranks {
		dist := make([]float64, n)
		for i, t := range terrainAt {
			if t == nil || rank(t) < r {
				dist[i] = 0
			} else {
				dist[i] = math.Inf(1)
			}
		}
		chamfer(dist, f.gw, f.gh)

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

	return f
}

func (f *upliftField) neighbours4(i int) []int {
	x, y := i%f.gw, i/f.gw
	ns := make([]int, 0, 4)
	if x > 0 {
		ns = append(ns, i-1)
	}
	if x < f.gw-1 {
		ns = append(ns, i+1)
	}
	if y > 0 {
		ns = append(ns, i-f.gw)
	}
	if y < f.gh-1 {
		ns = append(ns, i+f.gw)
	}
	return ns
}

// chamfer turns a grid of 0 (sources) and infinity into distances to the
// nearest source, in cells (two pass 3-4 chamfer, about euclidean).
func chamfer(d []float64, width, height int) {
	const a, b = 1, math.Sqrt2
	at := func(x, y int) float64 {
		if x < 0 || y < 0 || x >= width || y >= height {
			return math.Inf(1)
		}
		return d[y*width+x]
	}
	for y := range height {
		for x := range width {
			i := y*width + x
			d[i] = min(d[i], at(x-1, y)+a, at(x, y-1)+a, at(x-1, y-1)+b, at(x+1, y-1)+b)
		}
	}
	for y := height - 1; y >= 0; y-- {
		for x := width - 1; x >= 0; x-- {
			i := y*width + x
			d[i] = min(d[i], at(x+1, y)+a, at(x, y+1)+a, at(x+1, y+1)+b, at(x-1, y+1)+b)
		}
	}
}

// region is the region at a position, -1 if none.
func (f *upliftField) region(p geom.Vec2) int32 {
	x := min(max(int(p.X/f.cell), 0), f.gw-1)
	y := min(max(int(p.Y/f.cell), 0), f.gh-1)
	return f.label[y*f.gw+x]
}

// initialRates are the uplift rates of the regions before calibration: the
// terrains' uplift, or a first guess from their target height (summits at
// about 1500 m per mm/yr).
func (f *upliftField) initialRates() []float64 {
	rates := make([]float64, len(f.terrain))
	for i, t := range f.terrain {
		rates[i] = t.Uplift
		if t.Calibrated() {
			rates[i] = geom.Clamp(t.Height/1500, minUplift, maxUplift)
		}
	}
	return rates
}

// sampler returns the uplift (mm per year) at a position, for the given
// rates of the regions (copied).
func (f *upliftField) sampler(rates []float64) func(geom.Vec2) float64 {
	rates = slices.Clone(rates)
	value := func(x, y int) float64 {
		x, y = min(max(x, 0), f.gw-1), min(max(y, 0), f.gh-1)
		i := y*f.gw + x
		if f.label[i] < 0 {
			return 0
		}
		return rates[f.label[i]] * f.ramp[i]
	}

	return func(p geom.Vec2) float64 {
		// Bilinear, but not across the border of the region: the others'
		// values would leak in
		r := f.region(p)
		if r < 0 {
			return 0
		}
		x, y := p.X/f.cell-0.5, p.Y/f.cell-0.5
		x0, y0 := int(math.Floor(x)), int(math.Floor(y))
		fx, fy := x-float64(x0), y-float64(y0)

		sum, weight := 0.0, 0.0
		for _, c := range [4][3]float64{{0, 0, (1 - fx) * (1 - fy)}, {1, 0, fx * (1 - fy)}, {0, 1, (1 - fx) * fy}, {1, 1, fx * fy}} {
			cx, cy := min(max(x0+int(c[0]), 0), f.gw-1), min(max(y0+int(c[1]), 0), f.gh-1)
			if f.label[cy*f.gw+cx] == r {
				sum += c[2] * value(cx, cy)
				weight += c[2]
			}
		}
		if weight == 0 {
			return rates[r] * rampFloor
		}
		return sum / weight
	}
}

// calibrated tells whether some terrain is calibrated.
func (f *upliftField) calibrated() bool {
	return slices.ContainsFunc(f.terrain, (*config.Terrain).Calibrated)
}

// targets are the target heights of the regions (0 for uncalibrated ones).
func (f *upliftField) targets() []float64 {
	targets := make([]float64, len(f.terrain))
	for r, t := range f.terrain {
		if t.Calibrated() {
			targets[r] = t.Height
		}
	}
	return targets
}

// summits measures the summit height of each region in a simulation: NaN
// for regions too small to measure.
func (f *upliftField) summits(s *simState) []float64 {
	heights := make([][]float64, len(f.terrain))
	for v, p := range s.m.Points {
		if r := f.region(p); r >= 0 && s.active[v] {
			heights[r] = append(heights[r], s.h[v])
		}
	}

	summits := make([]float64, len(f.terrain))
	for r, hs := range heights {
		if len(hs) < minRegionVertices {
			summits[r] = math.NaN()
			continue
		}
		slices.Sort(hs)
		summits[r] = math.Max(1, hs[int(summitQuantile*float64(len(hs)-1))])
	}
	return summits
}

// calibrate adjusts the rates of the regions with a target, and returns the
// simulation of the coarse level with the final rates. It starts from a flat
// land, or from a previous simulation. Each next iteration continues from
// the previous one, for a third of the steps: the landscape is already
// close to settled.
func (w *World) calibrate(f *upliftField, rates, targets []float64, from *simState, erodibilityNoise func(geom.Vec2) float64,
	run func(*simState, int, string), phase string) *simState {

	s := from
	for it := range calibrationIterations {
		prev := s
		s = w.newSimState(w.levels[0], f.sampler(rates), erodibilityNoise)
		label := fmt.Sprintf("%s, coarse level (%d vertices): iteration %d", phase, len(s.m.Points), it+1)
		if prev == nil {
			s.startFlat(w.rng(streamNoise))
			run(s, w.Conf.Uplift.Steps, label)
		} else {
			s.h = slices.Clone(prev.h)
			run(s, max(1, w.Conf.Uplift.Steps/3), label)
		}

		worst := 0.0
		factors := make([]float64, len(rates))
		for r, summit := range f.summits(s) {
			factors[r] = 1
			if targets[r] <= 0 || math.IsNaN(summit) {
				continue
			}
			factors[r] = math.Pow(geom.Clamp(targets[r]/summit, 1.0/3, 3), calibrationDamping)

			// Regions at the uplift limits can't do better
			if limited := (factors[r] > 1 && rates[r] >= maxUplift) || (factors[r] < 1 && rates[r] <= minUplift); !limited {
				worst = math.Max(worst, math.Abs(summit/targets[r]-1))
			}
		}

		slog.Debug("calibrating", "iteration", it, "regions", len(rates), "worst error", math.Round(worst*1000)/1000)
		if worst < calibrationTolerance || it == calibrationIterations-1 {
			break
		}
		for r := range rates {
			rates[r] = geom.Clamp(rates[r]*factors[r], minUplift, maxUplift)
		}
	}

	return s
}
