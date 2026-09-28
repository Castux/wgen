package app

import (
	"math"

	"github.com/AllenDang/cimgui-go/imgui"

	"github.com/Castux/wgen/internal/config"
)

// Widgets shared by the panel and the dialogs.

// widgetEdit is the value of the widget being edited, kept between frames,
// as ImGui only writes it back when it changes, and edits are committed
// when done.
type widgetEdit struct {
	key   string // of the widget, empty if none
	value float64
	text  string
	color [3]float32
}

// track records whether the widget of key, just drawn, is being edited, and
// returns it. The caller then stores its value.
func (w *widgetEdit) track(key string) bool {
	if imgui.IsItemActive() {
		w.key = key
		return true
	}
	if w.key == key {
		w.key = ""
	}
	return false
}

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
	edited := func(key string, v float64) float64 {
		if a.widget.key == key {
			return a.widget.value
		}
		return v
	}

	shown := value
	fieldWidth := width
	if slider {
		sliderKey := key + "/slider"
		fieldWidth = 70 * a.uiScale
		sliderValue := float32(edited(sliderKey, value))
		imgui.SetNextItemWidth(max(width-fieldWidth-spacing, 1))
		imgui.SliderFloatV("##slider", &sliderValue, float32(lo), float32(hi), "", imgui.SliderFlagsNoInput)
		if a.widget.track(sliderKey) {
			a.widget.value = float64(sliderValue)
			shown = roundToStep(float64(sliderValue), step)
		}
		if imgui.IsItemDeactivatedAfterEdit() {
			committed, done = clamp(roundToStep(float64(sliderValue), step)), true
		}
		imgui.SameLineV(0, spacing)
	}

	format := "%.10g"
	if integer {
		format = "%.0f"
	}
	fieldKey := key + "/field"
	fieldValue := edited(fieldKey, shown)
	imgui.SetNextItemWidth(fieldWidth)
	imgui.InputDoubleV("##field", &fieldValue, 0, 0, format, imgui.InputTextFlagsAutoSelectAll)
	if a.widget.track(fieldKey) {
		a.widget.value = fieldValue
	}
	if imgui.IsItemDeactivatedAfterEdit() {
		committed, done = clamp(fieldValue), true
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

	if a.widget.key == key {
		value = a.widget.text
	}
	imgui.InputTextWithHint(label, "", &value, imgui.InputTextFlagsNone, nil)
	if a.widget.track(key) {
		a.widget.text = value
	}
	return value, imgui.IsItemDeactivatedAfterEdit()
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
		for _, option := range options {
			if imgui.SelectableBoolV(option, option == value, imgui.SelectableFlagsNone, imgui.NewVec2(0, 0)) && option != value {
				value, changed = option, true
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

func colorVec(c config.Color) imgui.Vec4 {
	return imgui.NewVec4(float32(c[0])/255, float32(c[1])/255, float32(c[2])/255, 1)
}

// colorFloats converts a color to the components of ImGui's color widgets.
func colorFloats(c config.Color) [3]float32 {
	return [3]float32{float32(c[0]) / 255, float32(c[1]) / 255, float32(c[2]) / 255}
}

// colorBytes converts the components of ImGui's color widgets to a color.
func colorBytes(c [3]float32) config.Color {
	return config.Color{uint8(math.Round(float64(c[0]) * 255)), uint8(math.Round(float64(c[1]) * 255)), uint8(math.Round(float64(c[2]) * 255))}
}
