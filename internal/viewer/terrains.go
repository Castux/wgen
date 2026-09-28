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
		switch t.Kind {
		case config.Sea:
			label += "  (sea level)"
		case config.Lake:
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
	colorKey := key + "color"
	color := colorFloats(t.Color)
	if a.widget.key == colorKey {
		color = a.widget.color
	}
	imgui.ColorEdit3V("Color##"+key, &color, imgui.ColorEditFlagsNoInputs|imgui.ColorEditFlagsPickerHueWheel)
	if a.widget.track(colorKey) {
		a.widget.color = color
	}
	if imgui.IsItemDeactivatedAfterEdit() {
		a.recolorTerrain(conf, t, colorBytes(color))
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

func formatMeters(z float64) string { return fmt.Sprintf("%.0f m", math.Round(z)) }

// editConfig applies a change to a copy of the config, and regenerates with
// it. The error is shown if the change makes it invalid.
func (a *app) editConfig(conf *config.Config, f func(*config.Config)) bool {
	edited := conf.Clone()
	f(edited)
	if err := a.session.SetConfig(edited); err != nil {
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
	if a.recolorCanvas(old, color) {
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
	n := len(conf.Terrains)
	color := conf.FreeColor(config.Color{uint8(40 + 53*n%200), uint8(90 + 97*n%150), uint8(60 + 31*n%180)})
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
	a.recolorCanvas(t.Color, replacement.Color)
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

	settings := a.settings
	changed := false

	if editing := settings.Editing; imgui.Checkbox("Paint the map (e)", &editing) {
		settings.setEditing(editing)
		changed = true
	}
	imgui.SetItemTooltip("In the map view: the left button paints the selected terrain, the others pan")

	for i, shape := range brushes {
		if i > 0 {
			imgui.SameLine()
		}
		if imgui.RadioButtonBool(shape, settings.Brush == shape) {
			settings.Brush = shape
			changed = true
		}
	}

	world, _ := a.session.Engine.Snapshot()
	sizeLabel := "Size (px)"
	if world != nil && world.MetersPerPixel > 0 {
		sizeLabel = fmt.Sprintf("Size (px, %.0f km)", settings.BrushRadius*world.MetersPerPixel/1000)
	}
	var edited bool
	settings.BrushRadius, edited = a.number("brush.radius", sizeLabel, settings.BrushRadius, minBrushRadius, maxBrushRadius, 1, false, true,
		"Radius, [ and ] to change it")
	changed = changed || edited

	changed = imgui.Checkbox("Lock shoreline (l)", &settings.LockShore) || changed
	imgui.SetItemTooltip("Land brushes leave sea and lakes alone, water brushes leave land alone")
	settings.PaintOpacity, edited = a.number("brush.opacity", "Painting opacity", settings.PaintOpacity, 0, 1, 0.05, false, false,
		"How much the painted map shows over the generated one, while painting")
	changed = changed || edited

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
		a.setSettings(settings)
	}
}
