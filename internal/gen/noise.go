package gen

import (
	"math"

	"github.com/Castux/wgen/internal/geom"
)

// Noise for slope variations: deterministic functions of the position and
// the seed.

// hash2 mixes a seed and integer coordinates into 64 random bits
// (splitmix64 finalizer).
func hash2(seed uint64, x, y int64) uint64 {
	h := seed ^ uint64(x)*0x9e3779b97f4a7c15 ^ uint64(y)*0xc2b2ae3d27d4eb4f
	h ^= h >> 30
	h *= 0xbf58476d1ce4e5b9
	h ^= h >> 27
	h *= 0x94d049bb133111eb
	h ^= h >> 31
	return h
}

// unit converts random bits to [0, 1).
func unit(h uint64) float64 { return float64(h>>11) / (1 << 53) }

// gradientNoise is 2D gradient (Perlin) noise, about -0.7..0.7, 0 at integer
// coordinates.
func gradientNoise(seed uint64, x, y float64) float64 {
	x0, y0 := math.Floor(x), math.Floor(y)
	fx, fy := x-x0, y-y0
	ix, iy := int64(x0), int64(y0)

	dot := func(cx, cy int64, dx, dy float64) float64 {
		a := unit(hash2(seed, cx, cy)) * 2 * math.Pi
		return math.Cos(a)*dx + math.Sin(a)*dy
	}
	fade := func(t float64) float64 { return t * t * t * (t*(t*6-15) + 10) }

	u, v := fade(fx), fade(fy)
	return geom.Lerp(
		geom.Lerp(dot(ix, iy, fx, fy), dot(ix+1, iy, fx-1, fy), u),
		geom.Lerp(dot(ix, iy+1, fx, fy-1), dot(ix+1, iy+1, fx-1, fy-1), u),
		v)
}

// cellNoise is the distance between the nearest and second nearest of
// random points (one per unit cell): 0 on the boundaries between their
// cells, up to about 0.7 inside.
func cellNoise(seed uint64, x, y float64) float64 {
	ix, iy := int64(math.Floor(x)), int64(math.Floor(y))
	d1, d2 := math.Inf(1), math.Inf(1)

	for cy := iy - 1; cy <= iy+1; cy++ {
		for cx := ix - 1; cx <= ix+1; cx++ {
			h := hash2(seed, cx, cy)
			px := float64(cx) + unit(h)
			py := float64(cy) + unit(hash2(h, 1, 0))
			d := math.Hypot(px-x, py-y)
			switch {
			case d < d1:
				d1, d2 = d, d1
			case d < d2:
				d2 = d
			}
		}
	}
	return d2 - d1
}

// fbm is fractal gradient noise: octaves of halving amplitude and doubling
// frequency, about -1..1.
func fbm(seed uint64, x, y float64, octaves int) float64 {
	sum, weight, amplitude := 0.0, 0.0, 1.0
	for i := range octaves {
		sum += amplitude * gradientNoise(hash2(seed, int64(i), 7), x, y)
		weight += amplitude
		amplitude /= 2
		x, y = x*2, y*2
	}
	return geom.Clamp(sum/weight*1.4, -1, 1)
}

// Noise is 2D gradient noise, about -0.7..0.7, for other packages (such as
// the map editor's brushes).
func Noise(seed uint64, x, y float64) float64 { return gradientNoise(seed, x, y) }
