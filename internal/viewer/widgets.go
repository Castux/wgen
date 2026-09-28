package viewer

import (
	"math"

	"github.com/AllenDang/cimgui-go/imgui"
)

// Widgets shared by the panel and the dialogs.

// number edits a value in a field, committed when done (Enter pressed or
// the field left): the new value is returned, with true, then. Values are
// clamped to lo..hi, and rounded for integers. With slider, a slider is
// shown too, for quick changes (rounded to step). key identifies the
// widget. tooltip, if any, shows on the label.
func (a *app) number(key, label string, value, lo, hi, step float64, integer, slider bool, tooltip string) (float64, bool) {
	imgui.PushIDStr(key)
	defer imgui.PopID()

	spacing := imgui.CurrentStyle().ItemInnerSpacing().X
	width := imgui.CalcItemWidth()
	committed, done := value, false
	clamp := func(v float64) float64 {
		v = math.Max(lo, math.Min(hi, v))
		if integer {
			v = math.Round(v)
		}
		return v
	}

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

	shown := value
	fieldWidth := width
	if slider {
		fieldWidth = 70 * a.uiScale
		s := float32(edited("/slider", value))
		imgui.SetNextItemWidth(max(width-fieldWidth-spacing, 1))
		imgui.SliderFloatV("##slider", &s, float32(lo), float32(hi), "", imgui.SliderFlagsNoInput)
		track("/slider", float64(s))
		if imgui.IsItemDeactivatedAfterEdit() {
			committed, done = clamp(roundToStep(float64(s), step)), true
		}
		if a.editKey == key+"/slider" {
			shown = roundToStep(float64(s), step)
		}
		imgui.SameLineV(0, spacing)
	}

	format := "%.10g"
	if integer {
		format = "%.0f"
	}
	f := edited("/field", shown)
	imgui.SetNextItemWidth(fieldWidth)
	imgui.InputDoubleV("##field", &f, 0, 0, format, imgui.InputTextFlagsAutoSelectAll)
	track("/field", f)
	if imgui.IsItemDeactivatedAfterEdit() {
		committed, done = clamp(f), true
	}

	imgui.SameLineV(0, spacing)
	imgui.TextUnformatted(label)
	if tooltip != "" {
		imgui.SetItemTooltip(tooltip)
	}

	return committed, done
}

// text edits a string, committed when done: the new value is returned, with
// true, then.
func (a *app) text(key, label string, value string) (string, bool) {
	imgui.PushIDStr(key)
	defer imgui.PopID()

	if a.editKey == key {
		value = a.editText
	}
	v := value
	imgui.InputTextWithHint(label, "", &v, imgui.InputTextFlagsNone, nil)
	if imgui.IsItemActive() {
		a.editKey, a.editText = key, v
	} else if a.editKey == key {
		a.editKey = ""
	}
	return v, imgui.IsItemDeactivatedAfterEdit()
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

func fullWidth() imgui.Vec2 { return imgui.NewVec2(-math.SmallestNonzeroFloat32, 0) }

func colorVec(c [3]uint8) imgui.Vec4 {
	return imgui.NewVec4(float32(c[0])/255, float32(c[1])/255, float32(c[2])/255, 1)
}
