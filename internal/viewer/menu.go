package viewer

import (
	"github.com/AllenDang/cimgui-go/imgui"

	"github.com/Castux/wgen/internal/engine"
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
		s := a.settings
		if imgui.MenuItemBoolPtrV("Paint the map", "E", &s.Editing, true) && s.Editing {
			s.View = "map"
		}
		imgui.MenuItemBoolPtrV("Lock the shoreline", "L", &s.LockShore, true)
		if imgui.BeginMenu("Brush") {
			for _, b := range brushes {
				if imgui.MenuItemBoolV(b, "", s.Brush == b, true) {
					s.Brush = b
				}
			}
			imgui.EndMenu()
		}
		if s != a.settings {
			a.setSettings(s)
		}
		imgui.EndMenu()
	}

	if imgui.BeginMenu("View") {
		s := a.settings
		if imgui.MenuItemBoolV("3D", "V", s.View == "orbit", true) {
			s.View = "orbit"
		}
		if imgui.MenuItemBoolV("Map", "V", s.View == "map", true) {
			s.View = "map"
		}
		imgui.Separator()
		if imgui.MenuItemBoolV("Terrain colors", "Shift", s.Color == "terrain", true) {
			s.Color = "terrain"
		}
		if imgui.MenuItemBoolV("Height colors", "Shift", s.Color == "height", true) {
			s.Color = "height"
		}
		if imgui.BeginMenu("Height scale") {
			if imgui.MenuItemBoolV("Rainbow", "", s.HeightScale == heightScale[0], true) {
				s.HeightScale = heightScale[0]
			}
			if imgui.MenuItemBoolV("Gray", "", s.HeightScale == heightScale[1], true) {
				s.HeightScale = heightScale[1]
			}
			imgui.EndMenu()
		}
		imgui.MenuItemBoolPtrV("Height legend", "", &s.Legend, true)
		imgui.Separator()
		lit := s.Shading == "lit"
		if imgui.MenuItemBoolPtrV("Lit", "Q", &lit, true) {
			s.Shading = map[bool]string{true: "lit", false: "unlit"}[lit]
		}
		imgui.MenuItemBoolPtrV("Wireframe", "W", &s.Wireframe, true)
		imgui.Separator()
		if imgui.MenuItemBoolV("Reset the view", "R", false, true) {
			a.resetView()
		}
		if s != a.settings {
			a.setSettings(s)
		}
		imgui.EndMenu()
	}

	if imgui.BeginMenu("Simulation") {
		s := a.settings
		imgui.MenuItemBoolPtrV("Watch the simulation", "", &s.Watch, true)
		if s != a.settings {
			a.setSettings(s)
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
		imgui.EndMenu()
	}

	imgui.EndMainMenuBar()
}
