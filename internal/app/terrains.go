package app

import (
	"fmt"
	"math"
	"slices"
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

	a.drawTerrainButtons(conf, brush)

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
		if a.world != nil && t.Height > 0 {
			if summit, ok := a.world.Summits[t.Name]; ok {
				imgui.TextDisabled(noFormat(fmt.Sprintf("Reached %s  (%+.0f%%)", formatMeters(summit), 100*(summit/t.Height-1))))
				imgui.SetItemTooltip("The height of this terrain's highest 5%%, in the last landscape: about where its summits are")
			}
		}
		a.drawCharacter(conf, t)
	}
	a.drawDetail(conf, t)
}

// drawTerrainButtons adds a terrain, and moves or removes the selected
// one. The sea and lakes stay first: land terrains move among themselves.
func (a *app) drawTerrainButtons(conf *config.Config, selected *config.Terrain) {
	if imgui.Button("Add") {
		a.addTerrain(conf)
	}
	imgui.SetItemTooltip("A new land terrain, painted with its own color")

	land := selected != nil && selected.Kind == config.Land
	index := slices.Index(conf.Terrains, selected)
	button := func(label, tooltip string, enabled bool) bool {
		imgui.SameLine()
		imgui.BeginDisabledV(!enabled)
		clicked := imgui.Button(label)
		imgui.EndDisabled()
		tooltipEvenDisabled(tooltip)
		return clicked
	}
	if button("Move up", "Up the list", land && index > 0 && conf.Terrains[index-1].Kind == config.Land) {
		a.moveTerrain(conf, selected, -1)
	}
	if button("Move down", "Down the list", land && index < len(conf.Terrains)-1) {
		a.moveTerrain(conf, selected, 1)
	}
	if button("Remove", "Its pixels become the first other land terrain. Can't be undone", land && len(conf.Land()) > 1) {
		a.removeTerrain(conf, selected)
	}
}

// moveTerrain moves a terrain up or down the list.
func (a *app) moveTerrain(conf *config.Config, t *config.Terrain, delta int) {
	a.editConfig(conf, func(c *config.Config) {
		i := slices.IndexFunc(c.Terrains, func(other *config.Terrain) bool { return other.Name == t.Name })
		j := i + delta
		if i < 0 || j < 0 || j >= len(c.Terrains) {
			return
		}
		c.Terrains[i], c.Terrains[j] = c.Terrains[j], c.Terrains[i]
	})
}

// drawCharacter is the kind of landform of a land terrain: a choice of
// characters, which set its slopes, rounding and river erosion, and those,
// which it can take from the project.
func (a *app) drawCharacter(conf *config.Config, t *config.Terrain) {
	key := "terrain." + t.Name + "."
	const fromProject = "Project default"
	inherits := t.CriticalSlope == config.Inherit && t.Rounding == config.Inherit && t.Erodibility == 1
	current := t.CharacterOf()
	shown := current
	switch {
	case inherits:
		shown = fromProject
	case shown == "":
		shown = "Custom"
	}
	if imgui.BeginCombo("Character", shown) {
		if imgui.SelectableBoolV(fromProject, inherits, imgui.SelectableFlagsNone, imgui.NewVec2(0, 0)) && !inherits {
			a.setTerrain(conf, t.Name, func(t *config.Terrain) {
				t.CriticalSlope, t.Rounding, t.Erodibility = config.Inherit, config.Inherit, 1
			})
		}
		imgui.SetItemTooltip("The slopes, rounding and river erosion of the project's Landscape")
		for _, c := range config.Characters {
			if imgui.SelectableBoolV(c.Name, c.Name == current, imgui.SelectableFlagsNone, imgui.NewVec2(0, 0)) && c.Name != current {
				a.setTerrain(conf, t.Name, func(t *config.Terrain) { t.SetCharacter(c) })
			}
			imgui.SetItemTooltip(c.Description)
		}
		imgui.EndCombo()
	}
	imgui.SetItemTooltip("The kind of landform: sets the slopes, rounding and river erosion below")

	edit := func(label, field string, own, project float64, set bool, low, high, step float64, tooltip string, apply func(*config.Terrain, float64)) {
		value, action := a.overridable(key+field, label, own, project, set, low, high, step, tooltip)
		switch action {
		case overrideSet:
			a.setTerrain(conf, t.Name, func(t *config.Terrain) { apply(t, value) })
		case overrideReset:
			a.setTerrain(conf, t.Name, func(t *config.Terrain) { apply(t, config.Inherit) })
		}
	}
	project := conf.Simulation
	edit("Slopes (°)", "slope", t.CriticalSlope, project.CriticalSlope, t.CriticalSlope != config.Inherit, 1, 89, 1,
		"The steepest slopes: hillslopes steeper than this collapse. High for sharp young mountains and cliffs, low for gentle hills",
		func(t *config.Terrain, v float64) { t.CriticalSlope = v })
	edit("Rounding", "rounding", t.Rounding, project.Rounding, t.Rounding != config.Inherit, 0, 1, 0.05,
		"How rounded the tops are, 0 to 1: soil creeping downhill smooths them, as on old mountains. 0: crisp",
		func(t *config.Terrain, v float64) { t.Rounding = v })
	// A multiplier of the project's: 1 is the project's
	edit("Erosion (×)", "erosion", t.Erodibility, 1, t.Erodibility != 1, 0, 100, 0.1,
		"How deep rivers cut, times the project's river erosion: more for soft rock and gorges, less for hard rock",
		func(t *config.Terrain, v float64) {
			if v == config.Inherit {
				v = 1
			}
			t.Erodibility = v
		})
}

// drawDetail is how fine the mesh is on a terrain: a choice of spacings, one
// per refinement level, or automatic (the finest on land, the coarsest on
// water).
func (a *app) drawDetail(conf *config.Config, t *config.Terrain) {
	metersPerPixel := a.metersPerPixel(conf)
	spacing := func(level int) string {
		meters := conf.Resolution * math.Pow(2, float64(conf.Levels-level)) * metersPerPixel
		if metersPerPixel == 0 {
			return fmt.Sprintf("%g px", conf.Resolution*math.Pow(2, float64(conf.Levels-level)))
		}
		return formatDistance(meters)
	}
	automatic := conf.Levels
	if t.IsWater() {
		automatic = 0
	}
	label := func(detail int) string {
		if detail == config.DetailAuto {
			return "Automatic, every " + spacing(automatic)
		}
		return "Every " + spacing(min(detail, conf.Levels))
	}

	if imgui.BeginCombo("Mesh", label(t.Detail)) {
		for _, detail := range append([]int{config.DetailAuto}, levelsFrom(conf.Levels)...) {
			if imgui.SelectableBoolV(label(detail), detail == t.Detail, imgui.SelectableFlagsNone, imgui.NewVec2(0, 0)) && detail != t.Detail {
				a.setTerrain(conf, t.Name, func(t *config.Terrain) { t.Detail = detail })
			}
		}
		imgui.EndCombo()
	}
	imgui.SetItemTooltip("How fine the mesh is on this terrain: finer gives finer valleys, and is slower. The finest is the Quality's resolution")
}

// levelsFrom are the refinement levels, from the coarsest: 0 to levels.
func levelsFrom(levels int) []int {
	all := make([]int, levels+1)
	for i := range all {
		all[i] = i
	}
	return all
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
		t := config.NewTerrain(name, color)
		t.Height = 1000
		c.Terrains = append(c.Terrains, t)
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

// drawPainting is painting the map: the mode, the brush, the shoreline
// lock. Undo and redo are in the Edit menu.
func (a *app) drawPainting() {
	if !imgui.CollapsingHeaderTreeNodeFlagsV("Painting", imgui.TreeNodeFlagsDefaultOpen) {
		return
	}

	settings := a.settings
	changed := false

	if editing := settings.Editing; imgui.Checkbox("Paint the map (E)", &editing) {
		settings.setEditing(editing)
		changed = true
	}
	imgui.SetItemTooltip("The left button paints the selected terrain, in the map or on the terrain in 3D. In 3D, the right button rotates")

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

	changed = imgui.Checkbox("Lock the shoreline (L)", &settings.LockShore) || changed
	imgui.SetItemTooltip("Land brushes leave sea and lakes alone, water brushes leave land alone")
	settings.PaintOpacity, edited = a.number("brush.opacity", "Painting opacity", settings.PaintOpacity, 0, 1, 0.05, false, false,
		"How much the painted map shows over the generated one, while painting")
	changed = changed || edited

	if changed {
		a.setSettings(settings)
	}
}
