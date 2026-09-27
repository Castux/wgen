package viewer

import (
	"encoding/json"
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
	// Top right, on first use
	display := imgui.CurrentIO().DisplaySize()
	width := 400 * a.uiScale
	imgui.SetNextWindowPosV(imgui.NewVec2(display.X-width-8, 8), imgui.CondFirstUseEver, imgui.NewVec2(0, 0))
	imgui.SetNextWindowSizeV(imgui.NewVec2(width, display.Y-16), imgui.CondFirstUseEver)

	if imgui.BeginV("wgen", nil, imgui.WindowFlagsNone) {
		keepInside(display)
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

// keepInside moves (and shrinks, if needed) the current window back inside
// the display, when the display got smaller than when the window was placed:
// its position is saved between sessions.
func keepInside(display imgui.Vec2) {
	if imgui.IsMouseDragging(imgui.MouseButtonLeft) {
		return
	}
	pos, size := imgui.WindowPos(), imgui.WindowSize()
	if size.X > display.X || size.Y > display.Y {
		size = imgui.NewVec2(min(size.X, display.X), min(size.Y, display.Y))
		imgui.SetWindowSizeVec2(size)
	}
	x := max(min(pos.X, display.X-size.X), 0)
	y := max(min(pos.Y, display.Y-size.Y), 0)
	if x != pos.X || y != pos.Y {
		imgui.SetWindowPosVec2(imgui.NewVec2(x, y))
	}
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

	// Rendered on the CPU: only updated when done editing
	s.RiverWidth, c = a.number("view.riverWidth", "River max width", s.RiverWidth, 0, 20, 0.1, false)
	edit(c)
	s.RiverPower, c = a.number("view.riverPower", "River width growth", s.RiverPower, 0, 1, 0.01, false)
	edit(c)
	s.Contours, c = a.number("view.contours", "Contour interval", s.Contours, 0, 100, 1, false)
	edit(c)
	s.Grid, c = a.number("view.grid", "Grid size", s.Grid, 0, 500, 10, false)
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
			value, changed = a.number("param."+key, p.Label, v, p.Min, p.Max, p.Step, p.Type == "int")
		}
		if changed && value != p.value {
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

// number edits a value with a slider, for quick changes between lo and hi
// (rounded to step), and a field next to it, for precise values (as typed,
// and possibly out of the slider range). Both only commit when done: when
// the slider is released, or Enter pressed or the field left. The new value
// is returned, with true, then. key identifies the widget.
func (a *app) number(key, label string, value, lo, hi, step float64, integer bool) (float64, bool) {
	imgui.PushIDStr(key)
	defer imgui.PopID()

	spacing := imgui.CurrentStyle().ItemInnerSpacing().X
	fieldWidth := 80 * a.uiScale
	sliderWidth := max(imgui.CalcItemWidth()-fieldWidth-spacing, 1)

	committed, done := value, false

	// The value being edited is kept between frames, as ImGui only writes
	// it back when it changes
	edited := func(id string, v float64) float64 {
		if a.editKey == key+id {
			return a.editValue
		}
		return v
	}
	track := func(id string, v float64) {
		if imgui.IsItemActive() {
			a.editKey, a.editValue = key+id, v
		} else if a.editKey == key+id {
			a.editKey = ""
		}
	}

	s := float32(edited("/slider", value))
	imgui.SetNextItemWidth(sliderWidth)
	imgui.SliderFloatV("##slider", &s, float32(lo), float32(hi), "", imgui.SliderFlagsNoInput)
	track("/slider", float64(s))
	if imgui.IsItemDeactivatedAfterEdit() {
		committed, done = roundToStep(float64(s), step), true
	}

	imgui.SameLineV(0, spacing)

	format := "%.10g"
	if integer {
		format = "%.0f"
	}
	// Follows the slider while dragged
	shown := value
	if a.editKey == key+"/slider" {
		shown = roundToStep(float64(s), step)
	}
	f := edited("/field", shown)
	imgui.SetNextItemWidth(fieldWidth)
	imgui.InputDoubleV("##field", &f, 0, 0, format, imgui.InputTextFlagsAutoSelectAll)
	track("/field", f)
	if imgui.IsItemDeactivatedAfterEdit() {
		committed, done = f, true
		if integer {
			committed = math.Round(f)
		}
	}

	imgui.SameLineV(0, spacing)
	imgui.TextUnformatted(label)

	return committed, done
}

// decimals is the number of decimals worth showing for values rounded to
// step.
func decimals(step float64) int {
	if step <= 0 {
		return 3
	}
	return max(0, int(math.Ceil(-math.Log10(step)-1e-9)))
}

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
