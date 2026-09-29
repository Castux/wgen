package app

import (
	"fmt"

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

// drawCues tells what the input does in the current mode, at the bottom
// of the view: in 3D, at eye level, while painting or measuring. cueHeight keeps its height,
// for windows not to hide it.
func (a *app) drawCues() {
	var lines []string
	if a.settings.View == viewOrbit && a.world != nil {
		if a.settings.Editing {
			lines = append(lines, "3D: right drag: rotate  ·  Shift+right drag: pan  ·  wheel: zoom  ·  R: reset the view")
		} else {
			lines = append(lines, "3D: drag: rotate  ·  right drag or Shift+drag: pan  ·  wheel: zoom  ·  double click: center there  ·  R: reset the view")
		}
	}
	if a.settings.View == viewEye && a.world != nil {
		lines = append(lines, fmt.Sprintf("Eye level: WASD or arrows move, Shift faster  ·  drag: look around  ·  wheel: speed, %s/s",
			formatDistance(a.eyeSpeed())))
	}
	if a.settings.Editing {
		brush := "the selected terrain"
		if conf := a.session.Engine.Config(); conf != nil {
			if t := a.brushTerrain(conf); t != nil {
				brush = t.Name
			}
		}
		shore := "off"
		if a.settings.LockShore {
			shore = "on"
		}
		lines = append(lines, fmt.Sprintf("Painting %s (E to stop): left button paints  ·  [ ]: brush size  ·  L: lock the shoreline (%s)  ·  Ctrl+Z: undo",
			brush, shore))
	}
	if a.settings.Measuring {
		if a.inspect.drawing {
			lines = append(lines, "Drawing a ruler: click to add points  ·  double click or Enter: finish  ·  Escape: stop")
		} else {
			lines = append(lines, "Measuring (M to stop): click to start a ruler  ·  click a ruler: its profile  ·  Delete: remove it")
		}
	}
	a.cueHeight = 0
	if len(lines) == 0 {
		return
	}

	viewWidth, viewHeight, _ := a.viewSize()
	imgui.SetNextWindowPosV(imgui.NewVec2(float32(viewWidth/2), float32(viewHeight)-8), imgui.CondAlways, imgui.NewVec2(0.5, 1))
	imgui.SetNextWindowBgAlpha(0.6)
	if imgui.BeginV("##cues", nil, overlayWindowFlags) {
		for _, line := range lines {
			imgui.TextUnformatted(line)
		}
		a.cueHeight = imgui.WindowHeight()
	}
	imgui.End()
}
