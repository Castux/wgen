// Package geom provides the small vector types used throughout wgen.
package geom

import "math"

type Vec2 struct{ X, Y float64 }

func (a Vec2) Add(b Vec2) Vec2      { return Vec2{a.X + b.X, a.Y + b.Y} }
func (a Vec2) Sub(b Vec2) Vec2      { return Vec2{a.X - b.X, a.Y - b.Y} }
func (a Vec2) Scale(s float64) Vec2 { return Vec2{a.X * s, a.Y * s} }
func (a Vec2) Dist(b Vec2) float64  { return math.Hypot(a.X-b.X, a.Y-b.Y) }
func Cross2(a, b Vec2) float64      { return a.X*b.Y - a.Y*b.X }

// Circumcenter of the triangle abc.
func Circumcenter(a, b, c Vec2) Vec2 {
	dx, dy := b.X-a.X, b.Y-a.Y
	ex, ey := c.X-a.X, c.Y-a.Y

	bl := dx*dx + dy*dy
	cl := ex*ex + ey*ey
	d := 0.5 / (dx*ey - dy*ex)

	return Vec2{
		a.X + (ey*bl-dy*cl)*d,
		a.Y + (dx*cl-ex*bl)*d,
	}
}

// PseudoAngle is a monotonic function of the angle of p, in [0, 1). Cheaper
// than atan2 and good enough for sorting points around a center.
func PseudoAngle(p Vec2) float64 {
	a := p.X / (math.Abs(p.X) + math.Abs(p.Y))
	if p.Y > 0 {
		return (3 - a) / 4
	}
	return (1 + a) / 4
}

// Barycentric coordinates of p in the triangle abc.
func Barycentric(a, b, c, p Vec2) (float64, float64, float64) {
	x := Cross2(b.Sub(p), c.Sub(p))
	y := Cross2(c.Sub(p), a.Sub(p))
	z := Cross2(a.Sub(p), b.Sub(p))
	s := x + y + z
	return x / s, y / s, z / s
}

func Lerp(a, b, x float64) float64 { return a*(1-x) + b*x }

func Clamp(x, lo, hi float64) float64 { return math.Max(lo, math.Min(hi, x)) }
