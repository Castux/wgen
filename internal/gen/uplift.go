package gen

import (
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

func (w *World) upliftModel() bool { return w.Conf.Uplift.Model == config.ModelUplift }

// upliftSpacing is the mesh spacing of a level, in pixels. The last level
// has the configured resolution.
func (w *World) upliftSpacing(level int) float64 {
	return w.Conf.Resolution * math.Pow(2, float64(w.Conf.Uplift.Levels-level))
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
	case t.Gradient < 0:
		return 0
	}
	return w.Conf.Uplift.Levels
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

// generateUpliftMeshes builds the meshes of all levels. Points are on hex
// lattices, each level's lattice containing the previous one: a point is
// added at the first level where it is on the lattice, if the terrain wants
// that much detail, and kept at the finer levels. It is jittered once, when
// added, so that it is at the same position at every level.
func (w *World) generateUpliftMeshes() error {
	conf := w.Conf
	levels := conf.Uplift.Levels
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
		jitter := 0.35 * s * conf.Jitter

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

	rec     []int32   // receiver (downhill neighbour), itself if none
	recDist []float64 // meters
	order   []int32   // base level first, every vertex after its receiver
	area    []float64 // drainage area, square meters
}

// simulate runs the uplift model on all levels, and sets the elevation and
// the rivers of the final mesh.
func (w *World) simulate() error {
	u := w.Conf.Uplift
	start := time.Now()
	upliftAt := w.upliftMap()
	erodibilityNoise := w.erodibilityNoise()

	var s *simState
	for k, m := range w.levels {
		prev := s
		s = w.newSimState(m, upliftAt, erodibilityNoise)

		steps := u.RefineSteps
		if prev == nil {
			// Start from a flat land, barely above the sea
			r := w.rng(streamNoise)
			for v := range s.h {
				if s.active[v] {
					s.h[v] = r.Float64()
				}
			}
			steps = u.Steps
		} else {
			s.transfer(prev, hash2(w.Conf.Seed, int64(k), 99))
		}

		t := time.Now()
		s.run(steps, u.TimeStep*1000, math.Tan(u.CriticalSlope*math.Pi/180))
		slog.Debug("simulated", "level", k, "vertices", len(m.Points), "steps", steps,
			"took", time.Since(t).Round(time.Millisecond))
	}

	w.setSimResult(s)
	slog.Debug("uplift model", "took", time.Since(start).Round(time.Millisecond))
	return nil
}

func (w *World) newSimState(m *mesh.Mesh, upliftAt, erodibilityNoise func(geom.Vec2) float64) *simState {
	u := w.Conf.Uplift
	n := len(m.Points)
	terrains := w.Conf.TerrainsByColor()
	mpp := w.MetersPerPixel

	s := &simState{
		m: m, mpp: mpp, expM: u.StreamM,
		h:       nanSlice(n),
		active:  make([]bool, n),
		base:    make([]bool, n),
		uplift:  make([]float64, n),
		k:       make([]float64, n),
		cell:    CellAreas(m),
		rec:     make([]int32, n),
		recDist: make([]float64, n),
		area:    make([]float64, n),
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
		case t.Gradient < 0 && t.IsSeaTerrain():
			s.base[v] = true
			s.h[v] = t.FixedShore
		case t.Gradient < 0:
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

// upliftMap returns the uplift rate (mm per year) at a position: the
// terrains' uplift, blurred.
func (w *World) upliftMap() func(geom.Vec2) float64 {
	terrains := w.Conf.TerrainsByColor()
	rate := func(p geom.Vec2) float64 {
		if t := terrains[w.pixel(p)]; t != nil && t.Gradient >= 0 {
			return t.Uplift
		}
		return 0
	}

	blur := w.Conf.Uplift.UpliftBlur * 1000 / w.MetersPerPixel // pixels
	if blur < 1 {
		return rate
	}

	// Sampled on a grid, blurred with three box blurs (about a gaussian)
	cell := math.Max(1, blur/4)
	gw, gh := int(math.Ceil(float64(w.Width)/cell)), int(math.Ceil(float64(w.Height)/cell))
	grid := make([]float64, gw*gh)
	for y := range gh {
		for x := range gw {
			p := geom.Vec2{X: math.Min((float64(x)+0.5)*cell, float64(w.Width-1)), Y: math.Min((float64(y)+0.5)*cell, float64(w.Height-1))}
			grid[y*gw+x] = rate(p)
		}
	}
	radius := max(1, int(math.Round(blur/cell/1.7)))
	for range 3 {
		boxBlur(grid, gw, gh, radius)
	}

	return func(p geom.Vec2) float64 {
		x := geom.Clamp(p.X/cell-0.5, 0, float64(gw-1))
		y := geom.Clamp(p.Y/cell-0.5, 0, float64(gh-1))
		x0, y0 := int(x), int(y)
		x1, y1 := min(x0+1, gw-1), min(y0+1, gh-1)
		fx, fy := x-float64(x0), y-float64(y0)
		return geom.Lerp(
			geom.Lerp(grid[y0*gw+x0], grid[y0*gw+x1], fx),
			geom.Lerp(grid[y1*gw+x0], grid[y1*gw+x1], fx),
			fy)
	}
}

// boxBlur blurs a grid in place, horizontally then vertically, clamping at
// the edges.
func boxBlur(grid []float64, width, height, radius int) {
	line := func(get func(int) float64, set func(int, float64), n int) {
		out := make([]float64, n)
		for i := range n {
			sum := 0.0
			for d := -radius; d <= radius; d++ {
				sum += get(min(max(i+d, 0), n-1))
			}
			out[i] = sum / float64(2*radius+1)
		}
		for i, v := range out {
			set(i, v)
		}
	}
	for y := range height {
		line(func(x int) float64 { return grid[y*width+x] }, func(x int, v float64) { grid[y*width+x] = v }, width)
	}
	for x := range width {
		line(func(y int) float64 { return grid[y*width+x] }, func(y int, v float64) { grid[y*width+x] = v }, height)
	}
}

// erodibilityNoise returns the erodibility multiplier at a position: rocks
// are not all as hard.
func (w *World) erodibilityNoise() func(geom.Vec2) float64 {
	u := w.Conf.Uplift
	if u.NoiseAmount == 0 {
		return func(geom.Vec2) float64 { return 1 }
	}
	noise := slopeNoise(config.SlopeNoise{
		Type: config.NoiseFBM, Scale: u.NoiseScale * 1000 / w.MetersPerPixel,
		Amplitude: 1, Octaves: 4, Stretch: 1,
	}, w.rng(streamNoise).Uint64()+1)
	return func(p geom.Vec2) float64 { return 1 + u.NoiseAmount*noise(p) }
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
		if step%fillEvery == 0 {
			s.fill()
		}
		s.route()
		s.erode(dt)
		s.collapse(criticalSlope)
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
