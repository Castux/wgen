package app

import (
	"github.com/AllenDang/cimgui-go/imgui"

	"github.com/Castux/wgen/internal/engine"
)

// message is the result of an action, shown in the status.
type message struct {
	text  string
	error bool
}

// Text colors of the status and the dialogs
var (
	busyColor   = imgui.NewVec4(1, 0.88, 0.51, 1)
	errorColor  = imgui.NewVec4(1, 0.54, 0.5, 1)
	noticeColor = imgui.NewVec4(0.56, 0.79, 0.98, 1)
	textColor   = imgui.NewVec4(0.93, 0.93, 0.93, 1)
)

// drawStatus shows the engine state and the result of the last action, in
// the bottom left corner.
func (a *app) drawStatus(state engine.State) {
	type line struct {
		text  string
		color imgui.Vec4
	}
	var lines []line

	switch {
	case state.Busy && state.Progress != "":
		lines = append(lines, line{state.Progress, busyColor})
	case state.Busy && state.Preview:
		lines = append(lines, line{"Refining...", busyColor})
	case state.Busy:
		lines = append(lines, line{"Generating...", busyColor})
	case a.loading():
		lines = append(lines, line{"Loading...", busyColor})
	}
	if a.exporting.Load() {
		lines = append(lines, line{"Exporting...", busyColor})
	}
	if state.Error != "" {
		lines = append(lines, line{state.Error, errorColor})
	}

	if m := a.message; m != nil {
		color := textColor
		if m.error {
			color = errorColor
		}
		lines = append(lines, line{m.text, color})
	}

	if len(lines) == 0 {
		return
	}

	display := imgui.CurrentIO().DisplaySize()
	imgui.SetNextWindowPosV(imgui.NewVec2(8, display.Y-8), imgui.CondAlways, imgui.NewVec2(0, 1))
	imgui.SetNextWindowBgAlpha(0.6)
	imgui.SetNextWindowSizeConstraints(imgui.NewVec2(0, 0), imgui.NewVec2(display.X*0.6, display.Y/2))

	if imgui.BeginV("##status", nil, overlayWindowFlags) {
		imgui.PushTextWrapPosV(display.X * 0.6)
		for _, l := range lines {
			imgui.PushStyleColorVec4(imgui.ColText, l.color)
			imgui.TextUnformatted(l.text)
			imgui.PopStyleColor()
		}
		imgui.PopTextWrapPos()
	}
	imgui.End()
}

// Windows shown over the view, like the status and the legend: not moved,
// focused or clicked
const overlayWindowFlags = imgui.WindowFlagsNoDecoration | imgui.WindowFlagsAlwaysAutoResize | imgui.WindowFlagsNoSavedSettings |
	imgui.WindowFlagsNoFocusOnAppearing | imgui.WindowFlagsNoNav | imgui.WindowFlagsNoInputs
