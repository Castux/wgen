// Package gen turns a map image, whose pixel colors are terrains, and a
// config into a landscape: elevation (in meters) and rivers.
//
// The landscape is simulated: the land rises, at rates found so that each
// terrain reaches its target summit height, rivers erode it following the
// stream power law, hillslopes collapse beyond a critical slope, and the sea
// stays at its level (see uplift.go and calibrate.go). River networks grow
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
	"time"

	"github.com/Castux/wgen/internal/config"
	"github.com/Castux/wgen/internal/geom"
	"github.com/Castux/wgen/internal/mesh"
)

type Stage int

const (
	StageImage   Stage = iota // load the map image
	StageMesh                 // triangulate
	StageTerrain              // assign terrains, simulate, water depth
	StageRaster               // rasterize
	StageNone
)

var stageNames = [...]string{"image", "mesh", "terrain", "raster", "none"}

func (s Stage) String() string { return stageNames[s] }

type World struct {
	Conf *config.Config

	// Map image, bottom row first (world y goes up)
	Width, Height int
	Outline       []config.Color

	Mesh *mesh.Mesh

	// The meshes of all levels, from the coarsest: the last one is Mesh
	levels []*mesh.Mesh

	// Meters per pixel: elevations are in meters, positions in pixels
	MetersPerPixel float64

	// Per vertex data. Terrain is nil for vertices outside the map.
	Terrain    []*config.Terrain
	Shore      []bool // water next to land
	Shores     []int32
	Z          []float64
	WaterLevel []float64
	Downhill   []int32   // -1 if none
	Flow       []int32   // vertices upstream, itself included
	Drainage   []float64 // area upstream, itself included, square pixels

	Lowest, Highest float64

	// Rasterized, Width * Height, bottom row first
	Heightmap []float64
	WaterMap  []float64

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
		return StageTerrain
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
	stage := ChangedStage(w.Conf, conf)
	n, err := w.run(conf, stage, opts)
	return n, stage, err
}

// ReloadImage reloads the map image and reruns the pipeline.
func (w *World) ReloadImage() (*World, Stage, error) {
	return w.ReloadImageWith(Options{})
}

// ReloadImageWith is ReloadImage, with options.
func (w *World) ReloadImageWith(opts Options) (*World, Stage, error) {
	n := *w
	if err := n.loadOutline(); err != nil {
		return nil, StageImage, err
	}

	stage := n.outlineStage(w)
	res, err := n.run(w.Conf, stage, opts)
	return res, stage, err
}

// WithOutline returns a new world with the given map instead of the image
// file (bottom row first), and the given config.
func (w *World) WithOutline(conf *config.Config, width, height int, outline []config.Color, opts Options) (*World, Stage, error) {
	if len(outline) != width*height {
		return nil, StageImage, fmt.Errorf("map of %d pixels for %dx%d", len(outline), width, height)
	}

	n := *w
	n.Width, n.Height, n.Outline = width, height, outline

	stage := StageMesh
	if w.Conf != nil {
		// Not the image stage: it would load the file
		stage = max(StageMesh, min(ChangedStage(w.Conf, conf), n.outlineStage(w)))
	}
	res, err := n.run(conf, stage, opts)
	return res, stage, err
}

// Rerun generates the world again from the simulation, with the same
// config: to watch it again.
func (w *World) Rerun(opts Options) (*World, Stage, error) {
	if w.Conf == nil || w.Mesh == nil {
		return nil, StageImage, errors.New("nothing generated yet")
	}
	n, err := w.run(w.Conf, StageTerrain, opts)
	return n, StageTerrain, err
}

// outlineStage is the first stage to rerun when the map changed from the
// old world's.
func (w *World) outlineStage(old *World) Stage {
	if w.Width != old.Width || w.Height != old.Height || old.Mesh == nil || w.Conf.Levels > 0 {
		// The mesh is refined according to the terrains
		return StageMesh
	}
	return StageTerrain
}

func (w *World) run(conf *config.Config, from Stage, opts Options) (*World, error) {
	if err := conf.Validate(); err != nil {
		return nil, err
	}

	n := *w
	n.Conf = conf
	n.opts = opts
	defer func() { n.opts = Options{} }()
	start := time.Now()

	steps := []struct {
		stage Stage
		name  string
		f     func() error
	}{
		{StageImage, "loading image", n.loadOutline},
		{StageMesh, "triangulating", n.generateMeshes},
		{StageTerrain, "assigning terrain types", n.assignTerrainTypes},
		{StageTerrain, "simulating", n.simulate},
		{StageTerrain, "computing water depth", func() error { n.computeWaterDepth(); n.finalizeElevation(); return nil }},
		{StageRaster, "rasterizing", func() error { n.rasterize(); return nil }},
	}

	for _, step := range steps {
		if step.stage < from {
			continue
		}

		if opts.canceled() {
			return nil, ErrCanceled
		}

		t := time.Now()
		if err := step.f(); err != nil {
			return nil, err
		}
		slog.Debug(step.name, "took", time.Since(t).Round(time.Millisecond))
	}

	if from < StageNone {
		slog.Info("generated",
			"from", from.String(),
			"vertices", len(n.Mesh.Points),
			"range", fmt.Sprintf("%.0f..%.0f m", n.Lowest, n.Highest),
			"took", time.Since(start).Round(time.Millisecond))
	}

	return &n, nil
}

// margin around the map covered by the mesh, pixels.
func (w *World) margin() float64 { return w.upliftSpacing(0) }

func (w *World) inBounds(p geom.Vec2) bool {
	return p.X >= 0 && p.X < float64(w.Width) && p.Y >= 0 && p.Y < float64(w.Height)
}

func (w *World) inBoundsPlusHalfMargin(p geom.Vec2) bool {
	h := w.margin() / 2
	return p.X > -h && p.X < float64(w.Width)+h && p.Y > -h && p.Y < float64(w.Height)+h
}

// pixel returns the map color at p, clamped to the image.
func (w *World) pixel(p geom.Vec2) config.Color {
	row := int(geom.Clamp(p.Y, 0, float64(w.Height-1)))
	col := int(geom.Clamp(p.X, 0, float64(w.Width-1)))
	return w.Outline[row*w.Width+col]
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
