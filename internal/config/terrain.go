package config

import "slices"

// Kind of terrain
type Kind int

const (
	Land Kind = iota
	Sea       // the base level, at sea level
	Lake      // water, flat, draining through its outlet
)

// Names of the water terrains, which every project has
const (
	SeaName  = "sea"
	LakeName = "lake"
)

type Terrain struct {
	Name  string
	Color Color
	Kind  Kind

	// Target summit height, meters (land): the simulation finds the uplift
	// that makes the high points of each region reach it. 0: no uplift.
	Height float64

	// Erodibility multiplier, of the project's (default 1).
	Erodibility float64

	// Steepest slopes, degrees, beyond which hillslopes collapse (Inherit:
	// the project's).
	CriticalSlope float64

	// Number of mesh refinement levels (DetailAuto: all levels on land, one
	// on water).
	Detail int
}

// DetailAuto is the default terrain detail.
const DetailAuto = -1

// Inherit is the value of a terrain setting that takes the project's.
const Inherit = -1

// Slope is the terrain's steepest slope, degrees: its own, or the
// project's.
func (t *Terrain) Slope(s Simulation) float64 {
	if t.CriticalSlope == Inherit {
		return s.CriticalSlope
	}
	return t.CriticalSlope
}

// NewTerrain is a terrain of that name, with the default settings: its kind
// from its name, the project's slopes and erosion, all the detail.
func NewTerrain(name string, color Color) *Terrain {
	return &Terrain{Name: name, Color: color, Kind: kindOf(name), Erodibility: 1, CriticalSlope: Inherit, Detail: DetailAuto}
}

func (t *Terrain) IsWater() bool { return t.Kind != Land }

// Calibrated tells whether the uplift of a terrain is found from its target
// height.
func (t *Terrain) Calibrated() bool { return t.Kind == Land && t.Height > 0 }

// Default terrains of new projects
var (
	DefaultSea  = Terrain{Name: SeaName, Color: Color{66, 66, 125}, Kind: Sea, Erodibility: 1, CriticalSlope: Inherit, Detail: DetailAuto}
	DefaultLake = Terrain{Name: LakeName, Color: Color{109, 148, 194}, Kind: Lake, Erodibility: 1, CriticalSlope: Inherit, Detail: DetailAuto}
	DefaultLand = []Terrain{
		{Name: "plains", Color: Color{135, 168, 81}, Height: 400, Erodibility: 1, CriticalSlope: Inherit, Detail: 1},
		{Name: "hills", Color: Color{209, 184, 134}, Height: 1500, Erodibility: 1, CriticalSlope: Inherit, Detail: 2},
		{Name: "mountains", Color: Color{101, 72, 31}, Height: 4500, Erodibility: 1, CriticalSlope: Inherit, Detail: DetailAuto},
	}
)

// kindOf is the kind of a terrain, which comes from its name.
func kindOf(name string) Kind {
	switch name {
	case SeaName:
		return Sea
	case LakeName:
		return Lake
	}
	return Land
}

// kindOrder is the order of the terrains in a config: water first.
var kindOrder = map[Kind]int{Sea: 0, Lake: 1, Land: 2}

// addWater adds the sea and lake terrains if missing, with their default
// colors (or free ones), and puts them first.
func (c *Config) addWater() {
	for _, water := range []Terrain{DefaultLake, DefaultSea} {
		if c.Terrain(water.Name) != nil {
			continue
		}
		t := water
		t.Color = c.FreeColor(t.Color)
		c.Terrains = append([]*Terrain{&t}, c.Terrains...)
	}
	slices.SortStableFunc(c.Terrains, func(a, b *Terrain) int {
		return kindOrder[a.Kind] - kindOrder[b.Kind]
	})
}

// FreeColor returns a color no terrain has: the given one, or one close to
// it.
func (c *Config) FreeColor(want Color) Color {
	used := c.TerrainsByColor()
	for i := 0; ; i++ {
		candidate := want
		candidate[i%3] = want[i%3] + uint8(i*37)
		if _, taken := used[candidate]; !taken || i > 1000 {
			return candidate
		}
	}
}

func (c *Config) Terrain(name string) *Terrain {
	for _, t := range c.Terrains {
		if t.Name == name {
			return t
		}
	}
	return nil
}

// Land returns the land terrains, in order.
func (c *Config) Land() []*Terrain {
	var land []*Terrain
	for _, t := range c.Terrains {
		if t.Kind == Land {
			land = append(land, t)
		}
	}
	return land
}

// TerrainsByColor builds the lookup table used to map image pixels to terrains.
func (c *Config) TerrainsByColor() map[Color]*Terrain {
	byColor := make(map[Color]*Terrain, len(c.Terrains))
	for _, t := range c.Terrains {
		byColor[t.Color] = t
	}
	return byColor
}

// TerrainsEqual compares terrains regardless of their order.
func TerrainsEqual(a, b *Config) bool {
	return terrainsMatch(a, b, func(x, y *Terrain) bool { return *x == *y })
}

// TerrainMeshesEqual tells whether terrains give the same mesh: same
// colors, kinds and details.
func TerrainMeshesEqual(a, b *Config) bool {
	return terrainsMatch(a, b, func(x, y *Terrain) bool { return x.Kind == y.Kind && x.Detail == y.Detail })
}

// terrainsMatch tells whether both configs have terrains of the same colors,
// and whether the terrains of each color are the same.
func terrainsMatch(a, b *Config, same func(x, y *Terrain) bool) bool {
	if len(a.Terrains) != len(b.Terrains) {
		return false
	}
	bByColor := b.TerrainsByColor()
	for _, ta := range a.Terrains {
		tb, ok := bByColor[ta.Color]
		if !ok || !same(ta, tb) {
			return false
		}
	}
	return true
}
