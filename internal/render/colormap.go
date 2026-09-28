package render

import (
	"math"

	"github.com/Castux/wgen/internal/geom"
)

// Height color scales
const (
	ScaleGray    = "gray"
	ScaleRainbow = "rainbow"
)

// Start of the rainbow scale in Turbo
const rainbowStart = 0.1

// HeightColor is the color of a land elevation, t from 0 (sea level) to 1
// (the highest point), as sRGB 0..1.
func HeightColor(scale string, t float64) [3]float64 {
	t = geom.Clamp(t, 0, 1)
	if scale == ScaleRainbow {
		// Without the darkest end, too close to black
		return Turbo(rainbowStart + (1-rainbowStart)*t)
	}
	return [3]float64{t, t, t}
}

// WaterColor is the color of a water depth, t from 0 (deepest) to 1 (the
// surface), as sRGB 0..1.
func WaterColor(t float64) [3]float64 {
	t = geom.Clamp(t, 0, 1)
	var c [3]float64
	for k := range 3 {
		c[k] = geom.Lerp(deepWater[k], shallowWater[k], t) / 255
	}
	return c
}

// Turbo is Google's improved rainbow colormap (Mikhailov, 2019), from its
// polynomial approximation: sRGB 0..1 for t in 0..1.
func Turbo(t float64) [3]float64 {
	t = geom.Clamp(t, 0, 1)
	t2, t3, t4, t5 := t*t, t*t*t, t*t*t*t, t*t*t*t*t
	r := 0.13572138 + 4.61539260*t - 42.66032258*t2 + 132.13108234*t3 - 152.94239396*t4 + 59.28637943*t5
	g := 0.09140261 + 2.19418839*t + 4.84296658*t2 - 14.18503333*t3 + 4.27729857*t4 + 2.82956604*t5
	b := 0.10667330 + 12.64194608*t - 60.58204836*t2 + 110.36276771*t3 - 89.90310912*t4 + 27.34824973*t5
	return [3]float64{geom.Clamp(r, 0, 1), geom.Clamp(g, 0, 1), geom.Clamp(b, 0, 1)}
}

// toBytes converts an sRGB 0..1 color to bytes.
func toBytes(c [3]float64) [3]uint8 {
	return [3]uint8{uint8(math.Round(c[0] * 255)), uint8(math.Round(c[1] * 255)), uint8(math.Round(c[2] * 255))}
}
