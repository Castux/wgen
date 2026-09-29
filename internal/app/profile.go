package app

import (
	"fmt"
	"math"

	"github.com/AllenDang/cimgui-go/imgui"

	"github.com/Castux/wgen/internal/gen"
	"github.com/Castux/wgen/internal/render"
)

// drawProfile is the altitude profile of the selected ruler: the ground in
// the height colors, water in blue, and the numbers of the path. Hovering
// the graph marks the point in the view.
func (a *app) drawProfile() {
	a.inspect.marker = nil
	r := a.selectedRuler()
	if r == nil || a.world == nil || a.inspect.probe == nil {
		return
	}
	points := r.points
	if a.inspect.drawing && a.inspect.hover != nil {
		points = append(points[:len(points):len(points)], a.inspect.hover.Position)
	}
	if len(points) < 2 {
		return
	}
	profile := a.inspect.probe.Profile(points)
	if !a.inspect.drawing {
		profile = *a.profileOf(r)
	}
	if len(profile.Samples) < 2 {
		return
	}

	display := imgui.CurrentIO().DisplaySize()
	viewWidth, _, _ := a.viewSize()
	imgui.SetNextWindowPosV(imgui.NewVec2(float32(viewWidth/2), display.Y-12), imgui.CondFirstUseEver, imgui.NewVec2(0.5, 1))
	imgui.SetNextWindowSizeV(imgui.NewVec2(float32(viewWidth*0.8), 260*a.uiScale), imgui.CondFirstUseEver)
	open := true
	if imgui.BeginV("Altitude profile", &open, imgui.WindowFlagsNoFocusOnAppearing) {
		keepInside(display)
		imgui.TextUnformatted(fmt.Sprintf("Length %s   Lowest %s   Highest %s   Climb %s   Descent %s",
			formatDistance(profile.Length), formatMeters(profile.Lowest), formatMeters(profile.Highest),
			formatMeters(profile.Ascent), formatMeters(profile.Descent)))
		if !a.inspect.drawing {
			imgui.SameLineV(0, 16*a.uiScale)
			if imgui.SmallButton("Delete") {
				a.deleteRuler()
				imgui.End()
				return
			}
			imgui.SetItemTooltip("Delete this ruler (Delete)")
		}
		a.drawProfileGraph(&profile)
	}
	imgui.End()
	if !open {
		a.finishRuler()
		a.inspect.selected = -1
	}
}

// Profile graph colors
var (
	graphBackground = imgui.ColorConvertFloat4ToU32(imgui.NewVec4(0.08, 0.08, 0.1, 1))
	graphGrid       = imgui.ColorConvertFloat4ToU32(imgui.NewVec4(1, 1, 1, 0.12))
	graphText       = imgui.ColorConvertFloat4ToU32(imgui.NewVec4(0.8, 0.8, 0.8, 1))
	graphLine       = imgui.ColorConvertFloat4ToU32(imgui.NewVec4(1, 1, 1, 0.9))
)

func (a *app) drawProfileGraph(profile *gen.Profile) {
	origin := imgui.CursorScreenPos()
	available := imgui.ContentRegionAvail()
	if available.X < 50 || available.Y < 40 {
		return
	}
	imgui.InvisibleButton("graph", available)
	hovered := imgui.IsItemHovered()
	drawList := imgui.WindowDrawList()

	// Margins for the labels
	left := imgui.CalcTextSize("-00000 m").X + 8*a.uiScale
	bottom := imgui.TextLineHeight() + 6*a.uiScale
	x0, y0 := origin.X+left, origin.Y+4*a.uiScale
	x1, y1 := origin.X+available.X-8*a.uiScale, origin.Y+available.Y-bottom
	drawList.AddRectFilled(imgui.NewVec2(x0, y0), imgui.NewVec2(x1, y1), graphBackground)

	// Elevations shown: the profile's, with sea level if there is water
	low, high := profile.Lowest, profile.Highest
	if profile.HasWater {
		low, high = math.Min(low, 0), math.Max(high, 0)
	}
	pad := math.Max((high-low)*0.08, 10)
	low, high = low-pad, high+pad
	toX := func(distance float64) float32 { return x0 + (x1-x0)*float32(distance/profile.Length) }
	toY := func(elevation float64) float32 { return y1 - (y1-y0)*float32((elevation-low)/(high-low)) }

	// Grid and labels
	for _, z := range niceTicks(low, high, 5) {
		y := toY(z)
		drawList.AddLine(imgui.NewVec2(x0, y), imgui.NewVec2(x1, y), graphGrid)
		label := formatMeters(z)
		drawList.AddTextVec2(imgui.NewVec2(x0-imgui.CalcTextSize(label).X-4*a.uiScale, y-imgui.TextLineHeight()/2), graphText, label)
	}
	for _, d := range niceTicks(0, profile.Length, 6) {
		x := toX(d)
		drawList.AddLine(imgui.NewVec2(x, y0), imgui.NewVec2(x, y1), graphGrid)
		label := formatDistance(d)
		drawList.AddTextVec2(imgui.NewVec2(x-imgui.CalcTextSize(label).X/2, y1+3*a.uiScale), graphText, label)
	}

	// Ground and water, column by column, overlapping by a pixel: their
	// antialiased edges would show seams
	world := a.world
	scale := a.settings.HeightScale
	samples := profile.Samples
	for i := 1; i < len(samples); i++ {
		previous, sample := samples[i-1], samples[i]
		xa, xb := toX(previous.Distance), toX(sample.Distance)+1
		groundColor := render.HeightColor(scale, math.Max(0, sample.Ground)/math.Max(world.Highest, 1))
		if sample.Ground < 0 {
			groundColor = [3]float64{0.35, 0.3, 0.25}
		}
		drawList.AddQuadFilled(
			imgui.NewVec2(xa, toY(previous.Ground)), imgui.NewVec2(xb, toY(sample.Ground)),
			imgui.NewVec2(xb, y1), imgui.NewVec2(xa, y1), packColor(groundColor, 1))
		if previous.Water && sample.Water {
			shallowness := 1 - (sample.Surface-sample.Ground)/math.Max(-world.Lowest, 1)
			drawList.AddQuadFilled(
				imgui.NewVec2(xa, toY(previous.Surface)), imgui.NewVec2(xb, toY(sample.Surface)),
				imgui.NewVec2(xb, toY(sample.Ground)), imgui.NewVec2(xa, toY(previous.Ground)), packColor(render.WaterColor(shallowness), 0.9))
		}
	}
	line := make([]imgui.Vec2, len(samples))
	for i, sample := range samples {
		line[i] = imgui.NewVec2(toX(sample.Distance), toY(sample.Surface))
	}
	drawPolyline(drawList, line, graphLine, 1.5*a.uiScale)

	// The hovered point: a line, its numbers, and a marker in the view
	if !hovered {
		return
	}
	mouse := imgui.CurrentIO().MousePos()
	distance := float64((mouse.X - x0) / (x1 - x0))
	if distance < 0 || distance > 1 {
		return
	}
	distance *= profile.Length
	closest := &samples[0]
	for i := range samples {
		if math.Abs(samples[i].Distance-distance) < math.Abs(closest.Distance-distance) {
			closest = &samples[i]
		}
	}
	x := toX(closest.Distance)
	drawList.AddLine(imgui.NewVec2(x, y0), imgui.NewVec2(x, y1), selectedColor)
	drawList.AddCircleFilled(imgui.NewVec2(x, toY(closest.Surface)), 4*a.uiScale, selectedColor)
	text := formatDistance(closest.Distance) + ": " + formatMeters(closest.Ground)
	if closest.Water {
		text = fmt.Sprintf("%s: water at %s, %s deep", formatDistance(closest.Distance),
			formatMeters(closest.Surface), formatMeters(closest.Surface-closest.Ground))
	}
	imgui.SetTooltip(noFormat(text))
	a.inspect.marker = closest
}

// niceTicks are round values between low and high, about count of them.
func niceTicks(low, high float64, count int) []float64 {
	span := high - low
	if !(span > 0) || count < 1 {
		return nil
	}
	raw := span / float64(count)
	magnitude := math.Pow(10, math.Floor(math.Log10(raw)))
	step := magnitude
	for _, factor := range []float64{1, 2, 2.5, 5, 10} {
		if step = factor * magnitude; step >= raw {
			break
		}
	}
	var ticks []float64
	for t := math.Ceil(low/step) * step; t <= high+step*1e-9; t += step {
		ticks = append(ticks, math.Round(t/step)*step+0) // +0: no negative zero
	}
	return ticks
}

func packColor(c [3]float64, alpha float32) uint32 {
	return imgui.ColorConvertFloat4ToU32(imgui.NewVec4(float32(c[0]), float32(c[1]), float32(c[2]), alpha))
}
