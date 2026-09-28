package viewer

import (
	"fmt"
	"image"
	"math"
	"slices"

	"github.com/AllenDang/cimgui-go/imgui"

	"github.com/Castux/wgen/internal/config"
	"github.com/Castux/wgen/internal/engine"
	"github.com/Castux/wgen/internal/gen"
)

// editor is the map editor: stamps of terrains, painted in the 2D view. The
// map is sent to the engine at the end of each stroke, and saved to the
// config's image file on demand.
type editor struct {
	canvas *canvas
	brush  string // terrain name
	dirty  bool   // changed since loaded or saved
	sent   []config.Color
	seed   uint64

	stale   bool            // the whole texture needs uploading
	changed image.Rectangle // part of the texture to update

	newSize int32 // of new maps
}

// syncCanvas takes the map of a new world, unless it is ours or the
// canvas has unsaved changes.
func (a *app) syncCanvas(w *gen.World) {
	ed := &a.editor
	ours := len(ed.sent) > 0 && len(w.Outline) > 0 && &ed.sent[0] == &w.Outline[0]
	if ed.canvas != nil && (ours || ed.dirty || ed.canvas.stroke != nil) {
		return
	}
	ed.canvas = newCanvas(w.Width, w.Height, w.Outline)
	ed.stale = true
}

// sendCanvas regenerates with the edited map.
func (a *app) sendCanvas() {
	c := a.editor.canvas
	a.editor.sent = slices.Clone(c.pixels)
	a.session.SetOutline(&engine.Outline{Width: c.width, Height: c.height, Pixels: a.editor.sent})
}

// updatePaintTexture uploads the edited map to the map view.
func (a *app) updatePaintTexture() {
	ed := &a.editor
	a.mapView.paintOpacity = 0
	if ed.canvas == nil {
		return
	}
	if a.settings.Editing {
		a.mapView.paintOpacity = a.settings.PaintOpacity
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

// brushTerrain is the terrain of the brush, the first one if unset.
func (a *app) brushTerrain(conf *config.Config) *config.Terrain {
	if t := conf.Terrain(a.editor.brush); t != nil {
		return t
	}
	if len(conf.Terrains) == 0 {
		return nil
	}
	a.editor.brush = conf.Terrains[0].Name
	return conf.Terrains[0]
}

// canvasPosition converts window coordinates to canvas pixels.
func (a *app) canvasPosition(x, y float64) (float64, float64) {
	c, cam := a.editor.canvas, &a.mapView.camera
	scale := float64(c.width) / cam.width
	return (x - cam.offset.X()) / cam.zoom * scale, (y - cam.offset.Y()) / cam.zoom * scale
}

// paintInput handles painting with the left button, in the map view. It
// returns whether the input was used.
func (a *app) paintInput() bool {
	ed := &a.editor
	conf := a.session.Engine.Config()
	if !a.settings.Editing || a.settings.View != "map" || ed.canvas == nil || conf == nil || a.mapView.camera.width == 0 {
		return false
	}
	t := a.brushTerrain(conf)
	if t == nil {
		return false
	}

	io := imgui.CurrentIO()
	mouse := io.MousePos()
	x, y := a.canvasPosition(float64(mouse.X), float64(mouse.Y))
	radius := a.settings.BrushRadius
	next := func() uint64 { ed.seed++; return ed.seed }

	c := ed.canvas
	c.protect = a.shoreLock(conf, t)
	switch {
	case c.stroke == nil && imgui.IsMouseClickedBool(imgui.MouseButtonLeft) && !io.WantCaptureMouse():
		ed.changed = ed.changed.Union(c.beginStroke(x, y, radius, t.Color, next()))
	case c.stroke != nil && imgui.IsMouseDown(imgui.MouseButtonLeft):
		ed.changed = ed.changed.Union(c.continueStroke(x, y, radius, t.Color, next))
	case c.stroke != nil:
		if c.endStroke() {
			ed.dirty = true
			a.sendCanvas()
		}
	}

	// Brush outline, in the brush's color
	if !io.WantCaptureMouse() || c.stroke != nil {
		scale := a.mapView.camera.zoom * a.mapView.camera.width / float64(c.width)
		col := imgui.ColorConvertFloat4ToU32(imgui.NewVec4(float32(t.Color[0])/255, float32(t.Color[1])/255, float32(t.Color[2])/255, 1))
		dl := imgui.ForegroundDrawListViewportPtr()
		dl.AddCircleV(mouse, float32(radius*scale), 0xff000000, 48, 3)
		dl.AddCircleV(mouse, float32(radius*scale), col, 48, 1.5)
	}
	return c.stroke != nil
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

// canvasOutline is the edited map, for saving.
func (a *app) canvasOutline() *engine.Outline {
	c := a.editor.canvas
	if c == nil {
		return nil
	}
	return &engine.Outline{Width: c.width, Height: c.height, Pixels: slices.Clone(c.pixels)}
}

// newMap replaces the map by an empty one, all of the first sea terrain.
func (a *app) newMap(width, height int) {
	conf := a.session.Engine.Config()
	if conf == nil {
		return
	}
	i := slices.IndexFunc(conf.Terrains, func(t *config.Terrain) bool { return t.Kind == config.Sea })
	if i < 0 {
		a.message = &message{text: "no sea terrain in the config", error: true}
		return
	}

	pixels := make([]config.Color, width*height)
	for p := range pixels {
		pixels[p] = conf.Terrains[i].Color
	}
	a.editor.canvas = newCanvas(width, height, pixels)
	a.editor.stale = true
	a.editor.dirty = true
	vw, vh, _ := a.viewSize()
	a.mapView.setSize(float64(width), float64(height), vw, vh)
	a.sendCanvas()
}

func (a *app) drawEditor() {
	if !imgui.CollapsingHeaderTreeNodeFlagsV("Map editor", imgui.TreeNodeFlagsDefaultOpen) {
		return
	}
	ed := &a.editor
	conf := a.session.Engine.Config()
	if conf == nil {
		return
	}

	if editing := a.settings.Editing; imgui.Checkbox("Paint the map (e)", &editing) {
		s := a.settings
		s.Editing = editing
		if editing {
			s.View = "map"
		}
		a.setSettings(s)
	}

	// Brushes: the terrains
	world, _ := a.session.Engine.Snapshot()
	mpp := 1.0
	if world != nil && world.MetersPerPixel > 0 {
		mpp = world.MetersPerPixel
	}
	brush := a.brushTerrain(conf)
	for _, t := range conf.Terrains {
		imgui.PushIDStr("brush." + t.Name)
		col := imgui.NewVec4(float32(t.Color[0])/255, float32(t.Color[1])/255, float32(t.Color[2])/255, 1)
		if imgui.ColorButtonV("##color", col, imgui.ColorEditFlagsNoTooltip, imgui.NewVec2(0, 0)) {
			ed.brush = t.Name
		}
		imgui.SameLine()
		label := t.Name
		switch {
		case t.Kind == config.Sea:
			label += " (sea)"
		case t.Kind == config.Lake:
			label += " (lake)"
		case t.Calibrated():
			label += fmt.Sprintf(" (%.0f m)", t.Height)
		}
		if imgui.SelectableBoolV(label, t == brush, imgui.SelectableFlagsNone, imgui.NewVec2(0, 0)) {
			ed.brush = t.Name
		}
		imgui.PopID()
	}

	s := a.settings
	changed := imgui.Checkbox("Lock shoreline (l)", &s.LockShore)
	imgui.SetItemTooltip("Land brushes leave sea and lakes alone, water brushes leave land alone")
	sizeLabel := "Brush (px)"
	if mpp != 1 {
		sizeLabel = fmt.Sprintf("Brush (px, %.0f km)", s.BrushRadius*mpp/1000)
	}
	var c bool
	s.BrushRadius, c = a.number("editor.radius", sizeLabel, s.BrushRadius, 2, 200, 1, false)
	changed = changed || c
	s.PaintOpacity, c = a.number("editor.opacity", "Painting opacity", s.PaintOpacity, 0, 1, 0.05, false)
	changed = changed || c
	if changed {
		s.BrushRadius = math.Max(1, s.BrushRadius)
		a.setSettings(s)
	}

	full := imgui.NewVec2(-math.SmallestNonzeroFloat32, 0)
	half := imgui.NewVec2(imgui.ContentRegionAvail().X/2-imgui.CurrentStyle().ItemSpacing().X/2, 0)
	canUndo := ed.canvas != nil && len(ed.canvas.undo) > 0
	canRedo := ed.canvas != nil && len(ed.canvas.redo) > 0
	imgui.BeginDisabledV(!canUndo)
	if imgui.ButtonV("Undo (ctrl+z)", half) {
		a.undo()
	}
	imgui.EndDisabled()
	imgui.SameLine()
	imgui.BeginDisabledV(!canRedo)
	if imgui.ButtonV("Redo (ctrl+y)", half) {
		a.redo()
	}
	imgui.EndDisabled()

	label := "Save map"
	if ed.dirty {
		label = "Save map (unsaved changes)"
	}
	imgui.BeginDisabledV(!ed.dirty)
	if imgui.ButtonV(label+"###savemap", full) {
		a.save()
	}
	imgui.EndDisabled()

	if imgui.ButtonV("New map...", full) {
		if ed.newSize == 0 {
			ed.newSize = 2048
			if ed.canvas != nil {
				ed.newSize = int32(ed.canvas.width)
			}
		}
		imgui.OpenPopupStr("New map")
	}
	if imgui.BeginPopupModalV("New map", nil, imgui.WindowFlagsAlwaysAutoResize) {
		imgui.TextUnformatted("Replace the map by an empty sea (undo not possible).")
		imgui.InputIntV("Size (pixels)", &ed.newSize, 256, 1024, imgui.InputTextFlagsNone)
		ed.newSize = min(max(ed.newSize, 64), 16384)
		if imgui.Button("Create") {
			a.newMap(int(ed.newSize), int(ed.newSize))
			imgui.CloseCurrentPopup()
		}
		imgui.SameLine()
		if imgui.Button("Cancel") {
			imgui.CloseCurrentPopup()
		}
		imgui.EndPopup()
	}
}
