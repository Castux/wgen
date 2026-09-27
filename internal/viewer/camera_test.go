package viewer

import (
	"math"
	"testing"

	"github.com/go-gl/mathgl/mgl64"
)

func near(a, b float64) bool { return math.Abs(a-b) < 1e-6*math.Max(1, math.Abs(b)) }

// project gives the window position (y down) of a world point.
func project(view, projection mgl64.Mat4, p mgl64.Vec3, width, height float64) mgl64.Vec2 {
	clip := projection.Mul4(view).Mul4x1(p.Vec4(1))
	ndc := clip.Vec3().Mul(1 / clip.W())
	return mgl64.Vec2{(ndc.X() + 1) / 2 * width, (1 - ndc.Y()) / 2 * height}
}

func TestOrbitCamera(t *testing.T) {
	var c orbitCamera
	c.reset(2000, 1000)

	if p := c.position(); !p.ApproxEqualThreshold(mgl64.Vec3{0, -1000, 2000}, 1e-9) {
		t.Errorf("initial position %v", p)
	}

	// Poles are not reachable
	c.rotate(0, -1e6, 800)
	if !(c.phi > 0) {
		t.Errorf("phi %v", c.phi)
	}
	c.rotate(0, 1e6, 800)
	if !(c.phi < math.Pi) {
		t.Errorf("phi %v", c.phi)
	}

	// A full turn for a drag of 2 * pi * height / (2 * pi) = height pixels
	c.reset(2000, 1000)
	theta := c.theta
	c.rotate(800, 0, 800)
	if !near(c.theta, theta-2*math.Pi) {
		t.Errorf("theta %v after a full turn, from %v", c.theta, theta)
	}

	// Panning keeps the target under the cursor
	c.reset(2000, 1000)
	const w, h = 1200.0, 800.0
	before := project(c.view(), c.projection(w/h), mgl64.Vec3{}, w, h)
	c.pan(30, -20, h)
	after := project(c.view(), c.projection(w/h), mgl64.Vec3{}, w, h)
	if d := after.Sub(before); !near(d.X(), 30) || !near(d.Y(), -20) {
		t.Errorf("origin moved by %v on screen, expected (30, -20)", d)
	}

	// Dolly is limited
	c.dolly(100)
	if c.radius != c.maxDistance {
		t.Errorf("radius %v, max %v", c.radius, c.maxDistance)
	}
}

func TestTopCamera(t *testing.T) {
	var c topCamera
	c.reset(2000, 1000)
	const w, h = 1000.0, 1000.0

	// The whole map fits
	for _, p := range []mgl64.Vec3{{-1000, -500, 0}, {1000, 500, 0}} {
		s := project(c.view(), c.projection(w/h), p, w, h)
		if s.X() < -1e-6 || s.X() > w+1e-6 || s.Y() < -1e-6 || s.Y() > h+1e-6 {
			t.Errorf("%v is displayed at %v", p, s)
		}
	}

	// Panning follows the mouse
	c.dolly(0.5)
	p := mgl64.Vec3{100, 200, 0}
	before := project(c.view(), c.projection(w/h), p, w, h)
	c.pan(15, 25, w, h)
	after := project(c.view(), c.projection(w/h), p, w, h)
	if d := after.Sub(before); !near(d.X(), 15) || !near(d.Y(), 25) {
		t.Errorf("point moved by %v on screen, expected (15, 25)", d)
	}
}

func TestMapCamera(t *testing.T) {
	c := mapCamera{width: 2000, height: 1000}
	c.fit(1000, 1000)
	if c.zoom != 0.5 || c.offset != (mgl64.Vec2{0, 250}) {
		t.Errorf("fit: zoom %v, offset %v", c.zoom, c.offset)
	}

	// The map point under the cursor stays there
	toMap := func(x, y float64) mgl64.Vec2 {
		return mgl64.Vec2{(x - c.offset.X()) / c.zoom, (y - c.offset.Y()) / c.zoom}
	}
	before := toMap(300, 400)
	c.zoomAt(300, 400, 3, 1000, 1000)
	if after := toMap(300, 400); !after.ApproxEqual(before) {
		t.Errorf("map point under the cursor moved from %v to %v", before, after)
	}

	// Zoom limits
	c.zoomAt(0, 0, 1e-6, 1000, 1000)
	if !near(c.zoom, 0.5/4) {
		t.Errorf("min zoom %v", c.zoom)
	}
	c.zoomAt(0, 0, 1e6, 1000, 1000)
	if c.zoom != 64 {
		t.Errorf("max zoom %v", c.zoom)
	}

	// Image scales are powers of two, up to the size limit (4096 pixels)
	for _, test := range []struct{ zoom, want float64 }{
		{0.3, 0.5}, {0.5, 0.5}, {0.6, 1}, {1.5, 2}, {64, 2.048},
	} {
		c.zoom = test.zoom
		if s := c.imageScale(1); s != test.want {
			t.Errorf("image scale at zoom %v: %v, expected %v", test.zoom, s, test.want)
		}
	}
}
