package app

import (
	"math"
	"time"

	"github.com/AllenDang/cimgui-go/imgui"
	"github.com/go-gl/glfw/v3.4/glfw"
	"github.com/go-gl/mathgl/mgl64"

	"github.com/Castux/wgen/internal/geom"
)

// The eye level view: first person, standing on the terrain. It starts at
// the center of the orbit view, looking the same way, and the orbit view
// continues from where the eye went. WASD (by position on the keyboard) or
// the arrows move, Shift faster, a drag looks around, the wheel sets the
// speed.

// eyeWalk is the state of walking.
type eyeWalk struct {
	speed     float64 // meters per second
	lastFrame time.Time
}

const (
	eyeDefaultSpeed = 50   // m/s
	eyeFastFactor   = 10   // with Shift
	eyeMinSpeed     = 1    // m/s
	eyeMaxSpeed     = 5000 // m/s
)

// Keys moving the eye, by position (the US layout's names): forward, right
var eyeKeys = map[glfw.Key][2]float64{
	glfw.KeyW: {1, 0}, glfw.KeyUp: {1, 0},
	glfw.KeyS: {-1, 0}, glfw.KeyDown: {-1, 0},
	glfw.KeyD: {0, 1}, glfw.KeyRight: {0, 1},
	glfw.KeyA: {0, -1}, glfw.KeyLeft: {0, -1},
}

// switchView keeps the 3D views together: the eye starts at the orbit's
// center, and the orbit continues from where the eye went.
func (a *app) switchView(from, to string) {
	v := a.terrain
	if v == nil {
		return
	}
	switch {
	case to == viewEye:
		v.eye.fromOrbit(&v.orbit)
		a.placeEye()
	case from == viewEye:
		v.eye.toOrbit(&v.orbit)
	}
}

// walk moves the eye with the keys held, and keeps it on the ground.
func (a *app) walk() {
	now := time.Now()
	dt := math.Min(now.Sub(a.eyeWalk.lastFrame).Seconds(), 0.1)
	a.eyeWalk.lastFrame = now
	if a.settings.View != viewEye || a.world == nil || a.inspect.probe == nil {
		return
	}

	if !imgui.CurrentIO().WantCaptureKeyboard() {
		var forward, right float64
		for key, direction := range eyeKeys {
			if a.window.GetKey(key) == glfw.Press {
				forward += direction[0]
				right += direction[1]
			}
		}
		if forward != 0 || right != 0 {
			speed := a.eyeSpeed()
			if a.window.GetKey(glfw.KeyLeftShift) == glfw.Press || a.window.GetKey(glfw.KeyRightShift) == glfw.Press {
				speed *= eyeFastFactor
			}
			step := speed * dt / a.terrain.metersPerPixel / math.Hypot(forward, right)
			a.terrain.eye.move(forward*step, right*step)
			a.activity() // keep moving while held
		}
	}
	a.followGround(dt)
}

// Rising and falling with the ground, the eye's height eases toward eye
// level (exponentially, in about this time, seconds), without going more
// than eyeMaxDip meters below
const (
	eyeEasing = 0.15
	eyeMaxDip = 0.5
)

// followGround keeps the eye in the map, easing toward eye level above the
// ground (or the water): steps, such as those of blocks, are climbed
// smoothly.
func (a *app) followGround(dt float64) {
	target, ok := a.eyeLevel()
	if !ok {
		return
	}
	eye := &a.terrain.eye
	z := eye.position.Z()
	z += (target - z) * (1 - math.Exp(-dt/eyeEasing))
	z = math.Max(z, target-eyeMaxDip/a.terrain.metersPerPixel)
	if math.Abs(target-z) > 1e-6/a.terrain.metersPerPixel {
		a.activity() // until settled
	}
	eye.position[2] = z
}

// placeEye puts the eye at eye level at once, in the map.
func (a *app) placeEye() {
	if target, ok := a.eyeLevel(); ok {
		a.terrain.eye.position[2] = target
	}
}

// eyeLevel clamps the eye in the map, and is the height of eye level there,
// in 3D units.
func (a *app) eyeLevel() (float64, bool) {
	v := a.terrain
	if a.inspect.probe == nil || v.width == 0 {
		return 0, false
	}
	eye := &v.eye
	x := geom.Clamp(eye.position.X(), -v.width/2, v.width/2)
	y := geom.Clamp(eye.position.Y(), -v.height/2, v.height/2)
	eye.position = mgl64.Vec3{x, y, eye.position.Z()}
	surface := a.groundHeight(geom.Vec2{X: x + v.width/2, Y: y + v.height/2})
	if math.IsNaN(surface) {
		return 0, false
	}
	return surface*a.zScale() + eyeHeight/v.metersPerPixel, true
}

func (a *app) eyeSpeed() float64 {
	if a.eyeWalk.speed <= 0 {
		a.eyeWalk.speed = eyeDefaultSpeed
	}
	return a.eyeWalk.speed
}
