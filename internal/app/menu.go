package app

import (
	"github.com/AllenDang/cimgui-go/imgui"

	"github.com/Castux/wgen/internal/engine"
	"github.com/Castux/wgen/internal/render"
)

// The menus have the commands and how the landscape is shown; the panel
// has the project (terrains, map and simulation parameters) and the
// painting tools. Each option is in one place.

// drawMenu is the main menu bar.
func (a *app) drawMenu(state engine.State) {
	if !imgui.BeginMainMenuBar() {
		return
	}
	a.menuHeight = imgui.WindowSize().Y

	if imgui.BeginMenu("File") {
		a.drawFileMenu()
		imgui.EndMenu()
	}
	if imgui.BeginMenu("Edit") {
		a.drawEditMenu()
		imgui.EndMenu()
	}
	if imgui.BeginMenu("View") {
		a.drawViewMenu()
		imgui.EndMenu()
	}
	if imgui.BeginMenu("Simulation") {
		a.drawSimulationMenu(state)
		imgui.EndMenu()
	}
	if imgui.BeginMenu("Help") {
		if imgui.MenuItemBool("Welcome") {
			a.openWelcome()
		}
		if imgui.MenuItemBool("Controls") {
			a.dialogs.help = true
		}
		imgui.Separator()
		imgui.MenuItemBoolV("wgen "+Version, "", false, false)
		imgui.EndMenu()
	}

	imgui.EndMainMenuBar()
}

func (a *app) drawFileMenu() {
	if imgui.MenuItemBoolV("New map...", "Ctrl+N", false, true) {
		a.newDialog()
	}
	if imgui.MenuItemBoolV("Open...", "Ctrl+O", false, true) {
		a.openDialog()
	}
	imgui.SetItemTooltip("A project, or an image to import as a map")
	if imgui.BeginMenuV("Open recent", len(a.recent) > 0) {
		for _, path := range a.recent {
			if imgui.MenuItemBoolV(displayName(path)+"##"+path, "", false, isFile(path)) {
				a.openRecent(path)
			}
			tooltipEvenDisabled(path)
		}
		imgui.Separator()
		if imgui.MenuItemBool("Clear the list") {
			a.clearRecent()
		}
		imgui.EndMenu()
	}
	if imgui.MenuItemBool("Open the example") {
		a.openExample()
	}
	imgui.Separator()
	if imgui.MenuItemBoolV("Save", "Ctrl+S", false, a.hasProject()) {
		a.saveProject(nil)
	}
	if imgui.MenuItemBoolV("Save as...", "Ctrl+Shift+S", false, a.hasProject()) {
		a.saveProjectAs(nil)
	}
	imgui.Separator()
	if imgui.MenuItemBoolV("Export...", "Ctrl+E", false, a.world != nil) {
		a.openExport()
	}
	imgui.Separator()
	if imgui.MenuItemBoolV("Quit", "Ctrl+Q", false, true) {
		a.quit()
	}
}

func (a *app) drawEditMenu() {
	canvas := a.editor.canvas
	if imgui.MenuItemBoolV("Undo painting", "Ctrl+Z", false, canvas != nil && len(canvas.undo) > 0) {
		a.undo()
	}
	if imgui.MenuItemBoolV("Redo painting", "Ctrl+Y", false, canvas != nil && len(canvas.redo) > 0) {
		a.redo()
	}
}

// drawViewMenu has every display setting: the view, colors, shading, the
// overlay, and inspecting (information under the cursor, rulers).
func (a *app) drawViewMenu() {
	settings := a.settings
	changed := false
	choice := func(label, shortcut string, value *string, option string) {
		if imgui.MenuItemBoolV(label, shortcut, *value == option, true) && *value != option {
			*value, changed = option, true
		}
	}
	toggle := func(label, shortcut string, value *bool) {
		changed = imgui.MenuItemBoolPtrV(label, shortcut, value, true) || changed
	}
	number := func(key, label string, value *float64, lo, hi, step float64, tooltip string) {
		var edited bool
		*value, edited = a.number(key, label, *value, lo, hi, step, false, false, tooltip)
		changed = changed || edited
	}
	imgui.PushItemWidth(90 * a.uiScale)
	defer imgui.PopItemWidth()

	choice("3D", "V", &settings.View, viewOrbit)
	choice("Map", "V", &settings.View, viewMap)
	choice("Eye level", "V", &settings.View, viewEye)
	imgui.SetItemTooltip("Standing on the terrain, at the center of the 3D view: WASD or arrows move, a drag looks around")
	if imgui.BeginMenu("Eye level ground") {
		choice("Facets", "", &settings.EyeGround, groundFacets)
		imgui.SetItemTooltip("The mesh as it is: flat facets, about a kilometer wide")
		choice("Smooth", "", &settings.EyeGround, groundSmooth)
		imgui.SetItemTooltip("Finer ground around you, coarser with distance (4 m to 256 m, out to 30 km), smoothly interpolated, with small details")
		choice("Blocks", "", &settings.EyeGround, groundBlocks)
		imgui.SetItemTooltip("1 m blocks around you, rounded from the smooth ground, then the smooth ground")
		imgui.EndMenu()
	}

	imgui.SeparatorText("Colors")
	choice("Terrains", "Shift", &settings.Color, colorTerrain)
	choice("Heights", "Shift", &settings.Color, colorHeight)
	choice("Single color", "Shift", &settings.Color, colorSingle)
	imgui.SetItemTooltip("Land in one color, the sea and lakes in theirs")
	imgui.BeginDisabledV(settings.Color != colorSingle)
	if land := colorFloats(settings.LandColor); imgui.ColorEdit3V("Land color", &land, imgui.ColorEditFlagsNoInputs|imgui.ColorEditFlagsPickerHueWheel) {
		settings.LandColor, changed = colorBytes(land), true
	}
	imgui.EndDisabled()
	imgui.BeginDisabledV(settings.Color != colorHeight)
	if imgui.BeginMenu("Height scale") {
		choice("Rainbow", "", &settings.HeightScale, render.ScaleRainbow)
		choice("Gray", "", &settings.HeightScale, render.ScaleGray)
		imgui.EndMenu()
	}
	toggle("Height legend", "", &settings.Legend)
	imgui.EndDisabled()

	imgui.SeparatorText("Shading")
	if lit := settings.Shading == shadingLit; imgui.MenuItemBoolPtrV("Lit", "Q", &lit, true) {
		settings.Shading, changed = shadingUnlit, true
		if lit {
			settings.Shading = shadingLit
		}
	}
	toggle("Wireframe", "W", &settings.Wireframe)
	number("view.verticalScale", "Vertical exaggeration", &settings.VerticalScale, 0.1, 100, 0.5,
		"Of the 3D view: at 1, mountains have their true proportions")

	imgui.SeparatorText("Overlay")
	number("view.riverWidth", "River width (px)", &settings.RiverWidth, 0, 100, 0.1,
		"Width of the largest river, in map pixels (0: no rivers)")
	number("view.riverPower", "River width growth", &settings.RiverPower, 0, 1, 0.01,
		"How much wider big rivers are than small ones")
	toggle("Rivers colored by basin", "B", &settings.BasinColors)
	imgui.SetItemTooltip("A color per drainage basin (the rivers flowing to the same place in the sea), instead of blue")
	number("view.contours", "Contour interval (m)", &settings.Contours, 0, 10000, 1, "0: no contour lines")
	number("view.grid", "Grid size (km)", &settings.GridKm, 0, 100000, 10, "0: no grid")

	imgui.SeparatorText("Inspect")
	toggle("Scale bar", "", &settings.ScaleBar)
	imgui.SetItemTooltip("In the map, in the bottom right corner; in 3D, on the ground, true where it lies")
	toggle("Information under the cursor", "", &settings.HoverInfo)
	imgui.SetItemTooltip("Position, terrain, elevation or water depth, drainage, at the top of the window")
	if measuring := settings.Measuring; imgui.MenuItemBoolPtrV("Measure with rulers", "M", &measuring, true) {
		settings.setMeasuring(measuring)
		changed = true
	}
	imgui.SetItemTooltip("Click to add points, double click or Enter to finish, click a ruler to see its profile, Delete to remove it")
	if imgui.MenuItemBoolV("Delete all rulers", "", false, len(a.inspect.rulers) > 0) {
		a.clearRulers()
	}

	imgui.Separator()
	if imgui.MenuItemBoolV("Reset the view", "R", false, true) {
		a.resetView()
	}

	if changed {
		a.setSettings(settings)
	}
}

// drawSimulationMenu is about watching the simulation. Its parameters are
// the project's, in the panel.
func (a *app) drawSimulationMenu(state engine.State) {
	settings := a.settings
	changed := imgui.MenuItemBoolPtrV("Watch the simulation", "", &settings.Watch, true)
	imgui.SetItemTooltip("Show the landscape as it is simulated, instead of the result only")

	imgui.PushItemWidth(90 * a.uiScale)
	imgui.BeginDisabledV(!settings.Watch)
	var edited bool
	settings.WatchSteps, edited = a.number("simulation.watchSteps", "Time steps per frame", settings.WatchSteps, 1, 1000, 1, true, false, "")
	imgui.EndDisabled()
	imgui.PopItemWidth()
	if changed || edited {
		a.setSettings(settings)
	}

	imgui.Separator()
	if imgui.MenuItemBoolV("Replay the simulation", "", false, !state.Busy && a.world != nil) {
		a.replay()
	}
	imgui.SetItemTooltip("Run it again, to watch it")
}
