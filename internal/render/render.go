// Package render draws a generated world into images: a base layer (terrain
// colors or elevation), hillshading, and an overlay of rivers, contour lines
// and a grid.
//
// Images are top row first, at a scale relative to the map image.
package render

import (
	"image"
	"image/color"
	"maps"
	"math"
	"runtime"
	"slices"
	"sync"

	"golang.org/x/image/vector"

	"github.com/Castux/wgen/internal/gen"
	"github.com/Castux/wgen/internal/geom"
)

// Base is the bottom layer of an image.
type Base string

const (
	BaseNone    Base = "none" // transparent, for overlay textures
	BaseTerrain Base = "terrain"
	BaseHeight  Base = "height"
)

type Options struct {
	Scale   float64 // image pixels per world unit (map image pixel)
	Base    Base
	Shading bool // hillshading, on a base

	// River width in world units is (drainage / max drainage) ^ RiverPower *
	// RiverWidth
	RiverPower float64
	RiverWidth float64

	// Rivers in a color per drainage basin, instead of all in blue
	BasinColors bool

	Contours float64 // elevation interval, 0 for none
	Grid     float64 // grid size in world units, 0 for none

	HeightScale string // colors of BaseHeight on land: ScaleGray (default) or ScaleRainbow
}

// MaxSize is the largest image side Render will produce.
const MaxSize = 8192

var (
	riverColor   = color.NRGBA{30, 120, 255, 255} // bright: rivers show over every base
	contourColor = [3]float64{0.25, 0.18, 0.1}
	contourAlpha = 0.5
	gridColor    = [3]float64{0, 0, 0}
	gridAlpha    = 0.5
)

// Light direction, same as the 3D view
var light = func() [3]float64 {
	l := [3]float64{-1, 1, 1}
	n := math.Sqrt(3)
	return [3]float64{l[0] / n, l[1] / n, l[2] / n}
}()

// Size returns the size of the image rendered at a given scale, and the
// scale, lowered to keep the image within MaxSize.
func Size(w *gen.World, scale float64) (int, int, float64) {
	scale = math.Min(scale, MaxSize/float64(max(w.Width, w.Height)))
	return max(1, int(math.Round(float64(w.Width)*scale))),
		max(1, int(math.Round(float64(w.Height)*scale))),
		scale
}

// Render draws the layers chosen by the options: the base, hillshading, the
// grid and contour lines, then rivers.
func Render(w *gen.World, options Options) *image.RGBA {
	width, height, scale := Size(w, options.Scale)
	options.Scale = scale

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	switch options.Base {
	case BaseTerrain:
		drawTerrainColors(w, img, scale)
	case BaseHeight:
		drawHeightColors(w, img, scale, options.HeightScale)
	}
	// BaseNone: transparent, the layers drawn over it (premultiplied alpha)

	if options.Shading && options.Base != BaseNone {
		shade(w, img, scale)
	}

	if options.Contours > 0 || options.Grid > 0 {
		drawLines(w, img, options)
	}

	if options.RiverWidth > 0 {
		drawRivers(w, img, options)
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
		triangle := m.Triangles[t]
		c0, c1, c2 := colors[triangle[0]], colors[triangle[1]], colors[triangle[2]]
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
	const step = 1.0 // of the finite differences, in world units

	// Elevations are in meters, positions in pixels
	metersPerPixel := 1.0
	if w.MetersPerPixel > 0 {
		metersPerPixel = w.MetersPerPixel
	}

	forEachPixel(img, scale, func(i int, p geom.Vec2) {
		dx := (Sample(w, w.Heightmap, geom.Vec2{X: p.X + step, Y: p.Y}) -
			Sample(w, w.Heightmap, geom.Vec2{X: p.X - step, Y: p.Y})) / (2 * step)
		dy := (Sample(w, w.Heightmap, geom.Vec2{X: p.X, Y: p.Y + step}) -
			Sample(w, w.Heightmap, geom.Vec2{X: p.X, Y: p.Y - step})) / (2 * step)
		dx /= metersPerPixel
		dy /= metersPerPixel

		// Normal is (-dx, -dy, 1), normalized
		length := math.Sqrt(dx*dx + dy*dy + 1)
		lambert := math.Max(0, (-dx*light[0]-dy*light[1]+light[2])/length)

		// Normalized so that flat ground keeps its color
		factor := (ambient + (1-ambient)*lambert) / (ambient + (1-ambient)*light[2])
		for k := range 3 {
			img.Pix[i+k] = uint8(math.Min(255, float64(img.Pix[i+k])*factor))
		}
	})
}

// drawLines draws contour lines and the grid, about one world unit wide (at
// least one pixel), so that they stay visible as a texture.
func drawLines(w *gen.World, img *image.RGBA, options Options) {
	lineWidth := math.Max(1, math.Round(options.Scale)) / options.Scale

	// Darkening a base, toward its color times the line's; over a
	// transparent overlay, the line's color with its alpha, which darkens
	// the same way what the overlay is drawn over
	blend := func(i int, c [3]float64, alpha float64) {
		if options.Base == BaseNone {
			for k := range 3 {
				img.Pix[i+k] = uint8(math.Round(c[k]*255*alpha + float64(img.Pix[i+k])*(1-alpha)))
			}
			img.Pix[i+3] = uint8(math.Round(255*alpha + float64(img.Pix[i+3])*(1-alpha)))
			return
		}
		for k := range 3 {
			v := float64(img.Pix[i+k])
			img.Pix[i+k] = uint8(math.Round(v*(1-alpha) + c[k]*v*alpha))
		}
	}

	// Contour level of a point
	level := func(p geom.Vec2) float64 {
		return math.Floor(Sample(w, w.Heightmap, p) / options.Contours)
	}

	forEachPixel(img, options.Scale, func(i int, p geom.Vec2) {
		if options.Grid > 0 {
			if math.Mod(p.X, options.Grid) < lineWidth || math.Mod(p.Y, options.Grid) < lineWidth {
				blend(i, gridColor, gridAlpha)
			}
		}

		// A contour crosses this pixel if the level changes within a line
		// width to the right or above
		if options.Contours > 0 {
			here := level(p)
			if here != level(geom.Vec2{X: p.X + lineWidth, Y: p.Y}) || here != level(geom.Vec2{X: p.X, Y: p.Y + lineWidth}) {
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

// drawRivers draws every downhill link as an antialiased segment with round
// caps, its width growing with the drainage area: in blue, or in a color per
// drainage basin.
func drawRivers(w *gen.World, img *image.RGBA, options Options) {
	width, height := img.Rect.Dx(), img.Rect.Dy()
	maxDrainage := MaxDrainage(w)
	m := w.Mesh

	// World to image coordinates, pixel centers at +0.5
	toImage := func(p geom.Vec2) geom.Vec2 {
		return geom.Vec2{X: p.X*options.Scale + 0.5, Y: float64(height-1) - p.Y*options.Scale + 0.5}
	}

	// The visible segments, by basin
	var outlets []int32
	if options.BasinColors {
		outlets = w.Outlets()
	}
	type segment struct {
		a, b   geom.Vec2
		radius float64
	}
	byBasin := map[int32][]segment{}
	visible := geom.Vec2{X: float64(width), Y: float64(height)}
	for v, d := range w.Downhill {
		if d < 0 {
			continue
		}
		radius := math.Pow(w.Drainage[v]/maxDrainage, options.RiverPower) * options.RiverWidth * options.Scale / 2
		a, b := toImage(m.Points[v]), toImage(m.Points[d])
		if math.Max(a.X, b.X) < -radius || math.Min(a.X, b.X) > visible.X+radius ||
			math.Max(a.Y, b.Y) < -radius || math.Min(a.Y, b.Y) > visible.Y+radius {
			continue
		}
		basin := int32(-1)
		if outlets != nil {
			basin = outlets[v]
		}
		byBasin[basin] = append(byBasin[basin], segment{a, b, radius})
	}

	// Each basin rasterized over its bounds only: a rasterizer has a buffer
	// of its size
	// In a stable order, for overlaps
	rasterizer := vector.NewRasterizer(1, 1)
	for _, basin := range slices.Sorted(maps.Keys(byBasin)) {
		segments := byBasin[basin]
		bounds := image.Rectangle{}
		for i, s := range segments {
			r := image.Rect(int(math.Floor(math.Min(s.a.X, s.b.X)-s.radius)), int(math.Floor(math.Min(s.a.Y, s.b.Y)-s.radius)),
				int(math.Ceil(math.Max(s.a.X, s.b.X)+s.radius))+1, int(math.Ceil(math.Max(s.a.Y, s.b.Y)+s.radius))+1)
			if i == 0 {
				bounds = r
			} else {
				bounds = bounds.Union(r)
			}
		}
		bounds = bounds.Intersect(img.Rect)
		if bounds.Empty() {
			continue
		}

		rasterizer.Reset(bounds.Dx(), bounds.Dy())
		offset := geom.Vec2{X: float64(bounds.Min.X), Y: float64(bounds.Min.Y)}
		for _, s := range segments {
			capsule(rasterizer, s.a.Sub(offset), s.b.Sub(offset), s.radius)
		}
		fill := riverColor
		if basin >= 0 {
			fill = BasinColor(basin)
		}
		rasterizer.Draw(img, bounds, image.NewUniform(fill), image.Point{})
	}
}

// BasinColor is the color of the rivers of a basin, given by its outlet:
// a hue from a hash of it, so that neighbouring basins likely differ,
// saturated and bright, to show over the terrain.
func BasinColor(outlet int32) color.NRGBA {
	h := uint64(outlet) + 0x9e3779b97f4a7c15
	h = (h ^ h>>30) * 0xbf58476d1ce4e5b9
	h = (h ^ h>>27) * 0x94d049bb133111eb
	h ^= h >> 31
	hue := float64(h>>11) / (1 << 53)
	saturation := 0.7 + 0.3*float64(h&0xff)/255
	c := hsv(hue, saturation, 1)
	return color.NRGBA{uint8(math.Round(c[0] * 255)), uint8(math.Round(c[1] * 255)), uint8(math.Round(c[2] * 255)), 255}
}

// hsv converts a color, hue in 0..1, to RGB 0..1.
func hsv(hue, saturation, value float64) [3]float64 {
	sector := hue * 6
	i := math.Floor(sector)
	f := sector - i
	p, q, t := value*(1-saturation), value*(1-saturation*f), value*(1-saturation*(1-f))
	switch int(i) % 6 {
	case 0:
		return [3]float64{value, t, p}
	case 1:
		return [3]float64{q, value, p}
	case 2:
		return [3]float64{p, value, t}
	case 3:
		return [3]float64{p, q, value}
	case 4:
		return [3]float64{t, p, value}
	}
	return [3]float64{value, p, q}
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
