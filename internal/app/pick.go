package app

import (
	"math"

	"github.com/go-gl/mathgl/mgl64"

	"github.com/Castux/wgen/internal/geom"
)

// Between the screen and the map: which map position is under a window
// position, and where a map position shows. Map positions are in map
// pixels, y up (as the world's); the 3D view centers the map on the origin,
// elevations scaled by zScale.

// zScale converts elevations (meters) to 3D units (map pixels), with the
// vertical exaggeration.
func (a *app) zScale() float64 {
	return a.settings.VerticalScale / a.terrain.metersPerPixel
}

// mapPosition is the map position under a window position, if any: on the
// map image, or on the terrain in 3D.
func (a *app) mapPosition(x, y float64) (geom.Vec2, bool) {
	if a.world == nil || a.inspect.probe == nil {
		return geom.Vec2{}, false
	}
	width, height, _ := a.viewSize()

	var position geom.Vec2
	switch a.settings.View {
	case viewMap:
		camera := &a.mapView.camera
		if camera.zoom <= 0 {
			return geom.Vec2{}, false
		}
		position = geom.Vec2{X: (x - camera.offset.X()) / camera.zoom, Y: camera.height - (y-camera.offset.Y())/camera.zoom}

	default:
		near, far, ok := a.orbitRay(x, y, width, height)
		if !ok {
			return geom.Vec2{}, false
		}
		v := a.terrain
		zScale := a.zScale()
		surface := func(x, y float64) float64 {
			return a.inspect.probe.Ground(geom.Vec2{X: x + v.width/2, Y: y + v.height/2}) * zScale
		}
		hit, ok := marchRay(near, far, v.lowest*zScale, math.Max(v.highest, 0)*zScale, surface)
		if !ok {
			return geom.Vec2{}, false
		}
		position = geom.Vec2{X: hit.X() + v.width/2, Y: hit.Y() + v.height/2}
	}
	return position, a.inspect.probe.Inside(position)
}

// orbitRay is the ray under a window position in the 3D view, from the
// near plane to the far one.
func (a *app) orbitRay(x, y, width, height float64) (near, far mgl64.Vec3, ok bool) {
	camera := &a.terrain.orbit
	inverse := camera.projection(width / height).Mul4(camera.view()).Inv()
	ndcX, ndcY := 2*x/width-1, 1-2*y/height
	unproject := func(z float64) (mgl64.Vec3, bool) {
		p := inverse.Mul4x1(mgl64.Vec4{ndcX, ndcY, z, 1})
		if math.Abs(p.W()) < 1e-12 {
			return mgl64.Vec3{}, false
		}
		return p.Vec3().Mul(1 / p.W()), true
	}
	near, okNear := unproject(-1)
	far, okFar := unproject(1)
	return near, far, okNear && okFar
}

// marchRay finds where the segment from near to far first goes below a
// surface, whose heights (z) are within lowest..highest: in steps of half a
// unit, then by bisection. surface returns NaN outside of it.
func marchRay(near, far mgl64.Vec3, lowest, highest float64, surface func(x, y float64) float64) (mgl64.Vec3, bool) {
	direction := far.Sub(near)

	// The part of the segment within the heights of the surface
	start, end := 0.0, 1.0
	if dz := direction.Z(); math.Abs(dz) > 1e-12 {
		t0, t1 := (highest-near.Z())/dz, (lowest-near.Z())/dz
		start, end = math.Max(start, math.Min(t0, t1)), math.Min(end, math.Max(t0, t1))
	} else if near.Z() < lowest || near.Z() > highest {
		return mgl64.Vec3{}, false
	}
	if start >= end {
		return mgl64.Vec3{}, false
	}

	at := func(t float64) mgl64.Vec3 { return near.Add(direction.Mul(t)) }
	below := func(t float64) bool {
		p := at(t)
		z := surface(p.X(), p.Y())
		return !math.IsNaN(z) && p.Z() <= z
	}

	horizontal := math.Hypot(direction.X(), direction.Y()) * (end - start)
	steps := int(math.Min(20000, math.Max(2, horizontal/0.5)))
	previous := start
	for i := 0; i <= steps; i++ {
		t := start + (end-start)*float64(i)/float64(steps)
		if below(t) {
			if i == 0 {
				return at(t), true
			}
			low, high := previous, t
			for range 24 {
				middle := (low + high) / 2
				if below(middle) {
					high = middle
				} else {
					low = middle
				}
			}
			return at(high), true
		}
		previous = t
	}
	return mgl64.Vec3{}, false
}

// screenPosition is where a map position, at an elevation (meters), shows
// in the window: false if behind the camera.
func (a *app) screenPosition(position geom.Vec2, elevation float64) (x, y float64, ok bool) {
	width, height, _ := a.viewSize()
	switch a.settings.View {
	case viewMap:
		camera := &a.mapView.camera
		return camera.offset.X() + position.X*camera.zoom, camera.offset.Y() + (camera.height-position.Y)*camera.zoom, true

	default:
		v := a.terrain
		camera := &v.orbit
		if math.IsNaN(elevation) {
			elevation = 0
		}
		clip := camera.projection(width / height).Mul4(camera.view()).Mul4x1(
			mgl64.Vec4{position.X - v.width/2, position.Y - v.height/2, elevation * a.zScale(), 1})
		if clip.W() <= 0 {
			return 0, 0, false
		}
		ndcX, ndcY := clip.X()/clip.W(), clip.Y()/clip.W()
		return (ndcX + 1) / 2 * width, (1 - ndcY) / 2 * height, true
	}
}
