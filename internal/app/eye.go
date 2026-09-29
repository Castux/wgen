package app

import (
	"fmt"
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
	a.placeEye()
}

// placeEye keeps the eye in the map, at eye level above the ground (or the
// water).
func (a *app) placeEye() {
	v := a.terrain
	if a.inspect.probe == nil || v.width == 0 {
		return
	}
	eye := &v.eye
	x := geom.Clamp(eye.position.X(), -v.width/2, v.width/2)
	y := geom.Clamp(eye.position.Y(), -v.height/2, v.height/2)
	z := eye.position.Z()
	if surface := a.groundHeight(geom.Vec2{X: x + v.width/2, Y: y + v.height/2}); !math.IsNaN(surface) {
		z = surface*a.zScale() + eyeHeight/v.metersPerPixel
	}
	eye.position = mgl64.Vec3{x, y, z}
}

func (a *app) eyeSpeed() float64 {
	if a.eyeWalk.speed <= 0 {
		a.eyeWalk.speed = eyeDefaultSpeed
	}
	return a.eyeWalk.speed
}

// drawEyeHelp tells how to move, at the bottom of the eye level view.
func (a *app) drawEyeHelp() {
	if a.settings.View != viewEye || a.world == nil {
		return
	}
	viewWidth, viewHeight, _ := a.viewSize()
	imgui.SetNextWindowPosV(imgui.NewVec2(float32(viewWidth/2), float32(viewHeight)-8), imgui.CondAlways, imgui.NewVec2(0.5, 1))
	imgui.SetNextWindowBgAlpha(0.6)
	if imgui.BeginV("##eye", nil, overlayWindowFlags) {
		imgui.TextUnformatted(fmt.Sprintf("WASD or arrows: move, Shift: faster  ·  drag: look around  ·  wheel: speed, %s/s",
			formatDistance(a.eyeSpeed())))
	}
	imgui.End()
}
