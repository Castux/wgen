// Package gen turns a map image, whose pixel colors are terrains, and a
// config into a landscape: elevation (in meters) and rivers.
//
// The landscape is simulated: the land rises, at rates found so that each
// terrain reaches its target summit height, rivers erode it following the
// stream power law, hillslopes collapse beyond a critical slope, and the sea
// stays at its level (see simulation.go and calibrate.go). River networks grow
// into the rising land from the coasts, which gives branching valleys and
// winding ridges.
//
// The pipeline is split in stages, and a config change only reruns the stages
// it affects. A World is an immutable snapshot: updating it returns a new
// World that shares the unchanged data with the old one.
package gen

import (
	"errors"
	"fmt"
	"log/slog"
	"math"
	"math/rand/v2"
	"time"

	"github.com/Castux/wgen/internal/config"
	"github.com/Castux/wgen/internal/geom"
	"github.com/Castux/wgen/internal/mesh"
)

type Stage int

const (
	StageImage      Stage = iota // load the map image
	StageMesh                    // build the meshes of all levels
	StageSimulation              // assign terrains, simulate, water depth
	StageRaster                  // rasterize
	StageNone
)

var stageNames = [...]string{"image", "mesh", "terrain", "raster", "none"}

func (s Stage) String() string { return stageNames[s] }

// World is a generated world, immutable once returned.
type World struct {
	Config *config.Config

	// Map image, bottom row first (world y goes up)
	Width, Height int
	Map           []config.Color

	Mesh *mesh.Mesh

	// The meshes of all levels, from the coarsest: the last one is Mesh
	levels []*mesh.Mesh

	// Meters per pixel: elevations are in meters, positions in pixels
	MetersPerPixel float64

	// Per vertex data. Terrain is nil for vertices outside the map.
	Terrain    []*config.Terrain
	Shore      []bool // water next to land
	Shores     []int32
	Elevation  []float64
	WaterLevel []float64
	Downhill   []int32   // -1 if none
	Drainage   []float64 // area upstream, itself included, square pixels

	Lowest, Highest float64

	// Rasterized, Width * Height, bottom row first
	Heightmap []float64
	WaterMap  []float64

	// The last generation: its first stage, and how long it took
	From Stage
	Took time.Duration

	opts Options // of the generation in progress
}

// Options of a generation.
type Options struct {
	// Canceled is polled during the generation, which stops with
	// ErrCanceled when it returns true.
	Canceled func() bool

	// Preview is called with a coarse version of the result, before
	// refining it. It must not keep the world past the generation if that
	// fails.
	Preview func(*World)

	// Watch, if set, is called during the simulation with the world as it
	// is, every WatchSteps time steps, and a description of what is being
	// done: to see the process.
	Watch      func(w *World, progress string)
	WatchSteps int
}

// ErrCanceled is returned by generations stopped by Options.Canceled.
var ErrCanceled = errors.New("generation canceled")

func (o Options) canceled() bool { return o.Canceled != nil && o.Canceled() }

// New runs the full pipeline for a config.
func New(conf *config.Config) (*World, error) {
	w, _, err := (&World{}).Update(conf)
	return w, err
}

// ChangedStage returns the first stage that must be rerun when going from the
// old config to the new one. The mesh depends on the terrains (their
// details), the simulation on everything else.
func ChangedStage(old, conf *config.Config) Stage {
	switch {
	case old == nil || conf.ImagePath() != old.ImagePath():
		return StageImage

	case conf.Resolution != old.Resolution ||
		conf.Seed != old.Seed ||
		conf.Levels != old.Levels ||
		!config.TerrainMeshesEqual(conf, old):
		return StageMesh

	case !config.TerrainsEqual(conf, old) ||
		conf.Simulation != old.Simulation ||
		conf.MapWidth != old.MapWidth:
		return StageSimulation
	}

	return StageNone
}

// Update returns a new world for the new config, rerunning only the needed
// stages. The receiver is left untouched. If nothing needs recomputing, the
// returned world shares everything but the config with the old one.
func (w *World) Update(conf *config.Config) (*World, Stage, error) {
	return w.UpdateWith(conf, Options{})
}

// UpdateWith is Update, with options.
func (w *World) UpdateWith(conf *config.Config, opts Options) (*World, Stage, error) {
	stage := ChangedStage(w.Config, conf)
	next, err := w.run(conf, stage, opts)
	return next, stage, err
}

// ReloadImage reloads the map image and reruns the pipeline.
func (w *World) ReloadImage() (*World, Stage, error) {
	return w.ReloadImageWith(Options{})
}

// ReloadImageWith is ReloadImage, with options.
func (w *World) ReloadImageWith(opts Options) (*World, Stage, error) {
	next := *w
	if err := next.loadMap(); err != nil {
		return nil, StageImage, err
	}

	stage := next.mapStage(w)
	result, err := next.run(w.Config, stage, opts)
	return result, stage, err
}

// WithMap returns a new world with the given map instead of the image
// file (bottom row first), and the given config.
func (w *World) WithMap(conf *config.Config, width, height int, paintedMap []config.Color, opts Options) (*World, Stage, error) {
	if len(paintedMap) != width*height {
		return nil, StageImage, fmt.Errorf("map of %d pixels for %dx%d", len(paintedMap), width, height)
	}

	next := *w
	next.Width, next.Height, next.Map = width, height, paintedMap

	stage := StageMesh
	if w.Config != nil {
		// Not the image stage: it would load the file
		stage = max(StageMesh, min(ChangedStage(w.Config, conf), next.mapStage(w)))
	}
	result, err := next.run(conf, stage, opts)
	return result, stage, err
}

// Rerun generates the world again from the simulation, with the same
// config: to watch it again.
func (w *World) Rerun(opts Options) (*World, Stage, error) {
	if w.Config == nil || w.Mesh == nil {
		return nil, StageImage, errors.New("nothing generated yet")
	}
	next, err := w.run(w.Config, StageSimulation, opts)
	return next, StageSimulation, err
}

// mapStage is the first stage to rerun when the map changed from the
// old world's.
func (w *World) mapStage(old *World) Stage {
	if w.Width != old.Width || w.Height != old.Height || old.Mesh == nil || w.Config.Levels > 0 {
		// The mesh is refined according to the terrains
		return StageMesh
	}
	return StageSimulation
}

// run returns a new world for the config, rerunning the stages from the
// given one.
func (w *World) run(conf *config.Config, from Stage, opts Options) (*World, error) {
	if err := conf.Validate(); err != nil {
		return nil, err
	}

	next := *w
	next.Config = conf
	next.opts = opts
	defer func() { next.opts = Options{} }()
	start := time.Now()

	steps := []struct {
		stage Stage
		name  string
		run   func() error
	}{
		{StageImage, "loading image", next.loadMap},
		{StageMesh, "building meshes", next.generateMeshes},
		{StageSimulation, "assigning terrains", next.assignTerrains},
		{StageSimulation, "simulating", next.simulate},
		{StageSimulation, "computing water depth", func() error {
			next.computeWaterDepth()
			next.fillMargin()
			next.computeElevationRange()
			return nil
		}},
		{StageRaster, "rasterizing", func() error { next.rasterize(); return nil }},
	}

	for _, step := range steps {
		if step.stage < from {
			continue
		}

		if opts.canceled() {
			return nil, ErrCanceled
		}

		stepStart := time.Now()
		if err := step.run(); err != nil {
			return nil, err
		}
		slog.Debug(step.name, "took", time.Since(stepStart).Round(time.Millisecond))
	}

	next.From, next.Took = from, time.Since(start)
	if from < StageNone {
		slog.Info("generated",
			"from", from.String(),
			"vertices", len(next.Mesh.Points),
			"range", fmt.Sprintf("%.0f..%.0f m", next.Lowest, next.Highest),
			"took", time.Since(start).Round(time.Millisecond))
	}

	return &next, nil
}

// margin around the map covered by the mesh, pixels.
func (w *World) margin() float64 { return w.levelSpacing(0) }

func (w *World) inBounds(p geom.Vec2) bool {
	return p.X >= 0 && p.X < float64(w.Width) && p.Y >= 0 && p.Y < float64(w.Height)
}

func (w *World) inBoundsPlusHalfMargin(p geom.Vec2) bool {
	halfMargin := w.margin() / 2
	return p.X > -halfMargin && p.X < float64(w.Width)+halfMargin && p.Y > -halfMargin && p.Y < float64(w.Height)+halfMargin
}

// pixel returns the map color at p, clamped to the image.
func (w *World) pixel(p geom.Vec2) config.Color {
	row := int(geom.Clamp(p.Y, 0, float64(w.Height-1)))
	col := int(geom.Clamp(p.X, 0, float64(w.Width-1)))
	return w.Map[row*w.Width+col]
}

// Random streams, so that each stage's randomness only depends on the seed
const (
	streamMesh = iota + 1
	streamNoise
)

func (w *World) rng(stream uint64) *rand.Rand {
	return rand.New(rand.NewPCG(w.Config.Seed, stream))
}

// Vertex predicates

func (w *World) IsWater(v int32) bool { return w.Terrain[v] != nil && w.Terrain[v].IsWater() }
func (w *World) IsLand(v int32) bool  { return w.Terrain[v] != nil && !w.Terrain[v].IsWater() }
func (w *World) IsSea(v int32) bool   { return w.Terrain[v] != nil && w.Terrain[v].Kind == config.Sea }
func (w *World) IsLake(v int32) bool  { return w.Terrain[v] != nil && w.Terrain[v].Kind == config.Lake }

func nanSlice(n int) []float64 {
	s := make([]float64, n)
	for i := range s {
		s[i] = math.NaN()
	}
	return s
}

// Outlets is, for every vertex, where its water ends: downhill until there
// is no further (the sea, the edge of the map). Vertices of the same outlet
// are its drainage basin.
func (w *World) Outlets() []int32 {
	const unknown = -1
	outlets := make([]int32, len(w.Downhill))
	for v := range outlets {
		outlets[v] = unknown
	}
	var path []int32
	for v := range outlets {
		// Down to a vertex of known outlet, or to the end
		u := int32(v)
		path = path[:0]
		for outlets[u] == unknown && w.Downhill[u] >= 0 && len(path) <= len(outlets) {
			path = append(path, u)
			u = w.Downhill[u]
		}
		outlet := outlets[u]
		if outlet == unknown {
			outlet = u
			outlets[u] = u
		}
		for _, p := range path {
			outlets[p] = outlet
		}
	}
	return outlets
}
