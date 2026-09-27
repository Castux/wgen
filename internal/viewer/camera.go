package viewer

import (
	"math"

	"github.com/go-gl/mathgl/mgl64"
)

// Cameras of the 3D views. World coordinates have z up, the map centered on
// the origin. Mouse movements are in window coordinates (y down), and the
// speeds are those of three.js OrbitControls, which the previous web viewer
// used.

// orbitCamera is a perspective camera orbiting around a target.
type orbitCamera struct {
	target mgl64.Vec3

	// Position around the target: phi is the angle from the z axis, theta
	// the angle around it, 0 when looking toward +y.
	radius, phi, theta float64

	maxDistance float64
	far         float64
}

const orbitFOV = 60.0 // vertical, degrees

// reset frames a map of the given size.
func (c *orbitCamera) reset(width, height float64) {
	extent := math.Max(width, height)
	c.target = mgl64.Vec3{}
	c.setPosition(mgl64.Vec3{0, -height, extent})
	c.maxDistance = extent * 1.5
	c.far = extent * 2.25
}

func (c *orbitCamera) setPosition(p mgl64.Vec3) {
	offset := p.Sub(c.target)
	c.radius = offset.Len()
	c.phi = math.Acos(mgl64.Clamp(offset.Z()/c.radius, -1, 1))
	c.theta = math.Atan2(offset.X(), -offset.Y())
}

func (c *orbitCamera) position() mgl64.Vec3 {
	s := math.Sin(c.phi)
	return c.target.Add(mgl64.Vec3{
		c.radius * s * math.Sin(c.theta),
		-c.radius * s * math.Cos(c.theta),
		c.radius * math.Cos(c.phi),
	})
}

func (c *orbitCamera) view() mgl64.Mat4 {
	return mgl64.LookAtV(c.position(), c.target, mgl64.Vec3{0, 0, 1})
}

// projection has its near plane following the distance to the target, so
// that close ups work (true scale mountains are small at the map's scale).
func (c *orbitCamera) projection(aspect float64) mgl64.Mat4 {
	near := mgl64.Clamp(c.radius/1000, 0.001, 1)
	return mgl64.Perspective(mgl64.DegToRad(orbitFOV), aspect, near, c.far)
}

// Closest distance to the target, in map units
const orbitMinDistance = 0.05

// rotate orbits for a mouse movement, in a view of the given height.
func (c *orbitCamera) rotate(dx, dy, viewHeight float64) {
	const eps = 1e-6
	c.theta -= 2 * math.Pi * dx / viewHeight
	c.phi = mgl64.Clamp(c.phi-2*math.Pi*dy/viewHeight, eps, math.Pi-eps)
}

// pan moves the target in the screen plane, following the mouse.
func (c *orbitCamera) pan(dx, dy, viewHeight float64) {
	// World units per pixel at the target distance
	scale := 2 * c.radius * math.Tan(mgl64.DegToRad(orbitFOV)/2) / viewHeight

	forward := c.target.Sub(c.position()).Normalize()
	right := forward.Cross(mgl64.Vec3{0, 0, 1})
	if right.Len() < 1e-9 {
		right = mgl64.Vec3{math.Cos(c.theta), math.Sin(c.theta), 0}
	}
	right = right.Normalize()
	up := right.Cross(forward)

	c.target = c.target.Sub(right.Mul(dx * scale)).Add(up.Mul(dy * scale))
}

// dolly moves toward the target (factor < 1) or away from it.
func (c *orbitCamera) dolly(factor float64) {
	c.radius = mgl64.Clamp(c.radius*factor, orbitMinDistance, c.maxDistance)
}

// topCamera is an orthographic camera looking down.
type topCamera struct {
	center        mgl64.Vec2
	zoom          float64
	width, height float64 // map size
}

func (c *topCamera) reset(width, height float64) {
	*c = topCamera{zoom: 1, width: width, height: height}
}

// halfSize is the half extent of the view, fitting the map at zoom 1.
func (c *topCamera) halfSize(aspect float64) (float64, float64) {
	half := math.Max(c.height, c.width/aspect) / 2 / c.zoom
	return half * aspect, half
}

func (c *topCamera) view() mgl64.Mat4 {
	extent := math.Max(c.width, c.height)
	return mgl64.Translate3D(-c.center.X(), -c.center.Y(), -extent)
}

func (c *topCamera) projection(aspect float64) mgl64.Mat4 {
	hx, hy := c.halfSize(aspect)
	extent := math.Max(c.width, c.height)
	return mgl64.Ortho(-hx, hx, -hy, hy, 0, 2*extent)
}

// pan follows the mouse, in a view of the given size.
func (c *topCamera) pan(dx, dy, viewWidth, viewHeight float64) {
	hx, hy := c.halfSize(viewWidth / viewHeight)
	c.center = c.center.Sub(mgl64.Vec2{dx * 2 * hx / viewWidth, -dy * 2 * hy / viewHeight})
}

// dolly zooms in (factor < 1) or out.
func (c *topCamera) dolly(factor float64) {
	c.zoom /= factor
}

// mapCamera is the 2D view transform: the top left corner of the map is
// displayed at offset, in window coordinates, and the map is zoom times its
// size.
type mapCamera struct {
	zoom          float64 // window pixels per world unit
	offset        mgl64.Vec2
	width, height float64 // map size
}

// fitZoom shows the whole map in a view of the given size.
func (c *mapCamera) fitZoom(viewWidth, viewHeight float64) float64 {
	return math.Min(viewWidth/c.width, viewHeight/c.height)
}

func (c *mapCamera) fit(viewWidth, viewHeight float64) {
	c.zoom = c.fitZoom(viewWidth, viewHeight)
	c.offset = mgl64.Vec2{(viewWidth - c.width*c.zoom) / 2, (viewHeight - c.height*c.zoom) / 2}
}

// zoomAt zooms by factor, keeping the point under the cursor in place.
func (c *mapCamera) zoomAt(x, y, factor, viewWidth, viewHeight float64) {
	zoom := mgl64.Clamp(c.zoom*factor, c.fitZoom(viewWidth, viewHeight)/4, 64)
	factor = zoom / c.zoom

	c.offset = mgl64.Vec2{x - (x-c.offset.X())*factor, y - (y-c.offset.Y())*factor}
	c.zoom = zoom
}

// imageScale is the map image scale (relative to the outline image) worth
// rendering at the current zoom: a power of two, so that zooming doesn't
// rerender all the time, and at most maxMapImageSize pixels wide.
func (c *mapCamera) imageScale(pixelRatio float64) float64 {
	if !(c.zoom > 0) {
		return 1
	}
	wanted := c.zoom * pixelRatio
	limit := maxMapImageSize / math.Max(c.width, c.height)
	return math.Min(limit, math.Pow(2, math.Ceil(math.Log2(wanted))))
}

const maxMapImageSize = 4096
