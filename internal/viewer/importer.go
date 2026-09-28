package viewer

import (
	"cmp"
	"slices"

	"github.com/Castux/wgen/internal/config"
)

// Importing an image as a map: its colors are picked as terrains, and every
// pixel takes the terrain of the closest picked color, which also cleans up
// antialiased edges and other stray colors.

// pick maps an image color to a terrain.
type pick struct {
	color   config.Color
	terrain string
}

type colorCount struct {
	color config.Color
	count int
}

// palette returns the colors of an image, the most common first.
func palette(pixels []config.Color) []colorCount {
	counts := map[config.Color]int{}
	for _, p := range pixels {
		counts[p]++
	}
	colors := make([]colorCount, 0, len(counts))
	for c, n := range counts {
		colors = append(colors, colorCount{c, n})
	}
	slices.SortFunc(colors, func(a, b colorCount) int {
		if a.count != b.count {
			return b.count - a.count
		}
		return cmp.Compare(a.color.String(), b.color.String())
	})
	return colors
}

// borderColor is the most common color on the border of an image: most
// likely the sea.
func borderColor(width, height int, pixels []config.Color) config.Color {
	counts := map[config.Color]int{}
	for x := range width {
		counts[pixels[x]]++
		counts[pixels[(height-1)*width+x]]++
	}
	for y := range height {
		counts[pixels[y*width]]++
		counts[pixels[y*width+width-1]]++
	}
	var best config.Color
	bestCount := -1
	for c, n := range counts {
		if n > bestCount || n == bestCount && c.String() < best.String() {
			best, bestCount = c, n
		}
	}
	return best
}

// defaultPicks guesses the terrains of an image: colors of the config's
// terrains are theirs; then the border color is the sea, and the most common
// other color the first land terrain.
func defaultPicks(width, height int, pixels []config.Color, conf *config.Config) []pick {
	var picks []pick
	colors := palette(pixels)
	terrains := conf.TerrainsByColor()
	picked := func(name string) bool {
		return slices.ContainsFunc(picks, func(p pick) bool { return p.terrain == name })
	}

	// Exact matches, if they are more than stray pixels
	for _, c := range colors {
		if t := terrains[c.color]; t != nil && c.count*1000 >= len(pixels) {
			picks = append(picks, pick{c.color, t.Name})
		}
	}

	pickedColor := func(c config.Color) bool {
		return slices.ContainsFunc(picks, func(p pick) bool { return p.color == c })
	}
	if !picked(config.SeaName) {
		if c := borderColor(width, height, pixels); !pickedColor(c) {
			picks = append(picks, pick{c, config.SeaName})
		}
	}
	if land := conf.Land(); len(land) > 0 && !slices.ContainsFunc(picks, func(p pick) bool { return conf.Terrain(p.terrain).Kind == config.Land }) {
		for _, c := range colors {
			if !pickedColor(c.color) {
				picks = append(picks, pick{c.color, land[0].Name})
				break
			}
		}
	}
	return picks
}

// classify gives every pixel the color of the terrain of the closest picked
// color.
func classify(pixels []config.Color, picks []pick, conf *config.Config) []config.Color {
	out := make([]config.Color, len(pixels))
	if len(picks) == 0 {
		copy(out, pixels)
		return out
	}

	targets := make([]config.Color, len(picks))
	for i, p := range picks {
		if t := conf.Terrain(p.terrain); t != nil {
			targets[i] = t.Color
		}
	}

	// Most images have few colors: classify each once
	cache := map[config.Color]config.Color{}
	for i, p := range pixels {
		c, ok := cache[p]
		if !ok {
			best, bestDist := 0, -1
			for j, pk := range picks {
				if d := colorDistance(p, pk.color); bestDist < 0 || d < bestDist {
					best, bestDist = j, d
				}
			}
			c = targets[best]
			if len(cache) < 1<<16 {
				cache[p] = c
			}
		}
		out[i] = c
	}
	return out
}

func colorDistance(a, b config.Color) int {
	d := 0
	for k := range 3 {
		x := int(a[k]) - int(b[k])
		d += x * x
	}
	return d
}
