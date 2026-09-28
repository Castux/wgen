// Package render draws a generated world into images: a base layer (terrain
// colors or elevation), hillshading, and an overlay of rivers, contour lines
// and a grid.
//
// Images are top row first, at a scale relative to the outline image.
package render

import (
	"image"
	"image/color"
	"image/draw"
	"math"
	"runtime"
	"sync"

	"golang.org/x/image/vector"

	"github.com/Castux/wgen/internal/gen"
	"github.com/Castux/wgen/internal/geom"
)

type Base string

const (
	BaseNone    Base = "none" // white, for overlay textures
	BaseTerrain Base = "terrain"
	BaseHeight  Base = "height"
)

type Options struct {
	Scale   float64
	Base    Base
	Shading bool

	// River width in world units is (drainage / max drainage) ^ RiverPower *
	// RiverWidth
	RiverPower float64
	RiverWidth float64

	Contours float64 // elevation interval, 0 for none
	Grid     float64 // grid size in world units, 0 for none

	HeightScale string // colors of BaseHeight on land: ScaleGray (default) or ScaleRainbow
}

// MaxSize is the largest image side Render will produce.
const MaxSize = 8192

var (
	riverColor   = color.NRGBA{66, 66, 125, 255}
	contourColor = [3]float64{0.25, 0.18, 0.1}
	contourAlpha = 0.5
	gridColor    = [3]float64{0, 0, 0}
	gridAlpha    = 0.5
)

// Light direction, same as the 3D viewer
var light = func() [3]float64 {
	l := [3]float64{-1, 1, 1}
	n := math.Sqrt(3)
	return [3]float64{l[0] / n, l[1] / n, l[2] / n}
}()

// Size of the image rendered at a given scale, clamped to MaxSize.
func Size(w *gen.World, scale float64) (int, int, float64) {
	scale = math.Min(scale, MaxSize/float64(max(w.Width, w.Height)))
	return max(1, int(math.Round(float64(w.Width)*scale))),
		max(1, int(math.Round(float64(w.Height)*scale))),
		scale
}

func Render(w *gen.World, o Options) *image.RGBA {
	width, height, scale := Size(w, o.Scale)
	o.Scale = scale

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	switch o.Base {
	case BaseTerrain:
		drawTerrainColors(w, img, scale)
	case BaseHeight:
		drawHeightColors(w, img, scale, o.HeightScale)
	default:
		draw.Draw(img, img.Bounds(), image.White, image.Point{}, draw.Src)
	}

	if o.Shading && o.Base != BaseNone {
		shade(w, img, scale)
	}

	if o.Contours > 0 || o.Grid > 0 {
		drawLines(w, img, o)
	}

	if o.RiverWidth > 0 {
		drawRivers(w, img, o)
	}

	return img
}

// parallelRows calls f on bands of rows, in parallel.
func parallelRows(n int, f func(y0, y1 int)) {
	workers := min(runtime.GOMAXPROCS(0), n)
	var wg sync.WaitGroup
	for i := range workers {
		y0, y1 := n*i/workers, n*(i+1)/workers
		wg.Go(func() { f(y0, y1) })
	}
	wg.Wait()
}

// forEachPixel calls f with the world position of each pixel, in parallel.
func forEachPixel(img *image.RGBA, scale float64, f func(i int, p geom.Vec2)) {
	width, height := img.Rect.Dx(), img.Rect.Dy()
	parallelRows(height, func(y0, y1 int) {
		for row := y0; row < y1; row++ {
			y := float64(height-1-row) / scale
			for col := range width {
				f(img.PixOffset(col, row), geom.Vec2{X: float64(col) / scale, Y: y})
			}
		}
	})
}

// VertexColor is the color of a vertex in terrain mode.
func VertexColor(w *gen.World, v int32) [3]uint8 {
	if t := w.Terrain[v]; t != nil {
		return t.Color
	}
	return [3]uint8{0, 0, 0}
}

func drawTerrainColors(w *gen.World, img *image.RGBA, scale float64) {
	width, height := img.Rect.Dx(), img.Rect.Dy()
	m := w.Mesh

	colors := make([][3]float64, len(m.Points))
	for v := range m.Points {
		c := VertexColor(w, int32(v))
		colors[v] = [3]float64{float64(c[0]), float64(c[1]), float64(c[2])}
	}

	w.RasterizeTriangles(width, height, scale, func(t, x, y int, a, b, c float64) {
		tri := m.Triangles[t]
		c0, c1, c2 := colors[tri[0]], colors[tri[1]], colors[tri[2]]
		i := img.PixOffset(x, height-1-y)
		for k := range 3 {
			img.Pix[i+k] = uint8(math.Round(a*c0[k] + b*c1[k] + c*c2[k]))
		}
		img.Pix[i+3] = 255
	})
}

// Height colors of water, from the deepest to the surface
var (
	deepWater    = [3]float64{25, 45, 100}
	shallowWater = [3]float64{110, 160, 215}
)

// drawHeightColors draws elevation: from sea level to the highest point on
// land (gray or rainbow), blue in water (sea and lakes), darker when deeper,
// so that shores show.
func drawHeightColors(w *gen.World, img *image.RGBA, scale float64, heightScale string) {
	highest := math.Max(w.Highest, 1)
	forEachPixel(img, scale, func(i int, p geom.Vec2) {
		z := Sample(w, w.Heightmap, p)

		// Water where its level is above the ground (the level is per
		// triangle: not interpolated)
		if level := nearest(w, w.WaterMap, p); level > z {
			t := geom.Clamp((z-w.Lowest)/math.Max(level-w.Lowest, 1e-9), 0, 1)
			for k := range 3 {
				img.Pix[i+k] = uint8(math.Round(geom.Lerp(deepWater[k], shallowWater[k], t)))
			}
			img.Pix[i+3] = 255
			return
		}

		c := toBytes(HeightColor(heightScale, z/highest))
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c[0], c[1], c[2], 255
	})
}

// nearest is the value of a raster of the world at the pixel of p.
func nearest(w *gen.World, data []float64, p geom.Vec2) float64 {
	x := int(geom.Clamp(math.Round(p.X), 0, float64(w.Width-1)))
	y := int(geom.Clamp(math.Round(p.Y), 0, float64(w.Height-1)))
	return data[y*w.Width+x]
}

// Sample interpolates a raster of the world (such as the heightmap) at p.
func Sample(w *gen.World, data []float64, p geom.Vec2) float64 {
	x := geom.Clamp(p.X, 0, float64(w.Width-1))
	y := geom.Clamp(p.Y, 0, float64(w.Height-1))

	x0, y0 := int(x), int(y)
	x1, y1 := min(x0+1, w.Width-1), min(y0+1, w.Height-1)
	fx, fy := x-float64(x0), y-float64(y0)

	at := func(x, y int) float64 {
		v := data[y*w.Width+x]
		if math.IsNaN(v) {
			return w.Lowest
		}
		return v
	}

	return geom.Lerp(
		geom.Lerp(at(x0, y0), at(x1, y0), fx),
		geom.Lerp(at(x0, y1), at(x1, y1), fx),
		fy)
}

// shade multiplies the image by a Lambertian hillshade of the heightmap.
func shade(w *gen.World, img *image.RGBA, scale float64) {
	const ambient = 0.4
	const d = 1.0 // finite differences step, in world units
	mpp := metersPerPixel(w)

	forEachPixel(img, scale, func(i int, p geom.Vec2) {
		dx := (Sample(w, w.Heightmap, geom.Vec2{X: p.X + d, Y: p.Y}) -
			Sample(w, w.Heightmap, geom.Vec2{X: p.X - d, Y: p.Y})) / (2 * d)
		dy := (Sample(w, w.Heightmap, geom.Vec2{X: p.X, Y: p.Y + d}) -
			Sample(w, w.Heightmap, geom.Vec2{X: p.X, Y: p.Y - d})) / (2 * d)

		// Elevations may be in other units than pixels
		dx /= mpp
		dy /= mpp

		// Normal is (-dx, -dy, 1), normalized
		n := math.Sqrt(dx*dx + dy*dy + 1)
		lambert := math.Max(0, (-dx*light[0]-dy*light[1]+light[2])/n)

		// Normalized so that flat ground keeps its color
		f := (ambient + (1-ambient)*lambert) / (ambient + (1-ambient)*light[2])
		for k := range 3 {
			img.Pix[i+k] = uint8(math.Min(255, float64(img.Pix[i+k])*f))
		}
	})
}

// drawLines draws contour lines and the grid, about one world unit wide (at
// least one pixel), so that they stay visible as a texture.
func drawLines(w *gen.World, img *image.RGBA, o Options) {
	lineWidth := math.Max(1, math.Round(o.Scale)) / o.Scale

	blend := func(i int, c [3]float64, alpha float64) {
		for k := range 3 {
			v := float64(img.Pix[i+k])
			img.Pix[i+k] = uint8(math.Round(v*(1-alpha) + c[k]*v*alpha))
		}
	}

	forEachPixel(img, o.Scale, func(i int, p geom.Vec2) {
		if o.Grid > 0 {
			fx := math.Mod(p.X, o.Grid)
			fy := math.Mod(p.Y, o.Grid)
			if fx < lineWidth || fy < lineWidth {
				blend(i, gridColor, gridAlpha)
			}
		}

		// A contour crosses this pixel if the level changes within a line
		// width to the right or above
		if o.Contours > 0 {
			level := func(q geom.Vec2) float64 {
				return math.Floor(Sample(w, w.Heightmap, q) / o.Contours)
			}
			l := level(p)
			if l != level(geom.Vec2{X: p.X + lineWidth, Y: p.Y}) || l != level(geom.Vec2{X: p.X, Y: p.Y + lineWidth}) {
				blend(i, contourColor, contourAlpha)
			}
		}
	})
}

// MaxDrainage is the reference for river widths: the largest on land.
func MaxDrainage(w *gen.World) float64 {
	maxDrainage := 1.0
	for v, a := range w.Drainage {
		if w.IsLand(int32(v)) {
			maxDrainage = math.Max(maxDrainage, a)
		}
	}
	return maxDrainage
}

// metersPerPixel is the elevation unit, per horizontal unit (pixel).
func metersPerPixel(w *gen.World) float64 {
	if w.MetersPerPixel > 0 {
		return w.MetersPerPixel
	}
	return 1
}

// drawRivers draws every downhill link as an antialiased segment with round
// caps, its width growing with the drainage area.
func drawRivers(w *gen.World, img *image.RGBA, o Options) {
	width, height := img.Rect.Dx(), img.Rect.Dy()
	maxDrainage := MaxDrainage(w)
	m := w.Mesh

	// World to image coordinates, pixel centers at +0.5
	toImage := func(p geom.Vec2) geom.Vec2 {
		return geom.Vec2{X: p.X*o.Scale + 0.5, Y: float64(height-1) - p.Y*o.Scale + 0.5}
	}

	r := vector.NewRasterizer(width, height)
	visible := geom.Vec2{X: float64(width), Y: float64(height)}

	for v, d := range w.Downhill {
		if d < 0 {
			continue
		}

		radius := math.Pow(w.Drainage[v]/maxDrainage, o.RiverPower) * o.RiverWidth * o.Scale / 2
		a, b := toImage(m.Points[v]), toImage(m.Points[d])

		if math.Max(a.X, b.X) < -radius || math.Min(a.X, b.X) > visible.X+radius ||
			math.Max(a.Y, b.Y) < -radius || math.Min(a.Y, b.Y) > visible.Y+radius {
			continue
		}

		capsule(r, a, b, radius)
	}

	r.Draw(img, img.Bounds(), image.NewUniform(riverColor), image.Point{})
}

// capsule adds a segment with round caps to the rasterizer. All capsules have
// the same winding, so that overlaps don't cancel out.
func capsule(r *vector.Rasterizer, a, b geom.Vec2, radius float64) {
	const steps = 8 // per half circle

	dir := b.Sub(a)
	angle := math.Atan2(dir.Y, dir.X)

	point := func(c geom.Vec2, t float64) (float32, float32) {
		return float32(c.X + radius*math.Cos(t)), float32(c.Y + radius*math.Sin(t))
	}

	// Half circle around b, then half circle around a
	x, y := point(b, angle-math.Pi/2)
	r.MoveTo(x, y)
	for i := 1; i <= steps; i++ {
		r.LineTo(point(b, angle-math.Pi/2+math.Pi*float64(i)/steps))
	}
	for i := 0; i <= steps; i++ {
		r.LineTo(point(a, angle+math.Pi/2+math.Pi*float64(i)/steps))
	}
	r.ClosePath()
}
