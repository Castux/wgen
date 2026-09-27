// Package gen turns a terrain outline image and a config into a heightmap.
//
// Elevation is not simulated from the peaks down: it is built from the sea
// shore up, each terrain type giving the local slope. River flow is then
// computed on that surface, and elevation is recomputed with the slopes along
// rivers reduced, which carves (fake, but natural looking) valleys.
//
// The pipeline is split in stages, and a config change only reruns the stages
// it affects. A World is an immutable snapshot: updating it returns a new
// World that shares the unchanged data with the old one.
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

type Stage int

const (
	StageImage   Stage = iota // load the outline image
	StageMesh                 // triangulate
	StageTerrain              // assign terrains, elevation and rivers
	StageErosion              // erode, water depth
	StageRaster               // rasterize and blur
	StageNone
)

var stageNames = [...]string{"image", "mesh", "terrain", "erosion", "raster", "none"}

func (s Stage) String() string { return stageNames[s] }

type World struct {
	Conf *config.Config

	// Outline image, bottom row first (world y goes up)
	Width, Height int
	Outline       []config.Color

	Mesh *mesh.Mesh

	// Uplift model: the meshes of all levels, the last one is Mesh
	levels []*mesh.Mesh

	// Elevation units per pixel: 1 for the slope model, meters for the
	// uplift model
	MetersPerPixel float64

	// Per vertex data. Terrain is nil for vertices outside the map.
	Terrain    []*config.Terrain
	Gradient   []float64
	Shore      []bool
	Shores     []int32
	Z          []float64
	WaterLevel []float64
	Downhill   []int32 // -1 if none
	Flow       []int32   // vertices upstream, itself included
	Drainage   []float64 // area upstream, itself included, square pixels

	Lowest, Highest float64

	// Rasterized, Width * Height, bottom row first
	Heightmap []float64
	WaterMap  []float64
}

// New runs the full pipeline for a config.
func New(conf *config.Config) (*World, error) {
	w, _, err := (&World{}).Update(conf)
	return w, err
}

// ChangedStage returns the first stage that must be rerun when going from the
// old config to the new one.
func ChangedStage(old, conf *config.Config) Stage {
	if conf.Uplift.Model == config.ModelUplift && (old == nil || old.Uplift.Model == config.ModelUplift) {
		return changedStageUplift(old, conf)
	}

	switch {
	case old == nil || conf.Path != old.Path:
		return StageImage

	case conf.Resolution != old.Resolution ||
		conf.Relax != old.Relax ||
		conf.Grid != old.Grid ||
		conf.Jitter != old.Jitter ||
		conf.Seed != old.Seed ||
		conf.Uplift.Model != old.Uplift.Model:
		return StageMesh

	case !config.TerrainsEqual(conf, old) ||
		conf.SmoothingRadius != old.SmoothingRadius ||
		conf.MaxHeight != old.MaxHeight ||
		conf.Noise != old.Noise:
		return StageTerrain

	case conf.ErosionMinFlow != old.ErosionMinFlow ||
		conf.ErosionFactor != old.ErosionFactor ||
		conf.Erosion != old.Erosion:
		return StageErosion

	case conf.BlurRadius != old.BlurRadius:
		return StageRaster
	}

	return StageNone
}

// changedStageUplift is ChangedStage for the uplift model: the mesh depends
// on the terrains (their details), and the simulation (in the terrain stage)
// does the erosion.
func changedStageUplift(old, conf *config.Config) Stage {
	switch {
	case old == nil || conf.Path != old.Path:
		return StageImage

	case conf.Resolution != old.Resolution ||
		conf.Grid != old.Grid ||
		conf.Jitter != old.Jitter ||
		conf.Seed != old.Seed ||
		conf.Uplift.Levels != old.Uplift.Levels ||
		!config.TerrainMeshesEqual(conf, old):
		return StageMesh

	case !config.TerrainsEqual(conf, old) ||
		conf.Uplift != old.Uplift ||
		conf.MaxHeight != old.MaxHeight:
		return StageTerrain

	case conf.BlurRadius != old.BlurRadius:
		return StageRaster
	}

	return StageNone
}

// Update returns a new world for the new config, rerunning only the needed
// stages. The receiver is left untouched. If nothing needs recomputing, the
// returned world shares everything but the config with the old one.
func (w *World) Update(conf *config.Config) (*World, Stage, error) {
	stage := ChangedStage(w.Conf, conf)
	n, err := w.run(conf, stage)
	return n, stage, err
}

// ReloadImage reloads the outline image and reruns the pipeline. The mesh is
// kept if the image size did not change.
func (w *World) ReloadImage() (*World, Stage, error) {
	n := *w
	if err := n.loadOutline(); err != nil {
		return nil, StageImage, err
	}

	stage := StageMesh
	if n.Width == w.Width && n.Height == w.Height {
		stage = StageTerrain
	}

	res, err := n.run(w.Conf, stage)
	return res, stage, err
}

func (w *World) run(conf *config.Config, from Stage) (*World, error) {
	if err := conf.Validate(); err != nil {
		return nil, err
	}

	n := *w
	n.Conf = conf
	start := time.Now()

	steps := []struct {
		stage Stage
		name  string
		f     func() error
	}{
		{StageImage, "loading image", n.loadOutline},
		{StageMesh, "triangulating", n.generateMesh},
		{StageTerrain, "assigning terrain types", n.assignTerrainTypes},
		{StageTerrain, "computing elevation", n.elevation},
		{StageTerrain, "computing river flow", func() error { n.rivers(); return nil }},
		{StageErosion, "eroding", func() error { n.erosion(); return nil }},
		{StageErosion, "computing water depth", func() error { n.computeWaterDepth(); n.finalizeElevation(); return nil }},
		{StageRaster, "rasterizing", func() error { n.rasterize(); return nil }},
		{StageRaster, "blurring", func() error { n.blurHeightmap(); return nil }},
	}

	for _, step := range steps {
		if step.stage < from {
			continue
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
			"range", fmt.Sprintf("%.2f..%.2f", n.Lowest, n.Highest),
			"took", time.Since(start).Round(time.Millisecond))
	}

	return &n, nil
}

func (w *World) margin() float64 {
	if w.upliftModel() {
		return w.upliftSpacing(0)
	}
	return w.Conf.Resolution * 4
}

func (w *World) inBounds(p geom.Vec2) bool {
	return p.X >= 0 && p.X < float64(w.Width) && p.Y >= 0 && p.Y < float64(w.Height)
}

func (w *World) inBoundsPlusHalfMargin(p geom.Vec2) bool {
	h := w.margin() / 2
	return p.X > -h && p.X < float64(w.Width)+h && p.Y > -h && p.Y < float64(w.Height)+h
}

// pixel returns the outline color at p, clamped to the image.
func (w *World) pixel(p geom.Vec2) config.Color {
	row := int(geom.Clamp(p.Y, 0, float64(w.Height-1)))
	col := int(geom.Clamp(p.X, 0, float64(w.Width-1)))
	return w.Outline[row*w.Width+col]
}

// Vertex predicates

func (w *World) IsWater(v int32) bool { return w.Terrain[v] != nil && w.Gradient[v] < 0 }
func (w *World) IsLand(v int32) bool  { return w.Terrain[v] != nil && w.Gradient[v] >= 0 }
func (w *World) IsSea(v int32) bool   { return w.IsWater(v) && w.Terrain[v].IsSeaTerrain() }
func (w *World) IsLake(v int32) bool  { return w.IsWater(v) && !w.Terrain[v].IsSeaTerrain() }

func nanSlice(n int) []float64 {
	s := make([]float64, n)
	for i := range s {
		s[i] = math.NaN()
	}
	return s
}
