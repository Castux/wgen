package app

import (
	"encoding/json"
	"strings"

	"github.com/AllenDang/cimgui-go/imgui"

	"github.com/Castux/wgen/internal/config"
	"github.com/Castux/wgen/internal/render"
)

// param is a project parameter, with its current value.
type param struct {
	config.Param
	value any
}

// panelParams returns the parameters of a config, cached as long as the
// config doesn't change.
func (a *app) panelParams(conf *config.Config) []param {
	if conf == a.paramsConf {
		return a.params
	}

	a.paramsConf = conf
	a.params = a.params[:0]
	for _, p := range conf.Schema() {
		value, err := conf.Value(p.Path)
		if err != nil {
			continue
		}
		a.params = append(a.params, param{p, value})
	}
	return a.params
}

// panelWidth is the width of the panel, docked on the right of the window
// (0 without a project, when there is none).
func (a *app) panelWidth() float32 {
	if a.session.Engine.Config() == nil {
		return 0
	}
	return 380 * a.uiScale
}

// drawPanel is the side panel, docked on the right, from the menu bar down:
// the project (terrains, map and simulation parameters) and painting. How
// it is shown is in the View menu.
func (a *app) drawPanel() {
	conf := a.session.Engine.Config()
	if conf == nil {
		return
	}
	display := imgui.CurrentIO().DisplaySize()
	width := a.panelWidth()
	imgui.SetNextWindowPosV(imgui.NewVec2(display.X-width, a.menuHeight), imgui.CondAlways, imgui.NewVec2(0, 0))
	imgui.SetNextWindowSizeV(imgui.NewVec2(width, display.Y-a.menuHeight), imgui.CondAlways)

	flags := imgui.WindowFlagsNoTitleBar | imgui.WindowFlagsNoMove | imgui.WindowFlagsNoResize | imgui.WindowFlagsNoCollapse |
		imgui.WindowFlagsNoSavedSettings | imgui.WindowFlagsNoBringToFrontOnFocus
	if imgui.BeginV("##panel", nil, flags) {
		imgui.PushItemWidth(-170 * a.uiScale)
		a.drawTerrains(conf)
		a.drawPainting()
		a.drawParams(conf)
		imgui.PopItemWidth()
	}
	imgui.End()
}

// drawParams are the parameters of the project, from the config schema, by
// group.
func (a *app) drawParams(conf *config.Config) {
	group, open := "", false
	for _, p := range a.panelParams(conf) {
		if p.Group != group {
			group = p.Group
			open = imgui.CollapsingHeaderTreeNodeFlagsV(group, imgui.TreeNodeFlagsNone)
		}
		if !open {
			continue
		}

		key := strings.Join(p.Path, ".")
		v, ok := p.value.(float64)
		if !ok {
			continue
		}
		value, changed := a.number("param."+key, p.Label, v, p.Min, p.Max, p.Step, p.Type == "int", false, p.Tooltip)
		if changed && value != v {
			a.patch(p.Path, value)
		}
	}
}

// replay runs the simulation again, watching it.
func (a *app) replay() {
	if !a.settings.Watch {
		settings := a.settings
		settings.Watch = true
		a.setSettings(settings)
	}
	a.session.Engine.Rerun()
}

// patch applies a parameter change, such as ["simulation", "erodibility"] =
// 1e-6, as the partial config {"simulation": {"erodibility": 1e-6}}.
func (a *app) patch(path []string, value any) {
	for i := len(path) - 1; i >= 0; i-- {
		value = map[string]any{path[i]: value}
	}
	data, err := json.Marshal(value)
	if err == nil {
		err = a.session.Patch(data)
	}

	a.message = nil
	if err != nil {
		a.message = &message{text: err.Error(), error: true}
	}
	a.paramsConf = nil // show the config values again, even if unchanged
}

// drawLegend shows what colors are what elevations, with height colors.
func (a *app) drawLegend() {
	world := a.world
	if !a.settings.Legend || a.settings.Color != colorHeight || world == nil {
		return
	}

	// Top left, under the menu
	imgui.SetNextWindowPosV(imgui.NewVec2(8, a.menuHeight+8), imgui.CondAlways, imgui.NewVec2(0, 0))
	imgui.SetNextWindowBgAlpha(0.7)

	if imgui.BeginV("##legend", nil, overlayWindowFlags) {
		scale := a.settings.HeightScale
		a.drawLegendBar("Land", 0, world.Highest, func(t float64) [3]float64 { return render.HeightColor(scale, t) })
		if world.Lowest < 0 {
			imgui.Spacing()
			a.drawLegendBar("Water", world.Lowest, 0, render.WaterColor)
		}
	}
	imgui.End()
}

// drawLegendBar is a color scale from low to high, the highest at the top,
// with a few elevations.
func (a *app) drawLegendBar(title string, low, high float64, color func(t float64) [3]float64) {
	barWidth, barHeight := 18*a.uiScale, 180*a.uiScale
	drawList := imgui.WindowDrawList()

	imgui.TextUnformatted(title)
	origin := imgui.CursorScreenPos()
	const steps = 48
	for i := range steps {
		t0, t1 := float64(i)/steps, float64(i+1)/steps
		c := color((t0 + t1) / 2)
		packed := imgui.ColorConvertFloat4ToU32(imgui.NewVec4(float32(c[0]), float32(c[1]), float32(c[2]), 1))
		y0 := origin.Y + barHeight*float32(1-t1)
		y1 := origin.Y + barHeight*float32(1-t0)
		drawList.AddRectFilled(imgui.NewVec2(origin.X, y0), imgui.NewVec2(origin.X+barWidth, y1), packed)
	}
	for i := range 5 {
		t := float64(i) / 4
		y := origin.Y + barHeight*float32(1-t) - imgui.TextLineHeight()/2
		drawList.AddTextVec2(imgui.NewVec2(origin.X+barWidth+6*a.uiScale, y), 0xffeeeeee, formatMeters(low+(high-low)*t))
	}
	imgui.Dummy(imgui.NewVec2(barWidth+70*a.uiScale, barHeight))
}
