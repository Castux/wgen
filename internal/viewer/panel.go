package viewer

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/AllenDang/cimgui-go/imgui"

	"github.com/Castux/wgen/internal/config"
	"github.com/Castux/wgen/internal/engine"
)

// param is a generation parameter, with its current value.
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

func (a *app) drawPanel(state engine.State) {
	display := imgui.CurrentIO().DisplaySize()
	imgui.SetNextWindowPosV(imgui.NewVec2(display.X-8, 8), imgui.CondFirstUseEver, imgui.NewVec2(1, 0))
	imgui.SetNextWindowSizeV(imgui.NewVec2(340*a.uiScale, 0), imgui.CondFirstUseEver)

	if imgui.BeginV("wgen", nil, imgui.WindowFlagsNone) {
		imgui.PushItemWidth(-150 * a.uiScale)
		a.drawViewSettings()
		a.drawActions(state)
		if conf := a.session.Engine.Config(); conf != nil {
			a.drawParams(conf)
		}
		imgui.PopItemWidth()
	}
	imgui.End()
}

func (a *app) drawViewSettings() {
	if !imgui.CollapsingHeaderTreeNodeFlagsV("View", imgui.TreeNodeFlagsDefaultOpen) {
		return
	}

	s := a.settings
	changed := false
	edit := func(c bool) { changed = changed || c }

	var c bool
	s.View, c = combo("View (tab)", s.View, views)
	edit(c)
	s.Color, c = combo("Color (shift)", s.Color, colors)
	edit(c)
	s.Shading, c = combo("Shading (q)", s.Shading, shadings)
	edit(c)
	edit(imgui.Checkbox("Wireframe (w)", &s.Wireframe))

	// Rendered on the CPU: only updated when done dragging
	s.RiverWidth, c = a.slider("view.riverWidth", "River max width", s.RiverWidth, 0, 20, 0.1)
	edit(c)
	s.RiverPower, c = a.slider("view.riverPower", "River width growth", s.RiverPower, 0, 1, 0.01)
	edit(c)
	s.Contours, c = a.slider("view.contours", "Contour interval", s.Contours, 0, 100, 1)
	edit(c)
	s.Grid, c = a.slider("view.grid", "Grid size", s.Grid, 0, 500, 10)
	edit(c)

	if changed {
		a.setSettings(s)
	}
}

func (a *app) drawActions(state engine.State) {
	if !imgui.CollapsingHeaderTreeNodeFlagsV("Actions", imgui.TreeNodeFlagsDefaultOpen) {
		return
	}

	full := imgui.NewVec2(-math.SmallestNonzeroFloat32, 0)

	label := "Save config"
	if state.Dirty {
		label = "Save config (unsaved changes)"
	}
	imgui.BeginDisabledV(!state.Dirty)
	if imgui.ButtonV(label+"###save", full) {
		a.save()
	}
	imgui.EndDisabled()

	imgui.BeginDisabledV(a.exporting.Load())
	if imgui.ButtonV("Export files", full) {
		a.export()
	}
	imgui.EndDisabled()

	if imgui.ButtonV("Reset view", full) {
		a.resetView()
	}
}

func (a *app) drawParams(conf *config.Config) {
	if !imgui.CollapsingHeaderTreeNodeFlagsV("Generation", imgui.TreeNodeFlagsDefaultOpen) {
		return
	}

	group, open := "", false
	for _, p := range a.panelParams(conf) {
		if p.Group != group {
			if open {
				imgui.TreePop()
			}
			group = p.Group
			flags := imgui.TreeNodeFlagsDefaultOpen
			if strings.HasPrefix(group, "Terrain: ") || group == "Export" {
				flags = imgui.TreeNodeFlagsNone
			}
			open = imgui.TreeNodeExStrV(group, flags)
		}
		if !open {
			continue
		}

		key := strings.Join(p.Path, ".")
		imgui.PushIDStr(key)

		var value any
		changed := false
		switch v := p.value.(type) {
		case bool:
			changed = imgui.Checkbox(p.Label, &v)
			value = v
		case string:
			value, changed = combo(p.Label, v, p.Options)
		case float64:
			value, changed = a.slider("param."+key, p.Label, v, p.Min, p.Max, p.Step)
		}
		if changed {
			a.patch(p.Path, value)
		}

		imgui.PopID()
	}
	if open {
		imgui.TreePop()
	}
}

// patch applies a parameter change, such as ["terrains", "sea", "gradient"]
// = -0.2, as the partial config {"terrains": {"sea": {"gradient": -0.2}}}.
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

// slider edits a number between lo and hi, rounded to step. The value is only
// committed when the slider is released (or typed in, with ctrl+click): the
// new value is returned, with true, then. key identifies the slider.
func (a *app) slider(key, label string, value, lo, hi, step float64) (float64, bool) {
	v := float32(value)
	if a.editKey == key {
		v = a.editValue
	}

	imgui.SliderFloatV(label, &v, float32(lo), float32(hi), stepFormat(step), imgui.SliderFlagsNone)

	if imgui.IsItemActive() {
		a.editKey, a.editValue = key, v
	} else if a.editKey == key {
		a.editKey = ""
	}

	if imgui.IsItemDeactivatedAfterEdit() {
		return roundToStep(float64(v), step), true
	}
	return value, false
}

// decimals is the number of decimals worth showing for values rounded to
// step.
func decimals(step float64) int {
	if step <= 0 {
		return 3
	}
	return max(0, int(math.Ceil(-math.Log10(step)-1e-9)))
}

func stepFormat(step float64) string { return fmt.Sprintf("%%.%df", decimals(step)) }

func roundToStep(v, step float64) float64 {
	if step <= 0 {
		return v
	}
	v = math.Round(v/step) * step

	// Without the floating point noise of the multiplication
	p := math.Pow(10, float64(decimals(step)))
	return math.Round(v*p) / p
}

func combo(label, value string, options []string) (string, bool) {
	changed := false
	if imgui.BeginCombo(label, value) {
		for _, o := range options {
			if imgui.SelectableBoolV(o, o == value, imgui.SelectableFlagsNone, imgui.NewVec2(0, 0)) && o != value {
				value, changed = o, true
			}
		}
		imgui.EndCombo()
	}
	return value, changed
}
