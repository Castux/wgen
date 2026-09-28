package viewer

import (
	"image"
	"math"
	"slices"

	"github.com/Castux/wgen/internal/config"
	"github.com/Castux/wgen/internal/gen"
	"github.com/Castux/wgen/internal/geom"
)

// canvas is the map being edited: a terrain color per pixel, bottom row
// first (as gen.World.Map). Positions are in image coordinates: x to the
// right, rows from the top.
type canvas struct {
	width, height int
	pixels        []config.Color

	// Stroke in progress
	stroke *stroke

	// protect tells which pixels stamps must leave alone (nil: none)
	protect func(config.Color) bool

	undo, redo []edit
}

// edit is an undoable change: the pixels of a rectangle before and after.
type edit struct {
	rect          image.Rectangle
	before, after []config.Color
}

type stroke struct {
	before       []config.Color // all pixels, at the start
	rect         image.Rectangle
	lastX, lastY float64 // last stamp
	shape        string
}

const maxUndo = 50

func newCanvas(width, height int, pixels []config.Color) *canvas {
	return &canvas{width: width, height: height, pixels: slices.Clone(pixels)}
}

func (c *canvas) index(x, row int) int { return (c.height-1-row)*c.width + x }

func (c *canvas) at(x, row int) config.Color { return c.pixels[c.index(x, row)] }

func (c *canvas) bounds() image.Rectangle { return image.Rect(0, 0, c.width, c.height) }

// stamp paints a shape of the given color and radius (pixels) around a
// point, and returns the rectangle of the pixels it may have changed:
//
//   - natural: a blob with a noisy edge, a little elongated, differently for
//     every seed
//   - round: a disk
//   - square: a square, of side twice the radius
func (c *canvas) stamp(cx, cy, radius float64, color config.Color, seed uint64, shape string) image.Rectangle {
	paint := func(x, row int) {
		i := c.index(x, row)
		if c.protect == nil || !c.protect(c.pixels[i]) {
			c.pixels[i] = color
		}
	}

	if shape == brushRound || shape == brushSquare {
		rect := image.Rect(int(math.Floor(cx-radius)), int(math.Floor(cy-radius)),
			int(math.Ceil(cx+radius)), int(math.Ceil(cy+radius))).Intersect(c.bounds())
		for row := rect.Min.Y; row < rect.Max.Y; row++ {
			for x := rect.Min.X; x < rect.Max.X; x++ {
				dx, dy := float64(x)+0.5-cx, float64(row)+0.5-cy
				if shape == brushSquare || dx*dx+dy*dy <= radius*radius {
					paint(x, row)
				}
			}
		}
		return rect
	}

	angle := unitRandom(seed, 1) * 2 * math.Pi
	elongation := 1 + 0.5*unitRandom(seed, 2)
	noiseX, noiseY := unitRandom(seed, 3)*1000, unitRandom(seed, 4)*1000
	cos, sin := math.Cos(angle), math.Sin(angle)

	reach := radius * 1.8
	rect := image.Rect(int(cx-reach), int(cy-reach), int(cx+reach)+1, int(cy+reach)+1).Intersect(c.bounds())

	if rect.Empty() {
		return rect
	}

	// Edge distorted by noise at the scale of the stamp. The noise is smooth
	// at that scale: sampled on a grid of about 32 steps per radius, and
	// interpolated, for big stamps to stay fast.
	noise := func(x, y float64) float64 {
		n := 0.0
		for o, amplitude, scale := 0, 1.0, 1.2/radius; o < 3; o, amplitude, scale = o+1, amplitude/2, scale*2 {
			n += amplitude * gen.Noise(seed+uint64(o), x*scale+noiseX, y*scale+noiseY)
		}
		return n
	}
	step := max(1, int(radius/32))
	gridWidth, gridHeight := rect.Dx()/step+2, rect.Dy()/step+2
	grid := make([]float64, gridWidth*gridHeight)
	for j := range gridHeight {
		for i := range gridWidth {
			grid[j*gridWidth+i] = noise(float64(rect.Min.X+i*step), float64(rect.Min.Y+j*step))
		}
	}

	for row := rect.Min.Y; row < rect.Max.Y; row++ {
		j, fy := (row-rect.Min.Y)/step, float64((row-rect.Min.Y)%step)/float64(step)
		for x := rect.Min.X; x < rect.Max.X; x++ {
			dx, dy := float64(x)+0.5-cx, float64(row)+0.5-cy
			u := (dx*cos + dy*sin) / elongation
			v := -dx*sin + dy*cos
			d := math.Hypot(u, v) / radius

			i, fx := (x-rect.Min.X)/step, float64((x-rect.Min.X)%step)/float64(step)
			n := geom.Lerp(
				geom.Lerp(grid[j*gridWidth+i], grid[j*gridWidth+i+1], fx),
				geom.Lerp(grid[(j+1)*gridWidth+i], grid[(j+1)*gridWidth+i+1], fx),
				fy)
			if d < 0.85+0.6*n {
				paint(x, row)
			}
		}
	}
	return rect
}

// unitRandom is a pseudo random number in [0, 1), the i-th of a seed.
func unitRandom(seed, i uint64) float64 { return float64(hashSeed(seed, i)>>11) / (1 << 53) }

func hashSeed(seed, i uint64) uint64 {
	h := seed*0x9e3779b97f4a7c15 + i*0xbf58476d1ce4e5b9
	h ^= h >> 31
	h *= 0x94d049bb133111eb
	h ^= h >> 29
	return h
}

// beginStroke starts a stroke with a stamp at a point.
func (c *canvas) beginStroke(x, y, radius float64, color config.Color, seed uint64, shape string) image.Rectangle {
	c.stroke = &stroke{before: slices.Clone(c.pixels), lastX: x, lastY: y, shape: shape}
	r := c.stamp(x, y, radius, color, seed, shape)
	c.stroke.rect = r
	return r
}

// continueStroke stamps along the way to a point, every fraction of the
// radius. seed gives the seed of each stamp.
func (c *canvas) continueStroke(x, y, radius float64, color config.Color, seed func() uint64) image.Rectangle {
	s := c.stroke
	if s == nil {
		return image.Rectangle{}
	}

	spacing := math.Max(1, radius*0.4)
	if s.shape != brushNatural {
		spacing = math.Max(1, radius*0.25)
	}
	var changed image.Rectangle
	for {
		dx, dy := x-s.lastX, y-s.lastY
		d := math.Hypot(dx, dy)
		if d < spacing {
			break
		}
		s.lastX += dx / d * spacing
		s.lastY += dy / d * spacing
		changed = changed.Union(c.stamp(s.lastX, s.lastY, radius, color, seed(), s.shape))
	}
	s.rect = s.rect.Union(changed)
	return changed
}

// endStroke finishes the stroke, making it undoable. It returns whether it
// changed anything.
func (c *canvas) endStroke() bool {
	s := c.stroke
	c.stroke = nil
	if s == nil || s.rect.Empty() {
		return false
	}

	e := edit{rect: s.rect, before: c.region(s.before, s.rect), after: c.region(c.pixels, s.rect)}
	if slices.Equal(e.before, e.after) {
		return false
	}
	c.undo = append(c.undo, e)
	if len(c.undo) > maxUndo {
		c.undo = c.undo[1:]
	}
	c.redo = nil
	return true
}

// region copies the pixels of a rectangle, row by row from the top.
func (c *canvas) region(pixels []config.Color, r image.Rectangle) []config.Color {
	out := make([]config.Color, 0, r.Dx()*r.Dy())
	for row := r.Min.Y; row < r.Max.Y; row++ {
		i := c.index(r.Min.X, row)
		out = append(out, pixels[i:i+r.Dx()]...)
	}
	return out
}

func (c *canvas) setRegion(r image.Rectangle, values []config.Color) {
	for row := r.Min.Y; row < r.Max.Y; row++ {
		i := c.index(r.Min.X, row)
		copy(c.pixels[i:i+r.Dx()], values[(row-r.Min.Y)*r.Dx():])
	}
}

// undoEdit reverts the last edit, and returns its rectangle (empty if
// there was none).
func (c *canvas) undoEdit() image.Rectangle {
	if len(c.undo) == 0 || c.stroke != nil {
		return image.Rectangle{}
	}
	e := c.undo[len(c.undo)-1]
	c.undo = c.undo[:len(c.undo)-1]
	c.setRegion(e.rect, e.before)
	c.redo = append(c.redo, e)
	return e.rect
}

// redoEdit applies the last undone edit again.
func (c *canvas) redoEdit() image.Rectangle {
	if len(c.redo) == 0 || c.stroke != nil {
		return image.Rectangle{}
	}
	e := c.redo[len(c.redo)-1]
	c.redo = c.redo[:len(c.redo)-1]
	c.setRegion(e.rect, e.after)
	c.undo = append(c.undo, e)
	return e.rect
}

// rgba returns the pixels of a rectangle as RGBA, rows from the top.
func (c *canvas) rgba(r image.Rectangle) []byte {
	out := make([]byte, 0, 4*r.Dx()*r.Dy())
	for row := r.Min.Y; row < r.Max.Y; row++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			p := c.at(x, row)
			out = append(out, p[0], p[1], p[2], 255)
		}
	}
	return out
}

// recolor replaces a color by another, in the map and its undo history: a
// terrain changed color, or was removed.
func (c *canvas) recolor(from, to config.Color) {
	replace := func(pixels []config.Color) {
		for i, p := range pixels {
			if p == from {
				pixels[i] = to
			}
		}
	}
	replace(c.pixels)
	for _, history := range [][]edit{c.undo, c.redo} {
		for _, e := range history {
			replace(e.before)
			replace(e.after)
		}
	}
}

// island paints a starting island in the middle of the map: land of the
// given colors, from the lowest to the highest, in smaller and smaller
// natural blobs.
func (c *canvas) island(land []config.Color, seed uint64) {
	cx, cy := float64(c.width)/2, float64(c.height)/2
	size := math.Min(float64(c.width), float64(c.height))

	for k, color := range land {
		// Each level: a few blobs around the center, smaller
		radius := size * 0.22 * math.Pow(0.55, float64(k))
		for i := range uint64(6) {
			angle := unitRandom(seed, uint64(k)*100+i) * 2 * math.Pi
			distance := radius * 0.8 * unitRandom(seed, uint64(k)*100+i+50)
			x, y := cx+distance*math.Cos(angle)*float64(c.width)/size, cy+distance*math.Sin(angle)
			c.stamp(x, y, radius, color, seed+uint64(k)*10+i, brushNatural)
		}
	}
}
