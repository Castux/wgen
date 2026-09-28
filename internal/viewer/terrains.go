package viewer

import (
	"fmt"
	"math"
	"strings"

	"github.com/AllenDang/cimgui-go/imgui"

	"github.com/Castux/wgen/internal/config"
)

// The terrains of the project: the brushes of the map editor, and their
// settings (name, color, target height, erodibility, detail). Sea and lake
// always exist; land terrains can be added and removed.

// drawTerrains lists the terrains: selecting one picks it as the brush, and
// its settings can be edited below the list.
func (a *app) drawTerrains(conf *config.Config) {
	if !imgui.CollapsingHeaderTreeNodeFlagsV("Terrains", imgui.TreeNodeFlagsDefaultOpen) {
		return
	}

	brush := a.brushTerrain(conf)
	for _, t := range conf.Terrains {
		imgui.PushIDStr("terrain." + t.Name)
		if imgui.ColorButtonV("##color", colorVec(t.Color), imgui.ColorEditFlagsNoTooltip, imgui.NewVec2(0, 0)) {
			a.editor.brush = t.Name
		}
		imgui.SameLine()
		label := t.Name
		switch {
		case t.Kind == config.Sea:
			label += "  (sea level)"
		case t.Kind == config.Lake:
			label += "  (lakes)"
		default:
			label += "  " + formatMeters(t.Height)
		}
		if imgui.SelectableBoolV(label, t == brush, imgui.SelectableFlagsNone, imgui.NewVec2(0, 0)) {
			a.editor.brush = t.Name
		}
		imgui.PopID()
	}

	if imgui.Button("Add terrain") {
		a.addTerrain(conf)
	}
	imgui.SetItemTooltip("A new land terrain, painted with its own color")

	if brush != nil {
		imgui.SeparatorText("Selected: " + brush.Name)
		a.drawTerrainSettings(conf, brush)
	}
}

func (a *app) drawTerrainSettings(conf *config.Config, t *config.Terrain) {
	key := "terrain." + t.Name + "."

	if t.Kind == config.Land {
		if name, done := a.text(key+"name", "Name", t.Name); done && name != t.Name {
			a.renameTerrain(conf, t, name)
		}
	}

	// Color: edited live in the widget, applied when done
	col := a.terrainColorEdit(key, t.Color)
	imgui.ColorEdit3V("Color##"+key, &col, imgui.ColorEditFlagsNoInputs|imgui.ColorEditFlagsPickerHueWheel)
	if imgui.IsItemActive() {
		a.editKey, a.editColor = key+"color", col
	} else if a.editKey == key+"color" {
		a.editKey = ""
	}
	if imgui.IsItemDeactivatedAfterEdit() {
		a.recolorTerrain(conf, t, config.Color{uint8(math.Round(float64(col[0]) * 255)), uint8(math.Round(float64(col[1]) * 255)), uint8(math.Round(float64(col[2]) * 255))})
	}

	if t.Kind == config.Land {
		if v, done := a.number(key+"height", "Target height (m)", t.Height, 0, 12000, 10, false, false,
			"Height the summits of each region of this terrain reach: most of it is lower, valleys much lower. 0: flat"); done && v != t.Height {
			a.setTerrain(conf, t.Name, func(t *config.Terrain) { t.Height = v })
		}
	}
	if v, done := a.number(key+"erodibility", "Erodibility factor", t.Erodibility, 0, 100, 0.1, false, false,
		"How easily rivers erode this terrain, relative to the simulation's erodibility: lower is harder rock, steeper valleys"); done && v != t.Erodibility {
		a.setTerrain(conf, t.Name, func(t *config.Terrain) { t.Erodibility = v })
	}
	if v, done := a.number(key+"detail", "Detail levels (-1: auto)", float64(t.Detail), -1, 6, 1, true, false,
		"How many times the mesh is refined in this terrain: more for mountains, less for plains and water. Auto: all on land, none on water"); done && int(v) != t.Detail {
		a.setTerrain(conf, t.Name, func(t *config.Terrain) { t.Detail = int(v) })
	}

	if t.Kind == config.Land {
		land := conf.Land()
		imgui.BeginDisabledV(len(land) < 2)
		if imgui.ButtonV("Remove terrain", fullWidth()) {
			a.removeTerrain(conf, t)
		}
		imgui.EndDisabled()
		imgui.SetItemTooltip("Its pixels become the first other land terrain. Can't be undone")
	}
}

// terrainColorEdit is the color shown in a terrain's color widget: being
// edited, or the terrain's.
func (a *app) terrainColorEdit(key string, c config.Color) [3]float32 {
	if a.editKey == key+"color" {
		return a.editColor
	}
	return [3]float32{float32(c[0]) / 255, float32(c[1]) / 255, float32(c[2]) / 255}
}

func formatMeters(z float64) string { return fmt.Sprintf("%.0f m", math.Round(z)) }

// editConfig applies a change to a copy of the config, and regenerates with
// it. The error is shown if the change makes it invalid.
func (a *app) editConfig(conf *config.Config, f func(*config.Config)) bool {
	n := conf.Clone()
	f(n)
	if err := a.session.SetConfig(n); err != nil {
		a.message = &message{text: err.Error(), error: true}
		return false
	}
	a.paramsConf = nil
	return true
}

func (a *app) setTerrain(conf *config.Config, name string, f func(*config.Terrain)) {
	a.editConfig(conf, func(c *config.Config) { f(c.Terrain(name)) })
}

func (a *app) renameTerrain(conf *config.Config, t *config.Terrain, name string) {
	name = strings.TrimSpace(name)
	switch {
	case name == "":
		return
	case conf.Terrain(name) != nil || name == config.SeaName || name == config.LakeName:
		a.message = &message{text: fmt.Sprintf("there is already a terrain named %q", name), error: true}
		return
	}
	old := t.Name
	if a.editConfig(conf, func(c *config.Config) { c.Terrain(old).Name = name }) && a.editor.brush == old {
		a.editor.brush = name
	}
}

// recolorTerrain gives a terrain a new color, and repaints its pixels.
func (a *app) recolorTerrain(conf *config.Config, t *config.Terrain, color config.Color) {
	if color == t.Color {
		return
	}
	if other, taken := conf.TerrainsByColor()[color]; taken {
		a.message = &message{text: fmt.Sprintf("%s already has that color", other.Name), error: true}
		return
	}
	old := t.Color
	if !a.editConfig(conf, func(c *config.Config) { c.Terrain(t.Name).Color = color }) {
		return
	}
	if c := a.editor.canvas; c != nil {
		c.recolor(old, color)
		a.editor.stale = true
		a.editor.dirty = true
		a.sendCanvas()
	}
}

// addTerrain adds a land terrain, and picks it as the brush.
func (a *app) addTerrain(conf *config.Config) {
	name := "terrain"
	for i := 2; conf.Terrain(name) != nil; i++ {
		name = fmt.Sprintf("terrain %d", i)
	}
	// A color of its own, away from the others
	color := conf.FreeColor(config.Color{uint8(40 + 53*len(conf.Terrains)%200), uint8(90 + 97*len(conf.Terrains)%150), uint8(60 + 31*len(conf.Terrains)%180)})
	if a.editConfig(conf, func(c *config.Config) {
		c.Terrains = append(c.Terrains, &config.Terrain{Name: name, Color: color, Height: 1000, Erodibility: 1, Detail: config.DetailAuto})
	}) {
		a.editor.brush = name
	}
}

// removeTerrain removes a land terrain: its pixels become the first other
// land terrain.
func (a *app) removeTerrain(conf *config.Config, t *config.Terrain) {
	var replacement *config.Terrain
	for _, other := range conf.Land() {
		if other != t {
			replacement = other
			break
		}
	}
	if replacement == nil {
		return
	}

	// Repainted first: the map must not have the removed color
	if c := a.editor.canvas; c != nil {
		c.recolor(t.Color, replacement.Color)
		a.editor.stale = true
		a.editor.dirty = true
	}
	name := t.Name
	if a.editConfig(conf, func(c *config.Config) {
		for i, other := range c.Terrains {
			if other.Name == name {
				c.Terrains = append(c.Terrains[:i], c.Terrains[i+1:]...)
				break
			}
		}
	}) {
		a.editor.brush = replacement.Name
		a.sendCanvas()
	}
}

// drawBrush is the map editor's brush: painting mode, shape, size.
func (a *app) drawBrush() {
	if !imgui.CollapsingHeaderTreeNodeFlagsV("Brush", imgui.TreeNodeFlagsDefaultOpen) {
		return
	}

	s := a.settings
	changed := false
	edit := func(c bool) { changed = changed || c }

	if editing := s.Editing; imgui.Checkbox("Paint the map (e)", &editing) {
		s.Editing = editing
		if editing {
			s.View = "map"
		}
		changed = true
	}
	imgui.SetItemTooltip("In the map view: the left button paints the selected terrain, the others pan")

	for i, shape := range brushes {
		if i > 0 {
			imgui.SameLine()
		}
		if imgui.RadioButtonBool(shape, s.Brush == shape) {
			s.Brush = shape
			changed = true
		}
	}

	world, _ := a.session.Engine.Snapshot()
	sizeLabel := "Size (px)"
	if world != nil && world.MetersPerPixel > 0 {
		sizeLabel = fmt.Sprintf("Size (px, %.0f km)", s.BrushRadius*world.MetersPerPixel/1000)
	}
	var c bool
	s.BrushRadius, c = a.number("brush.radius", sizeLabel, s.BrushRadius, 1, 500, 1, false, true, "Radius, [ and ] to change it")
	edit(c)

	edit(imgui.Checkbox("Lock shoreline (l)", &s.LockShore))
	imgui.SetItemTooltip("Land brushes leave sea and lakes alone, water brushes leave land alone")
	s.PaintOpacity, c = a.number("brush.opacity", "Painting opacity", s.PaintOpacity, 0, 1, 0.05, false, false,
		"How much the painted map shows over the generated one, while painting")
	edit(c)

	half := imgui.NewVec2(imgui.ContentRegionAvail().X/2-imgui.CurrentStyle().ItemSpacing().X/2, 0)
	canvas := a.editor.canvas
	imgui.BeginDisabledV(canvas == nil || len(canvas.undo) == 0)
	if imgui.ButtonV("Undo (ctrl+z)", half) {
		a.undo()
	}
	imgui.EndDisabled()
	imgui.SameLine()
	imgui.BeginDisabledV(canvas == nil || len(canvas.redo) == 0)
	if imgui.ButtonV("Redo (ctrl+y)", half) {
		a.redo()
	}
	imgui.EndDisabled()

	if changed {
		a.setSettings(s)
	}
}
