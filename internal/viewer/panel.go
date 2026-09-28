package viewer

import (
	"encoding/json"
	"strings"

	"github.com/AllenDang/cimgui-go/imgui"

	"github.com/Castux/wgen/internal/config"
	"github.com/Castux/wgen/internal/engine"
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

// drawPanel is the side panel: terrains and brushes, parameters, display.
func (a *app) drawPanel(state engine.State) {
	display := imgui.CurrentIO().DisplaySize()
	top := a.menuHeight
	width := 380 * a.uiScale
	imgui.SetNextWindowPosV(imgui.NewVec2(display.X-width-8, top+8), imgui.CondFirstUseEver, imgui.NewVec2(0, 0))
	imgui.SetNextWindowSizeV(imgui.NewVec2(width, display.Y-top-16), imgui.CondFirstUseEver)

	if imgui.BeginV("Panel", nil, imgui.WindowFlagsNone) {
		keepInside(display)
		imgui.PushItemWidth(-170 * a.uiScale)
		conf := a.session.Engine.Config()
		if conf != nil {
			a.drawTerrains(conf)
			a.drawBrush()
			a.drawParams(conf, state)
		}
		a.drawDisplay()
		imgui.PopItemWidth()
	}
	imgui.End()
}

func (a *app) drawDisplay() {
	if !imgui.CollapsingHeaderTreeNodeFlagsV("Display", imgui.TreeNodeFlagsNone) {
		return
	}

	settings := a.settings
	changed := false
	editCombo := func(label string, value *string, options []string) {
		var edited bool
		*value, edited = combo(label, *value, options)
		changed = changed || edited
	}
	editNumber := func(key, label string, value *float64, lo, hi, step float64, tooltip string) {
		var edited bool
		*value, edited = a.number(key, label, *value, lo, hi, step, false, false, tooltip)
		changed = changed || edited
	}

	editCombo("View (v)", &settings.View, views)
	editCombo("Colors (shift)", &settings.Color, colorModes)
	editCombo("Height scale", &settings.HeightScale, heightScales)
	changed = imgui.Checkbox("Height legend", &settings.Legend) || changed
	editCombo("Shading (q)", &settings.Shading, shadings)
	changed = imgui.Checkbox("Wireframe (w)", &settings.Wireframe) || changed
	editNumber("view.verticalScale", "Vertical exaggeration", &settings.VerticalScale, 0.1, 100, 0.5,
		"Of the 3D view: at 1, mountains have their true proportions")

	imgui.SeparatorText("Overlay")
	editNumber("view.riverWidth", "River width (px)", &settings.RiverWidth, 0, 100, 0.1,
		"Width of the largest river, in map pixels (0: no rivers)")
	editNumber("view.riverPower", "River width growth", &settings.RiverPower, 0, 1, 0.01,
		"How much wider big rivers are than small ones")
	editNumber("view.contours", "Contour interval (m)", &settings.Contours, 0, 10000, 1, "0: no contour lines")
	editNumber("view.grid", "Grid size (px)", &settings.Grid, 0, 10000, 10, "0: no grid")

	if changed {
		a.setSettings(settings)
	}
}

// drawParams are the parameters of the project, from the config schema, and
// watching the simulation.
func (a *app) drawParams(conf *config.Config, state engine.State) {
	group, open := "", false
	for _, p := range a.panelParams(conf) {
		if p.Group != group {
			group = p.Group
			open = imgui.CollapsingHeaderTreeNodeFlagsV(group, imgui.TreeNodeFlagsNone)
			if open && group == "Simulation" {
				a.drawWatch(state)
			}
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

// drawWatch is about watching the simulation.
func (a *app) drawWatch(state engine.State) {
	settings := a.settings
	changed := imgui.Checkbox("Watch the simulation", &settings.Watch)
	imgui.SetItemTooltip("Show the landscape as it is simulated, instead of the result only")
	var edited bool
	settings.WatchSteps, edited = a.number("simulation.watchSteps", "Time steps per frame", settings.WatchSteps, 1, 1000, 1, true, false, "")
	if changed || edited {
		a.setSettings(settings)
	}

	imgui.BeginDisabledV(state.Busy)
	if imgui.ButtonV("Replay the simulation", fullWidth()) {
		a.replay()
	}
	imgui.EndDisabled()
	imgui.Separator()
}

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
