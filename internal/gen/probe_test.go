package gen

import (
	"math"
	"testing"

	"github.com/Castux/wgen/internal/config"
	"github.com/Castux/wgen/internal/geom"
)

func TestProbe(t *testing.T) {
	w := generate(t, setup(t))
	probe := NewProbe(w)

	// Positions in map pixels, y up: the test island is 200 px
	mountain := probe.At(geom.Vec2{X: 120, Y: 120})
	if !mountain.Inside || mountain.Terrain == nil || mountain.Terrain.Name != "mountains" {
		t.Fatalf("mountain point: %+v", mountain)
	}
	if mountain.Water || mountain.Ground <= 0 {
		t.Errorf("mountain above sea level, dry: %+v", mountain)
	}
	if mountain.Drainage <= 0 {
		t.Errorf("mountain drainage: %g", mountain.Drainage)
	}

	sea := probe.At(geom.Vec2{X: 5, Y: 5})
	if sea.Terrain == nil || sea.Terrain.Kind != config.Sea || !sea.Water || sea.Surface != 0 || sea.Depth <= 0 {
		t.Errorf("sea point: %+v", sea)
	}

	lake := probe.At(geom.Vec2{X: 70, Y: 80})
	if lake.Terrain == nil || lake.Terrain.Kind != config.Lake || !lake.Water || lake.Surface <= 0 {
		t.Errorf("lake point: %+v", lake)
	}

	if outside := probe.At(geom.Vec2{X: -1, Y: 10}); outside.Inside {
		t.Error("point outside of the map is inside")
	}
}

func TestProfile(t *testing.T) {
	w := generate(t, setup(t))
	probe := NewProbe(w)

	// From the sea, over the mountain, to the sea
	path := []geom.Vec2{{X: 2, Y: 120}, {X: 120, Y: 120}, {X: 198, Y: 120}}
	profile := probe.Profile(path)

	if want := 196 * w.MetersPerPixel; math.Abs(profile.Length-want) > 1e-6*want {
		t.Errorf("length %g m, want %g", profile.Length, want)
	}
	if len(profile.Samples) < 100 {
		t.Fatalf("%d samples", len(profile.Samples))
	}
	first, last := profile.Samples[0], profile.Samples[len(profile.Samples)-1]
	if first.Distance != 0 || math.Abs(last.Distance-profile.Length) > 1e-6*profile.Length {
		t.Errorf("distances from %g to %g, length %g", first.Distance, last.Distance, profile.Length)
	}
	if !first.Water || !last.Water || !profile.HasWater || profile.Start != 0 || profile.End != 0 {
		t.Errorf("ends at sea level: %+v, %+v", first, last)
	}
	if profile.Highest < probe.At(geom.Vec2{X: 120, Y: 120}).Ground || profile.Lowest >= 0 {
		t.Errorf("range %g..%g", profile.Lowest, profile.Highest)
	}
	// Up from the sea and down again, on the surface
	if profile.Ascent < profile.Highest || math.Abs(profile.Ascent-profile.Descent) > 1e-6 {
		t.Errorf("ascent %g, descent %g, highest %g", profile.Ascent, profile.Descent, profile.Highest)
	}
	for i := 1; i < len(profile.Samples); i++ {
		if profile.Samples[i].Distance <= profile.Samples[i-1].Distance {
			t.Fatalf("distances not increasing at %d", i)
		}
	}

	if empty := probe.Profile(path[:1]); len(empty.Samples) != 0 || empty.Length != 0 {
		t.Errorf("profile of a point: %+v", empty)
	}
}

// Along the whole profile, water is above the ground: none of the shore
// triangles' missing water levels
func TestProfileShores(t *testing.T) {
	w := generate(t, setup(t))
	probe := NewProbe(w)
	profile := probe.Profile([]geom.Vec2{{X: 2, Y: 100}, {X: 198, Y: 100}, {X: 100, Y: 198}, {X: 100, Y: 2}})
	for _, s := range profile.Samples {
		if s.Surface < s.Ground || (s.Water && s.Surface <= s.Ground) {
			t.Fatalf("at %.0f m: surface %g, ground %g, water %v", s.Distance, s.Surface, s.Ground, s.Water)
		}
		if s.Surface < -1 && !s.Water {
			t.Fatalf("at %.0f m: dry below sea level, %g", s.Distance, s.Surface)
		}
	}
}
