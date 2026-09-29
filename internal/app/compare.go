package app

import (
	"math"

	"github.com/AllenDang/cimgui-go/imgui"

	"github.com/Castux/wgen/internal/gen"
	"github.com/Castux/wgen/internal/geom"
)

// Comparing with the previous result, in the map view: the landscape before
// the last change on the left of a line, the current one on its right. The
// line can be dragged. The previous result is the last complete one before
// the current (previews don't count), rendered with the same settings.

type comparison struct {
	on bool

	current, previous *gen.World
	previousVersion   int // of the previous world, for its image
	image             imageSlot

	split    float64 // position of the line, 0..1 of the map's width
	dragging bool
}

// Grabbing the line: within this distance of it, window units
const splitGrab = 6

// trackResults keeps the previous complete result, as a new world is shown.
func (a *app) trackResults(world *gen.World) {
	c := &a.compare
	if a.session.Engine.State().Preview || world == c.current {
		return
	}
	if c.current != nil {
		c.previous = c.current
		c.previousVersion++
	}
	c.current = world
}

// forgetResults forgets the results of another project.
func (a *app) forgetResults() {
	a.compare.current, a.compare.previous = nil, nil
	a.compare.previousVersion++
	if a.mapView != nil { // not yet, opening the first project
		a.mapView.previous.delete()
	}
}

func (a *app) toggleCompare() {
	c := &a.compare
	c.on = !c.on
	if c.split == 0 {
		c.split = 0.5
	}
}

// comparing tells whether the map view shows the comparison.
func (a *app) comparing() bool {
	return a.compare.on && a.settings.View == viewMap && a.compare.previous != nil
}

// compareInput drags the line: it returns whether the mouse is used.
func (a *app) compareInput() bool {
	c := &a.compare
	if !a.comparing() {
		c.dragging = false
		return false
	}
	camera := &a.mapView.camera
	mouse := imgui.CurrentIO().MousePos()
	lineX := camera.offset.X() + c.split*camera.width*camera.zoom
	near := math.Abs(float64(mouse.X)-lineX) <= splitGrab*float64(a.uiScale)

	switch {
	case !c.dragging && near && imgui.IsMouseClickedBool(imgui.MouseButtonLeft) && !imgui.CurrentIO().WantCaptureMouse():
		c.dragging = true
	case c.dragging && !imgui.IsMouseDown(imgui.MouseButtonLeft):
		c.dragging = false
	}
	if c.dragging {
		c.split = geom.Clamp((float64(mouse.X)-camera.offset.X())/(camera.width*camera.zoom), 0, 1)
	}
	if near || c.dragging {
		imgui.SetMouseCursor(imgui.MouseCursorResizeEW)
	}
	return c.dragging
}

// drawComparison draws the line between the results, with their names.
func (a *app) drawComparison() {
	if !a.comparing() {
		return
	}
	camera := &a.mapView.camera
	_, height, _ := a.viewSize()
	x := float32(camera.offset.X() + a.compare.split*camera.width*camera.zoom)
	top := float32(math.Max(camera.offset.Y(), float64(a.menuHeight)))
	bottom := float32(math.Min(camera.offset.Y()+camera.height*camera.zoom, height))

	drawList := imgui.BackgroundDrawList()
	white := imgui.ColorConvertFloat4ToU32(imgui.NewVec4(1, 1, 1, 1))
	drawList.AddLineV(imgui.NewVec2(x, top), imgui.NewVec2(x, bottom), shadowColor, 4*a.uiScale)
	drawList.AddLineV(imgui.NewVec2(x, top), imgui.NewVec2(x, bottom), white, 2*a.uiScale)
	middle := (top + bottom) / 2
	drawList.AddCircleFilled(imgui.NewVec2(x, middle), 7*a.uiScale, shadowColor)
	drawList.AddCircleFilled(imgui.NewVec2(x, middle), 5*a.uiScale, white)

	margin := 8 * a.uiScale
	labelY := top + 40*a.uiScale
	for _, label := range []struct {
		text string
		left bool
	}{{"Before", true}, {"Now", false}} {
		size := imgui.CalcTextSize(label.text)
		at := imgui.NewVec2(x+margin, labelY)
		if label.left {
			at.X = x - margin - size.X
		}
		drawList.AddTextVec2(imgui.NewVec2(at.X+1, at.Y+1), shadowColor, label.text)
		drawList.AddTextVec2(at, white, label.text)
	}
}
