package app

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

func TestEyeCamera(t *testing.T) {
	var orbit orbitCamera
	orbit.reset(100, 100)
	orbit.target = mgl64.Vec3{10, 20, 0}
	orbit.theta = 0.3

	var eye eyeCamera
	eye.fromOrbit(&orbit)
	if eye.position.X() != 10 || eye.position.Y() != 20 || eye.heading != 0.3 {
		t.Fatalf("eye from orbit: %v, heading %g", eye.position, eye.heading)
	}

	// Looking the way the orbit camera does, level
	toTarget := orbit.target.Sub(orbit.position())
	toTarget[2] = 0
	if f := eye.forward(); f.Dot(toTarget.Normalize()) < 0.999 {
		t.Errorf("eye forward %v, orbit looks %v", f, toTarget.Normalize())
	}

	// Moving forward goes where it looks
	before := eye.position
	eye.move(5, 0)
	if moved := eye.position.Sub(before); !near(moved.Len(), 5) || moved.Normalize().Dot(eye.forward()) < 0.999 {
		t.Errorf("moved %v, looking %v", moved, eye.forward())
	}
	// Right is to the right: forward cross right is up
	before = eye.position
	eye.move(0, 5)
	if right := eye.position.Sub(before); eye.forward().Cross(right).Z() >= 0 {
		t.Errorf("right %v is not to the right of %v", right, eye.forward())
	}

	eye.look(0, -1e6, 100)
	if eye.pitch > eyeMaxPitch+1e-9 {
		t.Errorf("pitch %g beyond %g", eye.pitch, eyeMaxPitch)
	}

	eye.toOrbit(&orbit)
	if orbit.target.X() != eye.position.X() || orbit.target.Z() != 0 || orbit.theta != eye.heading {
		t.Errorf("orbit from eye: %v, theta %g", orbit.target, orbit.theta)
	}
}
