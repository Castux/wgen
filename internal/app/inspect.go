package app

import (
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/AllenDang/cimgui-go/imgui"

	"github.com/Castux/wgen/internal/gen"
	"github.com/Castux/wgen/internal/geom"
)

// Inspecting the landscape: what is under the cursor (the hover readout),
// and rulers, whose altitude profile shows in a window (profile.go). Rulers
// are drawn in both views; in measuring mode, a left click adds a point to
// the ruler being drawn, or selects a ruler, or starts one. Drags still
// move the camera.

// inspector is the state of inspecting.
type inspector struct {
	probe   *gen.Probe
	probeOf *gen.World

	hover *gen.Point // under the cursor, nil if nothing

	rulers   []*ruler
	selected int  // index in rulers, -1 if none
	drawing  bool // the selected ruler is being drawn

	press           imgui.Vec2 // where the left button went down, measuring
	pressed         bool
	finishOnRelease bool // a double click finishes the ruler

	marker *gen.ProfileSample // hovered in the profile, shown in the view
}

// ruler is a path measured on the map, with its profile.
type ruler struct {
	points []geom.Vec2 // map positions

	profile   gen.Profile
	profileOf *gen.World
	profiled  int // number of points of the profile
}

// Clicks: the mouse moves less than this (window units) between press and
// release; rulers are selected within this distance
const (
	clickTolerance = 4
	rulerTolerance = 6
)

func (a *app) selectedRuler() *ruler {
	if i := a.inspect.selected; i >= 0 && i < len(a.inspect.rulers) {
		return a.inspect.rulers[i]
	}
	return nil
}

// profileOf is the profile of a ruler on the displayed world, computed when
// the ruler or the world changed.
func (a *app) profileOf(r *ruler) *gen.Profile {
	if r.profileOf != a.world || r.profiled != len(r.points) {
		r.profile = a.inspect.probe.Profile(r.points)
		r.profileOf, r.profiled = a.world, len(r.points)
	}
	return &r.profile
}

// clearRulers forgets the rulers, of another project.
func (a *app) clearRulers() {
	a.inspect.rulers = nil
	a.inspect.selected = -1
	a.inspect.drawing = false
	a.inspect.marker = nil
}

// updateInspector indexes a new world, and finds what is under the
// cursor.
func (a *app) updateInspector() {
	inspect := &a.inspect
	if a.world != inspect.probeOf {
		inspect.probe, inspect.probeOf = nil, a.world
		if a.world != nil {
			inspect.probe = gen.NewProbe(a.world)
		}
	}

	inspect.hover = nil
	io := imgui.CurrentIO()
	if inspect.probe == nil || (io.WantCaptureMouse() && !a.input.drag.active) {
		return
	}
	mouse := io.MousePos()
	if position, ok := a.mapPosition(float64(mouse.X), float64(mouse.Y)); ok {
		point := inspect.probe.At(position)
		inspect.hover = &point
	}
}

// measureInput handles the left button in measuring mode: clicks, not
// drags, which still move the camera.
func (a *app) measureInput() {
	inspect := &a.inspect
	if !a.settings.Measuring {
		inspect.pressed = false
		return
	}
	io := imgui.CurrentIO()
	mouse := io.MousePos()

	if imgui.IsMouseClickedBool(imgui.MouseButtonLeft) && !io.WantCaptureMouse() {
		inspect.press, inspect.pressed = mouse, true
		if imgui.IsMouseDoubleClicked(imgui.MouseButtonLeft) && inspect.drawing {
			inspect.finishOnRelease = true
		}
	}
	if !inspect.pressed || !imgui.IsMouseReleased(imgui.MouseButtonLeft) {
		return
	}
	inspect.pressed = false
	if math.Hypot(float64(mouse.X-inspect.press.X), float64(mouse.Y-inspect.press.Y)) > clickTolerance {
		return // a drag
	}
	if inspect.finishOnRelease {
		inspect.finishOnRelease = false
		a.finishRuler()
		return
	}
	a.rulerClick(float64(mouse.X), float64(mouse.Y))
}

// rulerClick adds a point to the ruler being drawn, or selects the ruler
// clicked, or starts a new one.
func (a *app) rulerClick(x, y float64) {
	inspect := &a.inspect
	position, ok := a.mapPosition(x, y)

	if inspect.drawing {
		if ok {
			r := a.selectedRuler()
			r.points = append(r.points, position)
		}
		return
	}
	if i := a.rulerAt(x, y); i >= 0 {
		inspect.selected = i
		return
	}
	if !ok {
		inspect.selected = -1
		return
	}
	inspect.rulers = append(inspect.rulers, &ruler{points: []geom.Vec2{position}})
	inspect.selected = len(inspect.rulers) - 1
	inspect.drawing = true
}

// finishRuler ends drawing the ruler: one of a single point is dropped.
func (a *app) finishRuler() {
	inspect := &a.inspect
	if !inspect.drawing {
		inspect.selected = -1
		return
	}
	inspect.drawing = false
	if r := a.selectedRuler(); r != nil && len(r.points) < 2 {
		a.deleteRuler()
	}
}

// deleteRuler removes the selected ruler.
func (a *app) deleteRuler() {
	inspect := &a.inspect
	if a.selectedRuler() == nil {
		return
	}
	inspect.rulers = slices.Delete(inspect.rulers, inspect.selected, inspect.selected+1)
	inspect.selected = -1
	inspect.drawing = false
	inspect.marker = nil
}

// rulerAt is the ruler drawn at a window position, -1 if none.
func (a *app) rulerAt(x, y float64) int {
	best, bestDistance := -1, float64(rulerTolerance)
	for i, r := range a.inspect.rulers {
		line := a.rulerLine(r, nil)
		for j := 1; j < len(line); j++ {
			if d := segmentDistance(x, y, line[j-1], line[j]); d < bestDistance {
				best, bestDistance = i, d
			}
		}
	}
	return best
}

func segmentDistance(x, y float64, a, b imgui.Vec2) float64 {
	ax, ay, bx, by := float64(a.X), float64(a.Y), float64(b.X), float64(b.Y)
	dx, dy := bx-ax, by-ay
	t := 0.0
	if length := dx*dx + dy*dy; length > 0 {
		t = geom.Clamp(((x-ax)*dx+(y-ay)*dy)/length, 0, 1)
	}
	return math.Hypot(x-(ax+t*dx), y-(ay+t*dy))
}

// rulerLine is a ruler in window coordinates, with extra (the cursor, while
// drawing) at the end if not nil. In 3D, it follows the ground between the
// points.
func (a *app) rulerLine(r *ruler, extra *geom.Vec2) []imgui.Vec2 {
	points := r.points
	if extra != nil {
		points = append(slices.Clone(points), *extra)
	}
	if len(points) == 0 || a.inspect.probe == nil {
		return nil
	}

	var line []imgui.Vec2
	add := func(p geom.Vec2) {
		if s, ok := a.pointOnScreen(p); ok {
			line = append(line, s)
		}
	}
	add(points[0])
	for i := 1; i < len(points); i++ {
		from, to := points[i-1], points[i]
		steps := 1
		if a.settings.View != viewMap {
			steps = int(math.Min(256, math.Max(1, from.Dist(to)/2)))
		}
		for k := 1; k <= steps; k++ {
			t := float64(k) / float64(steps)
			add(geom.Vec2{X: geom.Lerp(from.X, to.X, t), Y: geom.Lerp(from.Y, to.Y, t)})
		}
	}
	return line
}

// pathLength is the length of a path of map positions, in meters.
func (a *app) pathLength(points []geom.Vec2) float64 {
	length := 0.0
	for i := 1; i < len(points); i++ {
		length += points[i].Dist(points[i-1])
	}
	return length * a.world.MetersPerPixel
}

// Ruler colors
var (
	rulerColor    = imgui.ColorConvertFloat4ToU32(imgui.NewVec4(1, 1, 1, 1))
	selectedColor = imgui.ColorConvertFloat4ToU32(imgui.NewVec4(1, 0.85, 0.2, 1))
	shadowColor   = imgui.ColorConvertFloat4ToU32(imgui.NewVec4(0, 0, 0, 0.6))
)

// drawRulers draws the rulers over the view, under the windows, and the
// point hovered in the profile.
func (a *app) drawRulers() {
	inspect := &a.inspect
	if a.world == nil || inspect.probe == nil {
		return
	}
	drawList := imgui.BackgroundDrawList()
	thickness := 2 * a.uiScale

	for i, r := range inspect.rulers {
		color := rulerColor
		if i == inspect.selected {
			color = selectedColor
		}

		var extra *geom.Vec2
		points := r.points
		if i == inspect.selected && inspect.drawing && inspect.hover != nil {
			extra = &inspect.hover.Position
			points = append(slices.Clone(points), *extra)
		}
		line := a.rulerLine(r, extra)
		if len(line) >= 2 {
			drawPolyline(drawList, line, shadowColor, thickness+2*a.uiScale)
			drawPolyline(drawList, line, color, thickness)
		}
		for _, p := range r.points {
			if s, ok := a.pointOnScreen(p); ok {
				drawList.AddCircleFilled(s, 4*a.uiScale, shadowColor)
				drawList.AddCircleFilled(s, 3*a.uiScale, color)
			}
		}

		// Length at the end
		if len(points) >= 2 {
			if s, ok := a.pointOnScreen(points[len(points)-1]); ok {
				label := formatDistance(a.pathLength(points))
				at := imgui.NewVec2(s.X+8*a.uiScale, s.Y-imgui.TextLineHeight()/2)
				drawList.AddTextVec2(imgui.NewVec2(at.X+1, at.Y+1), shadowColor, label)
				drawList.AddTextVec2(at, color, label)
			}
		}
	}

	if m := inspect.marker; m != nil {
		if s, ok := a.pointOnScreen(m.Position); ok {
			drawList.AddCircleFilled(s, 6*a.uiScale, shadowColor)
			drawList.AddCircleFilled(s, 4*a.uiScale, selectedColor)
		}
	}
}

// drawPolyline draws connected lines. (AddPolyline's binding passes only
// the first point.)
func drawPolyline(drawList *imgui.DrawList, points []imgui.Vec2, color uint32, thickness float32) {
	for _, p := range points {
		drawList.PathLineTo(p)
	}
	drawList.PathStrokeV(color, imgui.DrawFlagsNone, thickness)
}

func (a *app) pointOnScreen(p geom.Vec2) (imgui.Vec2, bool) {
	x, y, ok := a.screenPosition(p, a.inspect.probe.Ground(p))
	return imgui.Vec2{X: float32(x), Y: float32(y)}, ok
}

// drawHover shows what is under the cursor, at the top of the window.
//
// At eye level, two lines: where the eye is, and where the cursor points,
// with how far.
func (a *app) drawHover() {
	point := a.inspect.hover
	var lines []string
	if a.settings.View == viewEye && a.world != nil && a.inspect.probe != nil {
		v := a.terrain
		here := a.inspect.probe.At(geom.Vec2{X: v.eye.position.X() + v.width/2, Y: v.eye.position.Y() + v.height/2})
		if here.Inside {
			lines = append(lines, "Here:    "+a.describe(&here))
		}
		if point != nil {
			lines = append(lines, "Cursor:  "+a.describe(point)+"  ·  "+formatDistance(a.distanceTo(point.Position, point.Ground))+" away")
		}
	} else if point != nil {
		lines = append(lines, a.describe(point))
	}
	if !a.settings.HoverInfo || len(lines) == 0 {
		return
	}

	viewWidth, _, _ := a.viewSize()
	imgui.SetNextWindowPosV(imgui.NewVec2(float32(viewWidth/2), a.menuHeight+8), imgui.CondAlways, imgui.NewVec2(0.5, 0))
	imgui.SetNextWindowBgAlpha(0.7)
	if imgui.BeginV("##hover", nil, overlayWindowFlags) {
		for _, line := range lines {
			imgui.TextUnformatted(line)
		}
	}
	imgui.End()
}

// describe is a line about a point: position from the top left corner,
// terrain, elevation or water depth, drainage.
func (a *app) describe(point *gen.Point) string {
	world := a.world
	metersPerKm := 1000 / world.MetersPerPixel
	parts := []string{fmt.Sprintf("%.1f, %.1f km", point.Position.X/metersPerKm, (float64(world.Height)-point.Position.Y)/metersPerKm)}

	name := "no terrain"
	if point.Terrain != nil {
		name = point.Terrain.Name
	}
	switch {
	case point.Water && point.Surface == 0:
		parts = append(parts, name, formatMeters(point.Depth)+" deep")
	case point.Water:
		parts = append(parts, name, "surface "+formatMeters(point.Surface), formatMeters(point.Depth)+" deep")
	default:
		parts = append(parts, name, formatMeters(point.Ground))
		if point.Drainage > 0 {
			parts = append(parts, "drainage "+formatArea(point.Drainage))
		}
	}
	return strings.Join(parts, "  ·  ")
}

// formatDistance writes a distance in meters, in km when long: to 0.1 km
// under 100 km, without the decimal for whole kilometers.
func formatDistance(meters float64) string {
	km := meters / 1000
	switch {
	case km >= 100 || (km >= 1 && math.Round(km*10) == math.Round(km)*10):
		return fmt.Sprintf("%.0f km", km)
	case km >= 1:
		return fmt.Sprintf("%.1f km", km)
	}
	return fmt.Sprintf("%.0f m", meters)
}

// formatArea writes an area in square meters, in km² when large.
func formatArea(squareMeters float64) string {
	km2 := squareMeters / 1e6
	switch {
	case km2 >= 100:
		return fmt.Sprintf("%.0f km²", km2)
	case km2 >= 1:
		return fmt.Sprintf("%.1f km²", km2)
	}
	return fmt.Sprintf("%.2f km²", km2)
}
