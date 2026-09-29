package app

import (
	"image"
	"math"
	"slices"

	"github.com/AllenDang/cimgui-go/imgui"

	"github.com/Castux/wgen/internal/config"
	"github.com/Castux/wgen/internal/engine"
	"github.com/Castux/wgen/internal/gen"
	"github.com/Castux/wgen/internal/geom"
)

// editor is the map editor: terrains painted in the map view, or on the
// terrain in 3D. The map is sent to the engine at the end of each stroke,
// and saved with the project.
type editor struct {
	canvas *canvas
	brush  string // terrain name
	dirty  bool   // changed since loaded or saved
	sent   []config.Color
	seed   uint64

	stale   bool            // the whole texture needs uploading
	changed image.Rectangle // part of the texture to update
}

// syncCanvas takes the map of a new world, unless it is ours or the
// canvas has unsaved changes.
func (a *app) syncCanvas(w *gen.World) {
	ed := &a.editor
	ours := len(ed.sent) > 0 && len(w.Map) > 0 && &ed.sent[0] == &w.Map[0]
	if ed.canvas != nil && (ours || ed.dirty || ed.canvas.stroke != nil) {
		return
	}
	ed.canvas = newCanvas(w.Width, w.Height, w.Map)
	ed.stale = true
}

// setCanvas replaces the edited map (new project).
func (a *app) setCanvas(c *canvas, dirty bool) {
	a.editor.canvas = c
	a.editor.stale = true
	a.editor.dirty = dirty
	a.editor.sent = nil
}

// sendCanvas regenerates with the edited map.
func (a *app) sendCanvas() {
	c := a.editor.canvas
	if c == nil {
		return // not loaded yet
	}
	a.editor.sent = slices.Clone(c.pixels)
	a.session.SetMap(&engine.Map{Width: c.width, Height: c.height, Pixels: a.editor.sent})
}

// recolorCanvas replaces a color in the edited map, a terrain's that
// changed. It returns whether there is a map.
func (a *app) recolorCanvas(from, to config.Color) bool {
	c := a.editor.canvas
	if c == nil {
		return false
	}
	c.recolor(from, to)
	a.editor.stale = true
	a.editor.dirty = true
	return true
}

// canvasMap is the edited map, for saving.
func (a *app) canvasMap() *engine.Map {
	c := a.editor.canvas
	if c == nil {
		return nil
	}
	return &engine.Map{Width: c.width, Height: c.height, Pixels: slices.Clone(c.pixels)}
}

// updatePaintTexture uploads the edited map, shown over the map view and
// the terrain while painting.
func (a *app) updatePaintTexture() {
	ed := &a.editor
	a.mapView.paintOpacity, a.terrain.paintOpacity = 0, 0
	a.terrain.paint = &a.mapView.paint
	if ed.canvas == nil {
		return
	}
	if a.settings.Editing {
		a.mapView.paintOpacity = a.settings.PaintOpacity
		a.terrain.paintOpacity = a.settings.PaintOpacity
	}

	c := ed.canvas
	switch {
	case ed.stale:
		a.mapView.paint.uploadPixels(c.width, c.height, c.rgba(c.bounds()))
	case !ed.changed.Empty():
		a.mapView.paint.updateRegion(ed.changed, c.rgba(ed.changed))
	}
	ed.stale, ed.changed = false, image.Rectangle{}
}

// brushTerrain is the terrain of the brush, the first land one if unset.
func (a *app) brushTerrain(conf *config.Config) *config.Terrain {
	if t := conf.Terrain(a.editor.brush); t != nil {
		return t
	}
	if land := conf.Land(); len(land) > 0 {
		a.editor.brush = land[0].Name
		return land[0]
	}
	if len(conf.Terrains) == 0 {
		return nil
	}
	a.editor.brush = conf.Terrains[0].Name
	return conf.Terrains[0]
}

// canvasPosition converts window coordinates to canvas pixels: false in 3D
// when not on the terrain.
func (a *app) canvasPosition(x, y float64) (float64, float64, bool) {
	c := a.editor.canvas
	if a.settings.View == viewMap {
		camera := &a.mapView.camera
		scale := float64(c.width) / camera.width
		return (x - camera.offset.X()) / camera.zoom * scale, (y - camera.offset.Y()) / camera.zoom * scale, true
	}
	position, ok := a.mapPosition(x, y)
	if !ok {
		return 0, 0, false
	}
	return a.canvasFromMap(position)
}

// canvasFromMap converts a map position (y up) to canvas pixels (rows from
// the top).
func (a *app) canvasFromMap(position geom.Vec2) (float64, float64, bool) {
	c, world := a.editor.canvas, a.world
	if world == nil || world.Width == 0 {
		return 0, 0, false
	}
	scale := float64(c.width) / float64(world.Width)
	return position.X * scale, (float64(world.Height) - position.Y) * scale, true
}

// paintInput handles painting with the left button, in the map view or on
// the terrain in 3D. It returns whether the input was used.
func (a *app) paintInput() bool {
	ed := &a.editor
	conf := a.session.Engine.Config()
	if !a.settings.painting() || ed.canvas == nil || conf == nil || a.world == nil || a.mapView.camera.width == 0 {
		return false
	}
	brush := a.brushTerrain(conf)
	if brush == nil {
		return false
	}

	io := imgui.CurrentIO()
	mouse := io.MousePos()
	x, y, onMap := a.canvasPosition(float64(mouse.X), float64(mouse.Y))
	radius := a.settings.BrushRadius
	shape := a.settings.Brush
	next := func() uint64 { ed.seed++; return ed.seed }

	canvas := ed.canvas
	canvas.protect = a.shoreLock(conf, brush)
	// In 3D, the stroke skips where the cursor is off the terrain
	switch {
	case canvas.stroke == nil && imgui.IsMouseClickedBool(imgui.MouseButtonLeft) && !io.WantCaptureMouse():
		if onMap {
			ed.changed = ed.changed.Union(canvas.beginStroke(x, y, radius, brush.Color, next(), shape))
		}
	case canvas.stroke != nil && imgui.IsMouseDown(imgui.MouseButtonLeft):
		if onMap {
			ed.changed = ed.changed.Union(canvas.continueStroke(x, y, radius, brush.Color, next))
		}
	case canvas.stroke != nil:
		if canvas.endStroke() {
			ed.dirty = true
			a.sendCanvas()
		}
	}

	if onMap && (!io.WantCaptureMouse() || canvas.stroke != nil) {
		if a.settings.View == viewMap {
			screenRadius := float32(radius * a.mapView.camera.zoom * a.mapView.camera.width / float64(canvas.width))
			drawBrushOutline(mouse, screenRadius, shape, brush.Color)
		} else if a.inspect.hover != nil {
			a.drawBrushOnTerrain(a.inspect.hover.Position, radius*float64(a.world.Width)/float64(canvas.width), shape, brush.Color)
		}
	}
	return canvas.stroke != nil
}

// drawBrushOnTerrain shows the brush in 3D, following the relief, around a
// map position, of a radius in map units.
func (a *app) drawBrushOnTerrain(center geom.Vec2, radius float64, shape string, color config.Color) {
	// Around the outline, counterclockwise, from the right
	var outline []geom.Vec2
	if shape == brushSquare {
		corners := []geom.Vec2{{X: 1, Y: -1}, {X: 1, Y: 1}, {X: -1, Y: 1}, {X: -1, Y: -1}}
		for i, corner := range corners {
			next := corners[(i+1)%len(corners)]
			for k := range 16 {
				t := float64(k) / 16
				outline = append(outline, geom.Vec2{X: center.X + radius*geom.Lerp(corner.X, next.X, t), Y: center.Y + radius*geom.Lerp(corner.Y, next.Y, t)})
			}
		}
	} else {
		for k := range 64 {
			angle := 2 * math.Pi * float64(k) / 64
			outline = append(outline, geom.Vec2{X: center.X + radius*math.Cos(angle), Y: center.Y + radius*math.Sin(angle)})
		}
	}

	var points []imgui.Vec2
	for _, p := range outline {
		if s, ok := a.pointOnScreen(p); ok {
			points = append(points, s)
		}
	}
	if len(points) < 2 {
		return
	}
	drawList := imgui.ForegroundDrawListViewportPtr()
	for _, stroke := range []struct {
		color     uint32
		thickness float32
	}{{0xff000000, 3}, {imgui.ColorConvertFloat4ToU32(colorVec(color)), 1.5}} {
		for _, p := range points {
			drawList.PathLineTo(p)
		}
		drawList.PathStrokeV(stroke.color, imgui.DrawFlagsClosed, stroke.thickness)
	}
}

// drawBrushOutline shows the brush around the mouse, in its color over a
// black outline.
func drawBrushOutline(mouse imgui.Vec2, radius float32, shape string, color config.Color) {
	packed := imgui.ColorConvertFloat4ToU32(colorVec(color))
	drawList := imgui.ForegroundDrawListViewportPtr()
	if shape == brushSquare {
		topLeft, bottomRight := imgui.NewVec2(mouse.X-radius, mouse.Y-radius), imgui.NewVec2(mouse.X+radius, mouse.Y+radius)
		drawList.AddRectV(topLeft, bottomRight, 0xff000000, 0, 0, 3)
		drawList.AddRectV(topLeft, bottomRight, packed, 0, 0, 1.5)
	} else {
		drawList.AddCircleV(mouse, radius, 0xff000000, 48, 3)
		drawList.AddCircleV(mouse, radius, packed, 48, 1.5)
	}
}

// shoreLock returns the pixels a brush must not paint with the shoreline
// locked: water for land brushes, land for water brushes. Water can still
// change between sea and lake, as the shore stays.
func (a *app) shoreLock(conf *config.Config, brush *config.Terrain) func(config.Color) bool {
	if !a.settings.LockShore {
		return nil
	}
	terrains := conf.TerrainsByColor()
	brushWater := brush.IsWater()
	return func(c config.Color) bool {
		t := terrains[c]
		return t != nil && t.IsWater() != brushWater
	}
}

func (a *app) undo() {
	if c := a.editor.canvas; c != nil {
		if r := c.undoEdit(); !r.Empty() {
			a.afterEdit(r)
		}
	}
}

func (a *app) redo() {
	if c := a.editor.canvas; c != nil {
		if r := c.redoEdit(); !r.Empty() {
			a.afterEdit(r)
		}
	}
}

func (a *app) afterEdit(r image.Rectangle) {
	a.editor.changed = a.editor.changed.Union(r)
	a.editor.dirty = true
	a.sendCanvas()
}
