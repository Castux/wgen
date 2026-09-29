package app

import (
	"math"
	"testing"

	"github.com/go-gl/mathgl/mgl64"
)

func TestMarchRay(t *testing.T) {
	// A slope rising along x: z = x / 10, on x in -100..100
	slope := func(x, y float64) float64 {
		if x < -100 || x > 100 {
			return math.NaN()
		}
		return x / 10
	}

	// Straight down onto x = 20
	hit, ok := marchRay(mgl64.Vec3{20, 0, 50}, mgl64.Vec3{20, 0, -50}, -10, 10, slope)
	if !ok || math.Abs(hit.Z()-2) > 1e-3 {
		t.Errorf("down: %v %v, want z 2", hit, ok)
	}

	// Level at z 5, from the left: meets the slope at x = 50
	hit, ok = marchRay(mgl64.Vec3{-200, 0, 5}, mgl64.Vec3{200, 0, 5}, -10, 10, slope)
	if !ok || math.Abs(hit.X()-50) > 0.01 {
		t.Errorf("level: %v %v, want x 50", hit, ok)
	}

	// Above everything
	if _, ok := marchRay(mgl64.Vec3{-200, 0, 20}, mgl64.Vec3{200, 0, 20}, -10, 10, slope); ok {
		t.Error("hit above the surface")
	}
	// Outside of the surface
	if _, ok := marchRay(mgl64.Vec3{150, 0, 50}, mgl64.Vec3{150, 0, -50}, -10, 10, slope); ok {
		t.Error("hit outside of the surface")
	}
}

func TestNiceTicks(t *testing.T) {
	for _, c := range []struct {
		low, high float64
		count     int
		want      []float64
	}{
		{0, 10, 5, []float64{0, 2, 4, 6, 8, 10}},
		{-130, 4600, 5, []float64{0, 1000, 2000, 3000, 4000}},
		{0, 96000, 6, []float64{0, 20000, 40000, 60000, 80000}},
		{-130, 0, 2, []float64{-100, 0}},
		{5, 5, 5, nil},
	} {
		got := niceTicks(c.low, c.high, c.count)
		if len(got) != len(c.want) {
			t.Errorf("niceTicks(%g, %g, %d) = %v, want %v", c.low, c.high, c.count, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("niceTicks(%g, %g, %d) = %v, want %v", c.low, c.high, c.count, got, c.want)
				break
			}
		}
	}
}

func TestFormats(t *testing.T) {
	for meters, want := range map[float64]string{0: "0 m", 950: "950 m", 1234: "1.2 km", 50000: "50 km", 123456: "123 km"} {
		if got := formatDistance(meters); got != want {
			t.Errorf("formatDistance(%g) = %q, want %q", meters, got, want)
		}
	}
	for squareMeters, want := range map[float64]string{5e5: "0.50 km²", 2.5e7: "25.0 km²", 1.2e9: "1200 km²"} {
		if got := formatArea(squareMeters); got != want {
			t.Errorf("formatArea(%g) = %q, want %q", squareMeters, got, want)
		}
	}
}

func TestNoNegativeZero(t *testing.T) {
	for _, tick := range niceTicks(-130, 20, 3) {
		if tick == 0 && math.Signbit(tick) {
			t.Error("negative zero tick")
		}
	}
}

func TestNiceLength(t *testing.T) {
	for limit, want := range map[float64]float64{1: 1, 1.9: 1, 2: 2, 4.99: 2, 7: 5, 150: 100, 260: 200, 999: 500, 0: 0} {
		if got := niceLength(limit); got != want {
			t.Errorf("niceLength(%g) = %g, want %g", limit, got, want)
		}
	}
}
