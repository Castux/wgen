package app

import (
	"math"

	"github.com/AllenDang/cimgui-go/imgui"
	"github.com/go-gl/mathgl/mgl64"

	"github.com/Castux/wgen/internal/geom"
)

// Scale bars: in the map view, a bar of a round length in the bottom right
// corner, as on online maps. In the 3D view, lengths change across the
// view: the bar lies on the ground, under a point toward the bottom right
// (the first on the terrain, from the corner toward the center), level and
// facing the camera, and follows the relief; it is true there.

// Longest scale bar, window units
const scaleBarMax = 150

// Where the ground scale bar may lie in perspective, as fractions of the
// view: the first on the terrain
var groundScaleAt = [][2]float64{{0.78, 0.82}, {0.7, 0.75}, {0.62, 0.68}, {0.55, 0.62}, {0.5, 0.55}}

// niceLength is the longest round length (1, 2 or 5 times a power of ten)
// up to limit.
func niceLength(limit float64) float64 {
	if !(limit > 0) {
		return 0
	}
	power := math.Pow(10, math.Floor(math.Log10(limit)))
	for _, factor := range []float64{5, 2, 1} {
		if factor*power <= limit {
			return factor * power
		}
	}
	return power
}

func (a *app) drawScaleBar() {
	if !a.settings.ScaleBar || a.settings.View == viewEye || a.world == nil || a.inspect.probe == nil {
		return
	}
	if a.settings.View == viewMap {
		a.drawMapScaleBar()
	} else {
		a.drawGroundScaleBar()
	}
}

// drawMapScaleBar is a bar in the bottom right corner of the map view.
func (a *app) drawMapScaleBar() {
	camera := &a.mapView.camera
	if camera.zoom <= 0 {
		return
	}
	metersPerUnit := a.world.MetersPerPixel / camera.zoom // per window unit
	length := niceLength(scaleBarMax * float64(a.uiScale) * metersPerUnit)
	pixels := float32(length / metersPerUnit)

	width, height, _ := a.viewSize()
	margin := 16 * a.uiScale
	right, y := float32(width)-margin, float32(height)-margin
	if a.cueHeight > 0 {
		y -= a.cueHeight + 8*a.uiScale // above the cues
	}
	drawBar([]imgui.Vec2{{X: right - pixels, Y: y}, {X: right, Y: y}}, formatDistance(length), a.uiScale)
}

// drawGroundScaleBar is a bar on the ground, in perspective: at the first
// place for it that is on the map, whole.
func (a *app) drawGroundScaleBar() {
	width, height, _ := a.viewSize()
	for _, at := range groundScaleAt {
		if line, label, ok := a.groundScaleBar(width*at[0], height*at[1]); ok {
			drawBar(line, label, a.uiScale)
			return
		}
	}
}

// groundScaleBar is the scale bar on the ground under a window position:
// the line along it, and its length.
func (a *app) groundScaleBar(x, y float64) ([]imgui.Vec2, string, bool) {
	center, ok := a.mapPosition(x, y)
	if !ok {
		return nil, "", false
	}

	// Level, along the screen's horizontal
	width, height, _ := a.viewSize()
	view, _, _ := a.terrain.camera(a.settings.View, width/height)
	right := geom.Vec2{X: view.At(0, 0), Y: view.At(0, 1)}
	length := math.Hypot(right.X, right.Y)
	if length < 1e-9 {
		return nil, "", false
	}
	right = right.Scale(1 / length)

	// Its length on screen, measured around the point
	probe := a.inspect.probe
	project := func(p geom.Vec2) (imgui.Vec2, bool) {
		if !probe.Inside(p) {
			return imgui.Vec2{}, false // no ground there
		}
		x, y, ok := a.screenPosition(p, probe.Ground(p))
		return imgui.Vec2{X: float32(x), Y: float32(y)}, ok
	}
	left, okLeft := project(center.Sub(right.Scale(0.5)))
	rightEnd, okRight := project(center.Add(right.Scale(0.5)))
	if !okLeft || !okRight {
		return nil, "", false
	}
	pixelsPerUnit := math.Hypot(float64(rightEnd.X-left.X), float64(rightEnd.Y-left.Y))
	if !(pixelsPerUnit > 0) {
		return nil, "", false
	}
	meters := niceLength(scaleBarMax * float64(a.uiScale) * a.world.MetersPerPixel / pixelsPerUnit)
	half := meters / a.world.MetersPerPixel / 2

	// Following the relief, all of it on the map
	const steps = 32
	var line []imgui.Vec2
	for k := 0; k <= steps; k++ {
		s, ok := project(center.Add(right.Scale(-half + 2*half*float64(k)/steps)))
		if !ok {
			return nil, "", false
		}
		line = append(line, s)
	}
	return line, formatDistance(meters), true
}

// drawBar draws a scale bar along a line of window positions, with ticks at
// both ends and its length above the middle.
func drawBar(line []imgui.Vec2, label string, uiScale float32) {
	drawList := imgui.BackgroundDrawList()
	white := imgui.ColorConvertFloat4ToU32(imgui.NewVec4(1, 1, 1, 1))
	tick := 5 * uiScale
	for _, stroke := range []struct {
		color     uint32
		thickness float32
	}{{shadowColor, 4 * uiScale}, {white, 2 * uiScale}} {
		drawPolyline(drawList, line, stroke.color, stroke.thickness)
		for _, end := range []imgui.Vec2{line[0], line[len(line)-1]} {
			drawList.AddLineV(imgui.NewVec2(end.X, end.Y-tick), imgui.NewVec2(end.X, end.Y+tick), stroke.color, stroke.thickness)
		}
	}
	first, last := line[0], line[len(line)-1]
	middle := imgui.NewVec2((first.X+last.X)/2, min(first.Y, last.Y, line[len(line)/2].Y))
	size := imgui.CalcTextSize(label)
	at := imgui.NewVec2(middle.X-size.X/2, middle.Y-tick-size.Y-2*uiScale)
	drawList.AddTextVec2(imgui.NewVec2(at.X+1, at.Y+1), shadowColor, label)
	drawList.AddTextVec2(at, white, label)
}

// cameraPosition is where the camera of a 3D view is, in 3D units.
func (a *app) cameraPosition() mgl64.Vec3 {
	if a.settings.View == viewEye {
		return a.terrain.eye.position
	}
	return a.terrain.orbit.position()
}

// distanceTo is the distance from the camera of a 3D view to a map
// position on the ground, in meters: as it would be without vertical
// exaggeration. At eye level: from where one stands.
func (a *app) distanceTo(position geom.Vec2, ground float64) float64 {
	v := a.terrain
	camera := a.cameraPosition()
	dx := (position.X - v.width/2 - camera.X()) * a.world.MetersPerPixel
	dy := (position.Y - v.height/2 - camera.Y()) * a.world.MetersPerPixel
	dz := ground - camera.Z()/a.zScale()
	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}
