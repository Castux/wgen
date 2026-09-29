package app

import (
	"math"
	"testing"

	"github.com/Castux/wgen/internal/gen"
	"github.com/Castux/wgen/internal/geom"
)

func TestGroundLayers(t *testing.T) {
	w := island(t)
	probe := gen.NewProbe(w)
	center := geom.Vec2{X: float64(w.Width) / 2, Y: float64(w.Height) / 2}

	outer := buildGrid(probe, w, center, 32, 64, probe.Surface)
	// The grid, and its skirt: 4 vertices and 12 indices per edge segment
	edges := 4 * (gridSize - 1)
	if len(outer.indices) != 6*(gridSize-1)*(gridSize-1)+12*edges || len(outer.vertices) != gridSize*gridSize+4*edges {
		t.Fatalf("grid of %d vertices, %d indices", len(outer.vertices), len(outer.indices))
	}

	// At its edges, a grid is on its parent's ground: no seams
	for k := 0; k < gridSize; k += 16 {
		for _, position := range []geom.Vec2{
			{X: outer.bounds[0] + float64(k)*outer.step, Y: outer.bounds[1]},
			{X: outer.bounds[0], Y: outer.bounds[1] + float64(k)*outer.step},
		} {
			z, ok := outer.heightAt(position)
			if want := probe.Surface(position); ok && !math.IsNaN(want) && math.Abs(z-want) > 1e-6 {
				t.Fatalf("edge at %v: %g, parent %g", position, z, want)
			}
		}
	}

	inner := buildGrid(probe, w, center, 4, 8, func(p geom.Vec2) float64 {
		z, _ := outer.heightAt(p)
		return z
	})
	if inner.size >= outer.size || !outer.inside(geom.Vec2{X: inner.bounds[0], Y: inner.bounds[1]}) {
		t.Fatalf("inner grid %v not in the outer %v", inner.bounds, outer.bounds)
	}

	blocks := buildBlocks(probe, w, center, inner)
	for i, top := range blocks.tops {
		if top != math.Round(top) {
			t.Fatalf("block %d top %g not on a meter", i, top)
		}
	}
	if len(blocks.indices) < 6*blocksSize*blocksSize || len(blocks.indices)%6 != 0 {
		t.Errorf("%d block indices", len(blocks.indices))
	}
	// The block under a point is the one heightAt tells
	position := geom.Vec2{X: blocks.bounds[0] + 10.5*blocks.step, Y: blocks.bounds[1] + 20.5*blocks.step}
	if z, ok := blocks.heightAt(position); !ok || z != blocks.tops[20*blocksSize+10] {
		t.Errorf("block height %g %v, top %g", z, ok, blocks.tops[20*blocksSize+10])
	}
}
