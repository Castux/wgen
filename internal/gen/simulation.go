package gen

import (
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/Castux/wgen/internal/config"
	"github.com/Castux/wgen/internal/geom"
	"github.com/Castux/wgen/internal/mesh"
)

// The landscape is simulated: terrains give the rate at which the land
// rises, rivers erode it following the stream power law, hillslopes collapse
// beyond a critical slope, and the sea is the fixed base level. River
// networks grow into the rising land from the shores, which gives them their
// branching, and valleys and ridges their fractal look.
//
// The simulation runs on the meshes of all levels (see meshes.go): coarse
// until the landscape settles, then each finer mesh starts from the previous
// result and adds detail.
//
// Elevations are in meters, horizontal positions still in pixels:
// MetersPerPixel converts.

// simulation is the simulation on one mesh.
type simulation struct {
	mesh        *mesh.Mesh
	elevation   []float64 // meters; NaN outside of the simulation
	active      []bool    // land and lakes: simulated
	baseLevel   []bool    // sea: fixed elevation, where rivers end
	uplift      []float64 // meters per year
	erodibility []float64
	maxSlope    []float64 // tangent of the critical slope
	cellArea    []float64 // square meters

	metersPerPixel float64
	areaExponent   float64 // m, of the stream power law

	canceled func() bool    // polled every step
	onStep   func(step int) // called after every step

	receiver         []int32   // downhill neighbour, itself if none
	receiverDistance []float64 // meters
	order            []int32   // base level first, every vertex after its receiver
	drainageArea     []float64 // square meters
}

// simulator runs the simulations of a generation, on all levels, and shows
// them as they go.
type simulator struct {
	w                *World
	params           config.Simulation
	field            *upliftField
	erodibilityNoise func(geom.Vec2) float64

	// Worlds of each mesh, with their terrains assigned, for the snapshots
	snapshots map[*mesh.Mesh]*World
}

// simulate runs the simulation on all levels, and sets the elevation and the
// rivers of the final mesh.
func (w *World) simulate() error {
	w.MetersPerPixel = w.Config.MetersPerPixel(w.Width)
	start := time.Now()

	r := &simulator{
		w:                w,
		params:           w.Config.Simulation,
		erodibilityNoise: w.erodibilityNoise(),
		snapshots:        map[*mesh.Mesh]*World{},
	}
	r.field = w.newUpliftField()
	rates := r.field.initialRates()

	var final *simulation
	if r.field.calibrated() {
		final = r.runCalibrated(rates)
	} else {
		final = r.runUncalibrated(rates)
	}

	if w.opts.canceled() {
		return ErrCanceled
	}

	w.setSimulationResult(final)
	slog.Debug("simulation", "took", time.Since(start).Round(time.Millisecond))
	return nil
}

// runUncalibrated simulates the coarse level from a flat land, and refines
// it, with the given rates.
func (r *simulator) runUncalibrated(rates []float64) *simulation {
	coarse := r.w.newSimulation(r.w.levels[0], r.field.sampler(rates), r.erodibilityNoise)
	coarse.startFlat(r.w.rng(streamNoise))
	r.run(coarse, r.params.Steps, fmt.Sprintf("Coarse level (%d vertices)", len(coarse.mesh.Points)))
	r.preview(coarse)
	return r.refine(coarse, rates, "")
}

// runCalibrated calibrates the rates on the coarse level, then again for
// what the refinement adds (see calibrate.go), and refines.
func (r *simulator) runCalibrated(rates []float64) *simulation {
	targets := r.field.targets()
	coarse := r.calibrate(rates, targets, nil, "Calibrating heights")
	r.preview(coarse)
	final := r.refine(coarse, rates, "First pass, ")
	if len(r.w.levels) == 1 {
		return final
	}

	before, after := r.field.summits(coarse), r.field.summits(final)
	for region := range targets {
		if ratio := after[region] / before[region]; targets[region] > 0 && ratio > 0 {
			targets[region] /= geom.Clamp(ratio, 1.0/3, 3)
		}
	}
	coarse = r.calibrate(rates, targets, coarse, "Calibrating heights again, for the refinement")
	return r.refine(coarse, rates, "Second pass, ")
}

// run runs a simulation for a number of steps, showing it to Options.Watch
// if set.
func (r *simulator) run(sim *simulation, steps int, phase string) {
	if watch := r.w.opts.Watch; watch != nil {
		every := max(1, r.w.opts.WatchSteps)
		sim.onStep = func(step int) {
			if (step+1)%every == 0 || step == 0 {
				if snapshot := r.snapshot(sim); snapshot != nil {
					watch(snapshot, fmt.Sprintf("%s: step %d of %d", phase, step+1, steps))
				}
			}
		}
	}

	start := time.Now()
	sim.run(steps, r.params.TimeStep*1000)
	sim.onStep = nil
	slog.Debug("simulated", "phase", phase, "vertices", len(sim.mesh.Points), "steps", steps,
		"took", time.Since(start).Round(time.Millisecond))
}

// refine simulates the finer levels in turn, each starting from the previous
// one, and returns the simulation of the last level.
func (r *simulator) refine(coarse *simulation, rates []float64, pass string) *simulation {
	upliftAt := r.field.sampler(rates)
	sim := coarse
	for k := 1; k < len(r.w.levels); k++ {
		previous := sim
		sim = r.w.newSimulation(r.w.levels[k], upliftAt, r.erodibilityNoise)
		sim.transfer(previous, hash2(r.w.Config.Seed, int64(k), 99))
		r.run(sim, r.params.RefineSteps, fmt.Sprintf("%sRefining, level %d of %d (%d vertices)",
			pass, k, len(r.w.levels)-1, len(sim.mesh.Points)))
	}
	return sim
}

// preview hands a world made of the coarse simulation to Options.Preview,
// if there are finer levels to come.
func (r *simulator) preview(coarse *simulation) {
	w := r.w
	if w.opts.Preview == nil || len(w.levels) < 2 || w.opts.canceled() {
		return
	}
	if snapshot := r.snapshot(coarse); snapshot != nil {
		w.opts.Preview(snapshot)
	}
}

// snapshot makes a world of a simulation in progress, on its mesh. The
// terrains of each mesh are assigned once.
func (r *simulator) snapshot(sim *simulation) *World {
	base, ok := r.snapshots[sim.mesh]
	if !ok {
		world := *r.w
		world.Mesh = sim.mesh
		world.opts = Options{}
		if err := world.assignTerrains(); err != nil {
			return nil
		}
		base = &world
		r.snapshots[sim.mesh] = base
	}

	snapshot := *base
	snapshot.setSimulationResult(sim)
	snapshot.computeWaterDepth()
	snapshot.computeElevationRange()
	snapshot.rasterize()
	return &snapshot
}

// newSimulation prepares a simulation on a mesh: the terrains of its
// vertices, their uplift rates (mm per year, from upliftAt) and
// erodibility. The elevation is left to set.
func (w *World) newSimulation(levelMesh *mesh.Mesh, upliftAt, erodibilityNoise func(geom.Vec2) float64) *simulation {
	params := w.Config.Simulation
	n := len(levelMesh.Points)
	terrains := w.Config.TerrainsByColor()
	metersPerPixel := w.MetersPerPixel

	s := &simulation{
		mesh:             levelMesh,
		metersPerPixel:   metersPerPixel,
		areaExponent:     params.StreamExponent,
		elevation:        nanSlice(n),
		active:           make([]bool, n),
		baseLevel:        make([]bool, n),
		uplift:           make([]float64, n),
		erodibility:      make([]float64, n),
		maxSlope:         make([]float64, n),
		cellArea:         CellAreas(levelMesh),
		receiver:         make([]int32, n),
		receiverDistance: make([]float64, n),
		drainageArea:     make([]float64, n),

		canceled: w.opts.canceled,
	}

	for v, p := range levelMesh.Points {
		s.cellArea[v] *= metersPerPixel * metersPerPixel
		// As in assignTerrains: the map edge extends a bit
		if !w.inBoundsPlusHalfMargin(p) {
			continue
		}
		terrain := terrains[w.pixel(p)]
		if terrain != nil {
			s.maxSlope[v] = math.Tan(terrain.Slope(params) * math.Pi / 180)
		}
		switch {
		case terrain == nil:
		case terrain.Kind == config.Sea:
			s.baseLevel[v] = true
			s.elevation[v] = 0
		case terrain.Kind == config.Lake:
			// Lakes: flat, eroded down to their outlet
			s.active[v] = true
			s.erodibility[v] = params.Erodibility * terrain.Erodibility * 100
		default:
			s.active[v] = true
			s.uplift[v] = upliftAt(p) / 1000
			s.erodibility[v] = params.Erodibility * terrain.Erodibility * erodibilityNoise(p)
		}
	}
	return s
}

// erodibilityNoise returns the erodibility multiplier at a position: rocks
// are not all as hard.
func (w *World) erodibilityNoise() func(geom.Vec2) float64 {
	params := w.Config.Simulation
	if params.ErodibilityNoise == 0 {
		return func(geom.Vec2) float64 { return 1 }
	}
	scale := params.NoiseScale * 1000 / w.MetersPerPixel
	seed := w.rng(streamNoise).Uint64() + 1
	return func(p geom.Vec2) float64 { return 1 + params.ErodibilityNoise*fbm(seed, p.X/scale, p.Y/scale, 4) }
}

// startFlat starts from a flat land, barely above the sea.
func (s *simulation) startFlat(r interface{ Float64() float64 }) {
	for v := range s.elevation {
		if s.active[v] {
			s.elevation[v] = r.Float64()
		}
	}
}

// transfer initializes the elevation from the simulation on the coarser
// mesh.
func (s *simulation) transfer(coarse *simulation, seed uint64) {
	elevation := s.interpolate(coarse, seed)
	for v := range s.elevation {
		switch {
		case s.baseLevel[v]:
		case s.active[v] && !math.IsNaN(elevation[v]):
			s.elevation[v] = elevation[v]
		case s.active[v]:
			s.elevation[v] = 0
		}
	}
}

// interpolate returns the elevations of the coarser simulation on this
// mesh: vertices it has keep their elevation, new ones get the average of
// their neighbours, plus a little noise to break symmetries. NaN where
// unknown.
func (s *simulation) interpolate(coarse *simulation, seed uint64) []float64 {
	coarseIndex := make(map[geom.Vec2]int32, len(coarse.mesh.Points))
	for v, p := range coarse.mesh.Points {
		coarseIndex[p] = int32(v)
	}

	elevation := nanSlice(len(s.elevation))
	known := make([]bool, len(s.elevation))
	for v, p := range s.mesh.Points {
		if i, ok := coarseIndex[p]; ok && !math.IsNaN(coarse.elevation[i]) {
			elevation[v], known[v] = coarse.elevation[i], true
		}
	}
	var queue []int32
	for v := range s.elevation {
		if !known[v] {
			queue = append(queue, int32(v))
		}
	}

	// Breadth first from the known vertices, a few passes for the gaps
	for len(queue) > 0 {
		var later []int32
		assigned := false
		for _, v := range queue {
			sum, count := 0.0, 0
			for _, n := range s.mesh.Neighbours[v] {
				if known[n] {
					sum += elevation[n]
					count++
				}
			}
			if count == 0 {
				later = append(later, v)
				continue
			}
			elevation[v] = sum/float64(count) + (unit(hash2(seed, int64(v), 0))*2-1)*2
			known[v] = true
			assigned = true
		}
		if !assigned {
			break
		}
		queue = later
	}
	return elevation
}

// run simulates a number of time steps of timeStep years. Depressions are
// filled every few steps (and at the end), so that rivers reach the sea.
func (s *simulation) run(steps int, timeStep float64) {
	const fillEvery = 10
	for step := range steps {
		if s.canceled != nil && s.canceled() {
			return
		}
		if step%fillEvery == 0 {
			s.fill()
		}
		s.route()
		s.erode(timeStep)
		s.collapse()
		if s.onStep != nil {
			s.onStep(step)
		}
	}
	s.fill()
	s.route()
}

// fill raises depressions to their outlet, plus a tiny slope so that they
// drain (priority flood, Barnes et al. 2014).
func (s *simulation) fill() {
	const epsilon = 0.01 // meters
	visited := make([]bool, len(s.elevation))
	queue := &vertexQueue{elevation: s.elevation}
	for v := range s.elevation {
		if s.baseLevel[v] {
			visited[v] = true
			queue.push(int32(v))
		}
	}

	for queue.Len() > 0 {
		lowest, _ := queue.pop()
		for _, n := range s.mesh.Neighbours[lowest] {
			if visited[n] || !s.active[n] {
				continue
			}
			visited[n] = true
			if s.elevation[n] < s.elevation[lowest]+epsilon {
				s.elevation[n] = s.elevation[lowest] + epsilon
			}
			queue.push(n)
		}
	}
}

// route computes receivers (steepest descent), the order from the base level
// up, and drainage areas.
func (s *simulation) route() {
	s.findReceivers()
	s.orderFromBaseLevel()
	s.accumulateDrainage()
}

// findReceivers gives every active vertex its steepest downhill neighbour.
func (s *simulation) findReceivers() {
	points, neighbours := s.mesh.Points, s.mesh.Neighbours
	parallelRanges(len(s.elevation), func(start, end int) {
		for v := start; v < end; v++ {
			s.receiver[v] = int32(v)
			if !s.active[v] {
				continue
			}
			steepest := 0.0
			for _, n := range neighbours[v] {
				if !s.active[n] && !s.baseLevel[n] {
					continue
				}
				distance := points[v].Dist(points[n]) * s.metersPerPixel
				if slope := (s.elevation[v] - s.elevation[n]) / distance; slope > steepest {
					steepest, s.receiver[v], s.receiverDistance[v] = slope, n, distance
				}
			}
		}
	})
}

// orderFromBaseLevel orders the vertices from the roots (base level, and
// pits) up, receivers before their donors.
func (s *simulation) orderFromBaseLevel() {
	n := len(s.elevation)

	// Donors of each vertex, compressed: those of v are
	// donors[start[v]:start[v+1]]
	start := make([]int32, n+1)
	for v := range n {
		if r := s.receiver[v]; r != int32(v) {
			start[r+1]++
		}
	}
	for v := range n {
		start[v+1] += start[v]
	}
	donors := make([]int32, start[n])
	next := make([]int32, n)
	copy(next, start[:n])
	for v := range n {
		if r := s.receiver[v]; r != int32(v) {
			donors[next[r]] = int32(v)
			next[r]++
		}
	}

	s.order = s.order[:0]
	for v := range n {
		if s.receiver[v] == int32(v) && (s.baseLevel[v] || s.active[v]) {
			s.order = append(s.order, int32(v))
		}
	}
	for i := 0; i < len(s.order); i++ {
		v := s.order[i]
		s.order = append(s.order, donors[start[v]:start[v+1]]...)
	}
}

// accumulateDrainage sums the cell areas downstream, from the top.
func (s *simulation) accumulateDrainage() {
	for _, v := range s.order {
		s.drainageArea[v] = 0
		if s.active[v] {
			s.drainageArea[v] = s.cellArea[v]
		}
	}
	for i := len(s.order) - 1; i >= 0; i-- {
		v := s.order[i]
		if r := s.receiver[v]; r != v {
			s.drainageArea[r] += s.drainageArea[v]
		}
	}
}

// erode applies uplift and stream power erosion (n = 1) for dt years,
// implicitly: each vertex is solved after its receiver, which makes any time
// step stable.
func (s *simulation) erode(dt float64) {
	for _, v := range s.order {
		if !s.active[v] {
			continue
		}
		r := s.receiver[v]
		if r == v {
			s.elevation[v] += s.uplift[v] * dt
			continue
		}
		f := s.erodibility[v] * dt * math.Pow(s.drainageArea[v], s.areaExponent) / s.receiverDistance[v]
		s.elevation[v] = (s.elevation[v] + s.uplift[v]*dt + f*s.elevation[r]) / (1 + f)
	}
}

// collapse limits slopes to the critical slope: steeper hillslopes slide.
func (s *simulation) collapse() {
	for _, v := range s.order {
		r := s.receiver[v]
		if !s.active[v] || r == v {
			continue
		}
		if limit := s.elevation[r] + s.maxSlope[v]*s.receiverDistance[v]; s.elevation[v] > limit {
			s.elevation[v] = limit
		}
	}
}

// setSimulationResult sets the world's elevation and rivers from the
// simulation of its mesh.
func (w *World) setSimulationResult(s *simulation) {
	n := len(s.elevation)
	w.Elevation = make([]float64, n)
	w.Downhill = make([]int32, n)
	w.Drainage = make([]float64, n)

	for v := range n {
		w.Elevation[v] = s.elevation[v]
		w.Downhill[v] = -1
		if r := s.receiver[v]; s.active[v] && r != int32(v) && (s.active[r] || s.baseLevel[r]) {
			w.Downhill[v] = r
		}
		w.Drainage[v] = s.drainageArea[v] / (s.metersPerPixel * s.metersPerPixel)
	}
}
