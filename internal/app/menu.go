package app

import (
	"github.com/AllenDang/cimgui-go/imgui"

	"github.com/Castux/wgen/internal/engine"
	"github.com/Castux/wgen/internal/render"
)

// drawMenu is the main menu bar.
func (a *app) drawMenu(state engine.State) {
	if !imgui.BeginMainMenuBar() {
		return
	}
	a.menuHeight = imgui.WindowSize().Y

	if imgui.BeginMenu("File") {
		if imgui.MenuItemBoolV("New map...", "Ctrl+N", false, true) {
			a.newDialog()
		}
		if imgui.MenuItemBoolV("Open...", "Ctrl+O", false, true) {
			a.openDialog()
		}
		imgui.SetItemTooltip("A project, or an image to import as a map")
		imgui.Separator()
		if imgui.MenuItemBoolV("Save", "Ctrl+S", false, true) {
			a.saveProject(nil)
		}
		if imgui.MenuItemBoolV("Save as...", "Ctrl+Shift+S", false, true) {
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
		imgui.EndMenu()
	}

	if imgui.BeginMenu("Edit") {
		c := a.editor.canvas
		if imgui.MenuItemBoolV("Undo", "Ctrl+Z", false, c != nil && len(c.undo) > 0) {
			a.undo()
		}
		if imgui.MenuItemBoolV("Redo", "Ctrl+Y", false, c != nil && len(c.redo) > 0) {
			a.redo()
		}
		imgui.Separator()
		settings := a.settings
		if editing := settings.Editing; imgui.MenuItemBoolPtrV("Paint the map", "E", &editing, true) {
			settings.setEditing(editing)
		}
		imgui.MenuItemBoolPtrV("Lock the shoreline", "L", &settings.LockShore, true)
		if imgui.BeginMenu("Brush") {
			for _, brush := range brushes {
				if imgui.MenuItemBoolV(brush, "", settings.Brush == brush, true) {
					settings.Brush = brush
				}
			}
			imgui.EndMenu()
		}
		if settings != a.settings {
			a.setSettings(settings)
		}
		imgui.EndMenu()
	}

	if imgui.BeginMenu("View") {
		settings := a.settings
		choice := func(label, shortcut string, value *string, option string) {
			if imgui.MenuItemBoolV(label, shortcut, *value == option, true) {
				*value = option
			}
		}
		choice("3D", "V", &settings.View, viewOrbit)
		choice("Map", "V", &settings.View, viewMap)
		imgui.Separator()
		choice("Terrain colors", "Shift", &settings.Color, colorTerrain)
		choice("Height colors", "Shift", &settings.Color, colorHeight)
		if imgui.BeginMenu("Height scale") {
			choice("Rainbow", "", &settings.HeightScale, render.ScaleRainbow)
			choice("Gray", "", &settings.HeightScale, render.ScaleGray)
			imgui.EndMenu()
		}
		imgui.MenuItemBoolPtrV("Height legend", "", &settings.Legend, true)
		imgui.Separator()
		if lit := settings.Shading == shadingLit; imgui.MenuItemBoolPtrV("Lit", "Q", &lit, true) {
			settings.Shading = shadingUnlit
			if lit {
				settings.Shading = shadingLit
			}
		}
		imgui.MenuItemBoolPtrV("Wireframe", "W", &settings.Wireframe, true)
		imgui.Separator()
		if imgui.MenuItemBoolV("Reset the view", "R", false, true) {
			a.resetView()
		}
		if settings != a.settings {
			a.setSettings(settings)
		}
		imgui.EndMenu()
	}

	if imgui.BeginMenu("Simulation") {
		settings := a.settings
		imgui.MenuItemBoolPtrV("Watch the simulation", "", &settings.Watch, true)
		if settings != a.settings {
			a.setSettings(settings)
		}
		if imgui.MenuItemBoolV("Replay the simulation", "", false, !state.Busy && a.world != nil) {
			a.replay()
		}
		imgui.EndMenu()
	}

	if imgui.BeginMenu("Help") {
		if imgui.MenuItemBool("Controls") {
			a.dialogs.help = true
		}
		imgui.Separator()
		imgui.MenuItemBoolV("wgen "+Version, "", false, false)
		imgui.EndMenu()
	}

	imgui.EndMainMenuBar()
}
