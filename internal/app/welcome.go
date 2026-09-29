package app

import (
	"github.com/AllenDang/cimgui-go/imgui"
)

// The welcome card, whenever there is no project (at startup) and from the
// Help menu: what wgen does, how to start, and the recent projects.

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
	if button("New map...", "An empty sea or a random island, of a size to choose") {
		a.newDialog()
	}
	if button("Open...", "A project, or an image: without a project next to it, it becomes a new one, "+
		"picking the colors of the sea and of the terrains") {
		a.openDialog()
	}
	if button("Open the example", "A finished map: two continents, mountain ranges, islands") {
		a.openExample()
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

	// Closing it leaves the project open, if any
	if a.session.Engine.Config() != nil {
		imgui.Separator()
		if imgui.Button("Close") {
			imgui.CloseCurrentPopup()
		}
	}
	imgui.EndPopup()
}

// welcomeWithoutProject shows the welcome card when there is no project,
// and no dialog open (such as one opened from the card, then canceled).
func (a *app) welcomeWithoutProject() {
	if a.session.Engine.Config() != nil || a.dialogs.opening() ||
		imgui.IsPopupOpenStrV("", imgui.PopupFlagsAnyPopupId|imgui.PopupFlagsAnyPopupLevel) {
		return
	}
	a.openWelcome()
}
