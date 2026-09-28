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

	s := a.settings
	changed := false
	edit := func(c bool) { changed = changed || c }

	var c bool
	s.View, c = combo("View (v)", s.View, views)
	edit(c)
	s.Color, c = combo("Colors (shift)", s.Color, colors)
	edit(c)
	s.HeightScale, c = combo("Height scale", s.HeightScale, heightScale)
	edit(c)
	edit(imgui.Checkbox("Height legend", &s.Legend))
	s.Shading, c = combo("Shading (q)", s.Shading, shadings)
	edit(c)
	edit(imgui.Checkbox("Wireframe (w)", &s.Wireframe))
	s.VerticalScale, c = a.number("view.verticalScale", "Vertical exaggeration", s.VerticalScale, 0.1, 100, 0.5, false, false,
		"Of the 3D view: at 1, mountains have their true proportions")
	edit(c)

	imgui.SeparatorText("Overlay")
	s.RiverWidth, c = a.number("view.riverWidth", "River width (px)", s.RiverWidth, 0, 100, 0.1, false, false,
		"Width of the largest river, in map pixels (0: no rivers)")
	edit(c)
	s.RiverPower, c = a.number("view.riverPower", "River width growth", s.RiverPower, 0, 1, 0.01, false, false,
		"How much wider big rivers are than small ones")
	edit(c)
	s.Contours, c = a.number("view.contours", "Contour interval (m)", s.Contours, 0, 10000, 1, false, false, "0: no contour lines")
	edit(c)
	s.Grid, c = a.number("view.grid", "Grid size (px)", s.Grid, 0, 10000, 10, false, false, "0: no grid")
	edit(c)

	if changed {
		a.setSettings(s)
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
	s := a.settings
	changed := imgui.Checkbox("Watch the simulation", &s.Watch)
	imgui.SetItemTooltip("Show the landscape as it is simulated, instead of the result only")
	var c bool
	s.WatchSteps, c = a.number("simulation.watchSteps", "Time steps per frame", s.WatchSteps, 1, 1000, 1, true, false, "")
	if changed || c {
		a.setSettings(s)
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
		s := a.settings
		s.Watch = true
		a.setSettings(s)
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
	w := a.world
	if !a.settings.Legend || a.settings.Color != "height" || w == nil {
		return
	}

	// Top left, under the menu
	imgui.SetNextWindowPosV(imgui.NewVec2(8, a.menuHeight+8), imgui.CondAlways, imgui.NewVec2(0, 0))
	imgui.SetNextWindowBgAlpha(0.7)
	flags := imgui.WindowFlagsNoDecoration | imgui.WindowFlagsAlwaysAutoResize | imgui.WindowFlagsNoSavedSettings |
		imgui.WindowFlagsNoFocusOnAppearing | imgui.WindowFlagsNoNav | imgui.WindowFlagsNoInputs

	if imgui.BeginV("##legend", nil, flags) {
		barWidth, barHeight := 18*a.uiScale, 180*a.uiScale
		draw := imgui.WindowDrawList()

		bar := func(title string, low, high float64, color func(t float64) [3]float64) {
			imgui.TextUnformatted(title)
			origin := imgui.CursorScreenPos()
			const steps = 48
			for i := range steps {
				t0, t1 := float64(i)/steps, float64(i+1)/steps
				c := color((t0 + t1) / 2)
				col := imgui.ColorConvertFloat4ToU32(imgui.NewVec4(float32(c[0]), float32(c[1]), float32(c[2]), 1))
				// Highest at the top
				y0 := origin.Y + barHeight*float32(1-t1)
				y1 := origin.Y + barHeight*float32(1-t0)
				draw.AddRectFilled(imgui.NewVec2(origin.X, y0), imgui.NewVec2(origin.X+barWidth, y1), col)
			}
			for i := range 5 {
				t := float64(i) / 4
				y := origin.Y + barHeight*float32(1-t) - imgui.TextLineHeight()/2
				draw.AddTextVec2(imgui.NewVec2(origin.X+barWidth+6*a.uiScale, y), 0xffeeeeee, formatMeters(low+(high-low)*t))
			}
			imgui.Dummy(imgui.NewVec2(barWidth+70*a.uiScale, barHeight))
		}

		scale := a.settings.HeightScale
		bar("Land", 0, w.Highest, func(t float64) [3]float64 { return render.HeightColor(scale, t) })
		if w.Lowest < 0 {
			imgui.Spacing()
			bar("Water", w.Lowest, 0, render.WaterColor)
		}
	}
	imgui.End()
}
