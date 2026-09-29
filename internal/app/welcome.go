package app

import (
	"github.com/AllenDang/cimgui-go/imgui"
)

// The welcome card, at startup (unless turned off) and from the Help menu:
// what wgen does, how to start, and the recent projects.

// Recent projects on the welcome card
const welcomeRecent = 5

func (a *app) openWelcome() { a.dialogs.welcome = true }

func (a *app) drawWelcome() {
	if !modal("welcome", "Welcome to wgen", &a.dialogs.welcome, 0, 0) {
		return
	}

	imgui.PushTextWrapPosV(imgui.CursorPosX() + 480*a.uiScale)
	imgui.TextUnformatted("Paint a map, get a landscape. Each color of the map is a terrain: the sea, lakes, and land terrains " +
		"with a target height for their summits. wgen raises the land from the sea and lets rivers carve it, " +
		"which gives valleys, ridges and river networks.")
	imgui.PopTextWrapPos()
	imgui.Spacing()

	button := func(label, tooltip string) bool {
		clicked := imgui.ButtonV(label, fullWidth())
		imgui.SetItemTooltip(tooltip)
		if clicked {
			imgui.CloseCurrentPopup()
		}
		return clicked
	}
	if button("Paint this map", "Paint terrains on the map shown, with the brush of the panel (E)") {
		settings := a.settings
		settings.setEditing(true)
		a.setSettings(settings)
	}
	if button("Open the example", "A finished map: two continents, mountain ranges, islands") {
		a.openExample()
	}
	if button("Import an image...", "A map painted elsewhere: pick the colors of the sea and of the terrains") {
		a.unsavedThen("import an image", func() {
			a.openFile("Import an image", "", "", false, imageExts, a.open)
		})
	}
	if button("New map...", "An empty sea or a random island, of a size to choose") {
		a.newDialog()
	}
	if button("Open a project...", "A project saved before, or an image") {
		a.openDialog()
	}

	if len(a.recent) > 0 {
		imgui.SeparatorText("Recent projects")
		for _, path := range a.recent[:min(len(a.recent), welcomeRecent)] {
			imgui.BeginDisabledV(!isFile(path))
			if imgui.SelectableBool(displayName(path) + "##" + path) {
				imgui.CloseCurrentPopup()
				a.openRecent(path)
			}
			imgui.EndDisabled()
			tooltipEvenDisabled(path)
		}
	}

	imgui.Separator()
	settings := a.settings
	if imgui.Checkbox("Show this at startup", &settings.ShowWelcome) {
		a.setSettings(settings)
	}
	imgui.SameLineV(imgui.ContentRegionAvail().X+imgui.CursorPosX()-imgui.CalcTextSize("Close").X-2*imgui.CurrentStyle().FramePadding().X, 0)
	if imgui.Button("Close") {
		imgui.CloseCurrentPopup()
	}
	imgui.EndPopup()
}
