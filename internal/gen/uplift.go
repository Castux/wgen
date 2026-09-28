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

// The uplift model (see config.UpliftModel): terrains give the rate at which
// the land rises, and the landscape is simulated: rivers erode it following
// the stream power law, hillslopes collapse beyond a critical slope, and the
// sea is the fixed base level. River networks grow into the rising land from
// the shores, which gives them their branching, and valleys and ridges their
// fractal look.
//
// The simulation runs on meshes of increasing resolution: coarse until the
// landscape settles, then each finer mesh starts from the previous result
// and adds detail. Each finer mesh has the points of the coarser one, plus
// new points in between where the terrains ask for more detail.
//
// Elevations are in meters, horizontal positions still in pixels:
// MetersPerPixel converts.

// upliftSpacing is the mesh spacing of a level, in pixels. The last level
// has the configured resolution.
func (w *World) upliftSpacing(level int) float64 {
	return w.Conf.Resolution * math.Pow(2, float64(w.Conf.Levels-level))
}

// detailAt is the number of refinement levels wanted at a position.
func (w *World) detailAt(p geom.Vec2, terrains map[config.Color]*config.Terrain) int {
	if !w.inBounds(p) {
		return 0
	}
	t := terrains[w.pixel(p)]
	switch {
	case t == nil:
		return 0
	case t.Detail != config.DetailAuto:
		return t.Detail
	case t.IsWater():
		return 0
	}
	return w.Conf.Levels
}

// maxDetailAround is the highest detail within radius of p (sampled), so
// that refined regions extend a bit beyond their terrain.
func (w *World) maxDetailAround(p geom.Vec2, radius float64, terrains map[config.Color]*config.Terrain) int {
	d := w.detailAt(p, terrains)
	for i := range 8 {
		a := float64(i) * math.Pi / 4
		q := geom.Vec2{X: p.X + radius*math.Cos(a), Y: p.Y + radius*math.Sin(a)}
		d = max(d, w.detailAt(q, terrains))
	}
	return d
}

// generateMeshes builds the meshes of all levels. Points are on hex
// lattices, each level's lattice containing the previous one: a point is
// added at the first level where it is on the lattice, if the terrain wants
// that much detail, and kept at the finer levels. It is jittered once, when
// added, so that it is at the same position at every level.
func (w *World) generateMeshes() error {
	conf := w.Conf
	levels := conf.Levels
	terrains := conf.TerrainsByColor()
	margin := w.margin()
	seed := w.rng(streamMesh).Uint64()

	type point struct {
		p     geom.Vec2
		level int
	}
	var points []point

	for k := 0; k <= levels; k++ {
		s := w.upliftSpacing(k)
		dy := s * math.Sqrt(3) / 2
		jitter := 0.35 * s

		for j := 0; -margin+float64(j)*dy < float64(w.Height)+margin; j++ {
			y := -margin + float64(j)*dy
			for i := 0; ; i++ {
				x := -margin + float64(i)*s + float64(j%2)*s/2
				if x >= float64(w.Width)+margin {
					break
				}

				// Point (i', j') of the coarser lattice is (2i' + j'%2, 2j')
				// on this one
				if k > 0 && j%2 == 0 && (i-(j/2)%2)%2 == 0 {
					continue
				}

				h := hash2(seed, int64(k)<<32|int64(i), int64(j))
				p := geom.Vec2{
					X: x + (unit(h)*2-1)*jitter,
					Y: y + (unit(hash2(h, 1, 0))*2-1)*jitter,
				}
				if k > 0 && w.maxDetailAround(p, w.upliftSpacing(k-1), terrains) < k {
					continue
				}
				points = append(points, point{p, k})
			}
		}
	}

	w.levels = make([]*mesh.Mesh, levels+1)
	for k := range w.levels {
		var ps []geom.Vec2
		for _, p := range points {
			if p.level <= k {
				ps = append(ps, p.p)
			}
		}
		m, err := mesh.Build(ps)
		if err != nil {
			return err
		}
		w.levels[k] = m
		slog.Debug("uplift mesh", "level", k, "vertices", len(m.Points))
	}

	w.Mesh = w.levels[levels]
	return nil
}

// simState is the simulation on one mesh.
type simState struct {
	m      *mesh.Mesh
	h      []float64 // elevation, meters; NaN outside of the simulation
	active []bool    // land, simulated
	base   []bool    // sea: fixed elevation, where rivers end
	uplift []float64 // meters per year
	k      []float64 // erodibility
	cell   []float64 // cell area, square meters

	mpp  float64 // meters per pixel
	expM float64 // area exponent

	canceled func() bool    // polled every step
	onStep   func(step int) // called after every step

	rec     []int32   // receiver (downhill neighbour), itself if none
	recDist []float64 // meters
	order   []int32   // base level first, every vertex after its receiver
	area    []float64 // drainage area, square meters
}

// simulate runs the uplift model on all levels, and sets the elevation and
// the rivers of the final mesh.
func (w *World) simulate() error {
	u := w.Conf.Simulation
	w.MetersPerPixel = w.Conf.MetersPerPixel(w.Width)
	start := time.Now()
	erodibilityNoise := w.erodibilityNoise()
	snapshots := map[*mesh.Mesh]*World{}

	run := func(s *simState, steps int, phase string) {
		if watch := w.opts.Watch; watch != nil {
			every := max(1, w.opts.WatchSteps)
			s.onStep = func(step int) {
				if (step+1)%every == 0 || step == 0 {
					if p := w.snapshot(s, snapshots); p != nil {
						watch(p, fmt.Sprintf("%s: step %d of %d", phase, step+1, steps))
					}
				}
			}
		}

		t := time.Now()
		s.run(steps, u.TimeStep*1000, math.Tan(u.CriticalSlope*math.Pi/180))
		s.onStep = nil
		slog.Debug("simulated", "phase", phase, "vertices", len(s.m.Points), "steps", steps,
			"took", time.Since(t).Round(time.Millisecond))
	}

	refine := func(coarse *simState, upliftAt func(geom.Vec2) float64, pass string) *simState {
		s := coarse
		for k := 1; k < len(w.levels); k++ {
			prev := s
			s = w.newSimState(w.levels[k], upliftAt, erodibilityNoise)
			s.transfer(prev, hash2(w.Conf.Seed, int64(k), 99))
			run(s, u.RefineSteps, fmt.Sprintf("%sRefining, level %d of %d (%d vertices)", pass, k, len(w.levels)-1, len(s.m.Points)))
		}
		return s
	}

	field := w.newUpliftField()
	rates := field.initialRates()

	var s *simState
	if !field.calibrated() {
		coarse := w.newSimState(w.levels[0], field.sampler(rates), erodibilityNoise)
		coarse.startFlat(w.rng(streamNoise))
		run(coarse, u.Steps, fmt.Sprintf("Coarse level (%d vertices)", len(coarse.m.Points)))
		w.preview(coarse, snapshots)
		s = refine(coarse, field.sampler(rates), "")
	} else {
		// Calibrated on the coarse level, then again for what the
		// refinement adds (see calibrate.go)
		targets := field.targets()
		coarse := w.calibrate(field, rates, targets, nil, erodibilityNoise, run, "Calibrating heights")
		w.preview(coarse, snapshots)
		s = refine(coarse, field.sampler(rates), "First pass, ")

		if len(w.levels) > 1 {
			before, after := field.summits(coarse), field.summits(s)
			for r := range targets {
				if ratio := after[r] / before[r]; targets[r] > 0 && ratio > 0 {
					targets[r] /= geom.Clamp(ratio, 1.0/3, 3)
				}
			}
			coarse = w.calibrate(field, rates, targets, coarse, erodibilityNoise, run, "Calibrating heights again, for the refinement")
			s = refine(coarse, field.sampler(rates), "Second pass, ")
		}
	}

	if w.opts.canceled() {
		return ErrCanceled
	}

	w.setSimResult(s)
	slog.Debug("uplift model", "took", time.Since(start).Round(time.Millisecond))
	return nil
}

// preview hands a world made of the coarse simulation to Options.Preview,
// if there are finer levels to come.
func (w *World) preview(coarse *simState, snapshots map[*mesh.Mesh]*World) {
	if w.opts.Preview == nil || len(w.levels) < 2 || w.opts.canceled() {
		return
	}
	if p := w.snapshot(coarse, snapshots); p != nil {
		w.opts.Preview(p)
	}
}

// snapshot makes a world of a simulation in progress, on its mesh. The
// terrains of each mesh are assigned once, in cache.
func (w *World) snapshot(s *simState, cache map[*mesh.Mesh]*World) *World {
	base, ok := cache[s.m]
	if !ok {
		p := *w
		p.Mesh = s.m
		p.opts = Options{}
		if err := p.assignTerrainTypes(); err != nil {
			return nil
		}
		base = &p
		cache[s.m] = base
	}

	p := *base
	p.setSimResult(s)
	p.computeWaterDepth()
	p.finalizeElevation()
	p.rasterize()
	return &p
}

func (w *World) newSimState(m *mesh.Mesh, upliftAt, erodibilityNoise func(geom.Vec2) float64) *simState {
	u := w.Conf.Simulation
	n := len(m.Points)
	terrains := w.Conf.TerrainsByColor()
	mpp := w.MetersPerPixel

	s := &simState{
		m: m, mpp: mpp, expM: u.StreamExponent,
		h:       nanSlice(n),
		active:  make([]bool, n),
		base:    make([]bool, n),
		uplift:  make([]float64, n),
		k:       make([]float64, n),
		cell:    CellAreas(m),
		rec:     make([]int32, n),
		recDist: make([]float64, n),
		area:    make([]float64, n),

		canceled: w.opts.canceled,
	}

	for v, p := range m.Points {
		s.cell[v] *= mpp * mpp
		// As in assignTerrainTypes: the map edge extends a bit
		if !w.inBoundsPlusHalfMargin(p) {
			continue
		}
		t := terrains[w.pixel(p)]
		switch {
		case t == nil:
		case t.Kind == config.Sea:
			s.base[v] = true
			s.h[v] = 0
		case t.Kind == config.Lake:
			// Lakes: flat, eroded down to their outlet
			s.active[v] = true
			s.k[v] = u.Erodibility * t.Erodibility * 100
		default:
			s.active[v] = true
			s.uplift[v] = upliftAt(p) / 1000
			s.k[v] = u.Erodibility * t.Erodibility * erodibilityNoise(p)
		}
	}
	return s
}

// startFlat starts from a flat land, barely above the sea.
func (s *simState) startFlat(r interface{ Float64() float64 }) {
	for v := range s.h {
		if s.active[v] {
			s.h[v] = r.Float64()
		}
	}
}

// erodibilityNoise returns the erodibility multiplier at a position: rocks
// are not all as hard.
func (w *World) erodibilityNoise() func(geom.Vec2) float64 {
	u := w.Conf.Simulation
	if u.ErodibilityNoise == 0 {
		return func(geom.Vec2) float64 { return 1 }
	}
	scale := u.NoiseScale * 1000 / w.MetersPerPixel
	seed := w.rng(streamNoise).Uint64() + 1
	return func(p geom.Vec2) float64 { return 1 + u.ErodibilityNoise*fbm(seed, p.X/scale, p.Y/scale, 4) }
}

// CellAreas returns the area of the Voronoi cell of each vertex, in square
// pixels. Cells on the hull are open, and those next to it can reach far
// out: they are limited to a few times the area of a hexagon of their
// average neighbour distance.
func CellAreas(m *mesh.Mesh) []float64 {
	areas := make([]float64, len(m.Points))
	for v, p := range m.Points {
		sum := 0.0
		for _, n := range m.Neighbours[v] {
			sum += p.Dist(m.Points[n])
		}
		d := sum / float64(max(1, len(m.Neighbours[v])))
		hexagon := math.Sqrt(3) / 2 * d * d

		ts := m.VertexTriangles[v]
		if len(ts) < 3 || len(ts) != len(m.Neighbours[v]) {
			areas[v] = hexagon
			continue
		}
		a := 0.0
		for i, t := range ts {
			c0, c1 := m.Circumcenters[t], m.Circumcenters[ts[(i+1)%len(ts)]]
			a += c0.X*c1.Y - c1.X*c0.Y
		}
		areas[v] = math.Min(math.Abs(a)/2, 3*hexagon)
	}
	return areas
}

// transfer initializes the elevation from the simulation on the coarser
// mesh: vertices it has keep their elevation, new ones get the average of
// their neighbours, plus a little noise to break symmetries.
func (s *simState) transfer(coarse *simState, seed uint64) {
	index := make(map[geom.Vec2]int32, len(coarse.m.Points))
	for v, p := range coarse.m.Points {
		index[p] = int32(v)
	}

	h := nanSlice(len(s.h))
	known := make([]bool, len(s.h))
	var queue []int32
	for v, p := range s.m.Points {
		if i, ok := index[p]; ok && !math.IsNaN(coarse.h[i]) {
			h[v], known[v] = coarse.h[i], true
		}
	}
	for v := range s.h {
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
			for _, n := range s.m.Neighbours[v] {
				if known[n] {
					sum += h[n]
					count++
				}
			}
			if count == 0 {
				later = append(later, v)
				continue
			}
			h[v] = sum/float64(count) + (unit(hash2(seed, int64(v), 0))*2-1)*2
			known[v] = true
			assigned = true
		}
		if !assigned {
			break
		}
		queue = later
	}

	for v := range s.h {
		switch {
		case s.base[v]:
		case s.active[v] && !math.IsNaN(h[v]):
			s.h[v] = h[v]
		case s.active[v]:
			s.h[v] = 0
		}
	}
}

// run simulates a number of time steps of dt years. Depressions are filled
// every few steps (and at the end), so that rivers reach the sea.
func (s *simState) run(steps int, dt, criticalSlope float64) {
	const fillEvery = 10
	for step := range steps {
		if s.canceled != nil && s.canceled() {
			return
		}
		if step%fillEvery == 0 {
			s.fill()
		}
		s.route()
		s.erode(dt)
		s.collapse(criticalSlope)
		if s.onStep != nil {
			s.onStep(step)
		}
	}
	s.fill()
	s.route()
}

// fill raises depressions to their outlet, plus a tiny slope so that they
// drain (priority flood, Barnes et al. 2014).
func (s *simState) fill() {
	const epsilon = 0.01 // meters
	visited := make([]bool, len(s.h))
	q := &vertexQueue{z: s.h}
	for v := range s.h {
		if s.base[v] {
			visited[v] = true
			q.push(int32(v))
		}
	}

	for q.Len() > 0 {
		c, _ := q.pop()
		for _, n := range s.m.Neighbours[c] {
			if visited[n] || !s.active[n] {
				continue
			}
			visited[n] = true
			if s.h[n] < s.h[c]+epsilon {
				s.h[n] = s.h[c] + epsilon
			}
			q.push(n)
		}
	}
}

// route computes receivers (steepest descent), the order from the base level
// up, and drainage areas.
func (s *simState) route() {
	m := s.m
	n := len(s.h)

	parallelRows(n, func(v0, v1 int) {
		for v := v0; v < v1; v++ {
			s.rec[v] = int32(v)
			if !s.active[v] {
				continue
			}
			best := 0.0
			for _, u := range m.Neighbours[v] {
				if !s.active[u] && !s.base[u] {
					continue
				}
				d := m.Points[v].Dist(m.Points[u]) * s.mpp
				if slope := (s.h[v] - s.h[u]) / d; slope > best {
					best, s.rec[v], s.recDist[v] = slope, u, d
				}
			}
		}
	})

	// Donors of each vertex, compressed
	start := make([]int32, n+1)
	for v := range n {
		if r := s.rec[v]; r != int32(v) {
			start[r+1]++
		}
	}
	for v := range n {
		start[v+1] += start[v]
	}
	donors := make([]int32, start[n])
	fillAt := make([]int32, n)
	copy(fillAt, start[:n])
	for v := range n {
		if r := s.rec[v]; r != int32(v) {
			donors[fillAt[r]] = int32(v)
			fillAt[r]++
		}
	}

	// From the roots (base level, and pits), receivers before donors
	s.order = s.order[:0]
	for v := range n {
		if s.rec[v] == int32(v) && (s.base[v] || s.active[v]) {
			s.order = append(s.order, int32(v))
		}
	}
	for i := 0; i < len(s.order); i++ {
		v := s.order[i]
		s.order = append(s.order, donors[start[v]:start[v+1]]...)
	}

	for _, v := range s.order {
		s.area[v] = 0
		if s.active[v] {
			s.area[v] = s.cell[v]
		}
	}
	for i := len(s.order) - 1; i >= 0; i-- {
		v := s.order[i]
		if r := s.rec[v]; r != v {
			s.area[r] += s.area[v]
		}
	}
}

// erode applies uplift and stream power erosion (n = 1), implicitly: each
// vertex is solved after its receiver, which makes any time step stable.
func (s *simState) erode(dt float64) {
	for _, v := range s.order {
		if !s.active[v] {
			continue
		}
		r := s.rec[v]
		if r == v {
			s.h[v] += s.uplift[v] * dt
			continue
		}
		f := s.k[v] * dt * math.Pow(s.area[v], s.expM) / s.recDist[v]
		s.h[v] = (s.h[v] + s.uplift[v]*dt + f*s.h[r]) / (1 + f)
	}
}

// collapse limits slopes to the critical slope: steeper hillslopes slide.
func (s *simState) collapse(criticalSlope float64) {
	for _, v := range s.order {
		r := s.rec[v]
		if !s.active[v] || r == v {
			continue
		}
		if limit := s.h[r] + criticalSlope*s.recDist[v]; s.h[v] > limit {
			s.h[v] = limit
		}
	}
}

// setSimResult sets the world's elevation and rivers from the simulation of
// its final mesh.
func (w *World) setSimResult(s *simState) {
	n := len(s.h)
	w.Z = make([]float64, n)
	w.Downhill = make([]int32, n)
	w.Flow = make([]int32, n)
	w.Drainage = make([]float64, n)

	for v := range n {
		w.Z[v] = s.h[v]
		w.Downhill[v] = -1
		if r := s.rec[v]; s.active[v] && r != int32(v) && (s.active[r] || s.base[r]) {
			w.Downhill[v] = s.rec[v]
		}
		w.Drainage[v] = s.area[v] / (s.mpp * s.mpp)
		if s.active[v] || s.base[v] {
			w.Flow[v] = 1
		}
	}

	for i := len(s.order) - 1; i >= 0; i-- {
		v := s.order[i]
		if r := s.rec[v]; r != v {
			w.Flow[r] += w.Flow[v]
		}
	}
}
