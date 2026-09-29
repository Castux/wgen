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

	// Rounding of the tops, 0..1: hillslope diffusion (Inherit: the
	// project's).
	Rounding float64

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

// Tops is the terrain's rounding, 0..1: its own, or the project's.
func (t *Terrain) Tops(s Simulation) float64 {
	if t.Rounding == Inherit {
		return s.Rounding
	}
	return t.Rounding
}

// NewTerrain is a terrain of that name, with the default settings: its kind
// from its name, the project's slopes and erosion, all the detail.
func NewTerrain(name string, color Color) *Terrain {
	return &Terrain{Name: name, Color: color, Kind: kindOf(name), Erodibility: 1, CriticalSlope: Inherit, Rounding: Inherit, Detail: DetailAuto}
}

func (t *Terrain) IsWater() bool { return t.Kind != Land }

// Calibrated tells whether the uplift of a terrain is found from its target
// height.
func (t *Terrain) Calibrated() bool { return t.Kind == Land && t.Height > 0 }

// Character is a kind of landform, a choice of a terrain's slopes,
// rounding and erosion.
type Character struct {
	Name          string
	Description   string
	CriticalSlope float64 // degrees
	Rounding      float64 // 0..1
	Erodibility   float64 // multiplier of the project's
}

// Characters, for the terrains
var Characters = []Character{
	{"Young mountains", "Sharp ridges and peaks, deep valleys (the Alps)", 40, 0, 1.5},
	{"Old mountains", "Rounded ridges, broad valleys (the Appalachians)", 30, 0.8, 1},
	{"Hills", "Rolling, rounded", 25, 0.5, 1},
	{"Plateau and mesas", "Hard rock: steep edges, little cut by rivers", 50, 0, 0.4},
	{"Badlands", "Soft rock, deeply cut by many small valleys", 45, 0, 3},
	{"Plains", "Gentle, softly undulating", 15, 0.5, 0.7},
}

// CharacterOf is the character a terrain has, "" if none of them.
func (t *Terrain) CharacterOf() string {
	for _, c := range Characters {
		if t.CriticalSlope == c.CriticalSlope && t.Rounding == c.Rounding && t.Erodibility == c.Erodibility {
			return c.Name
		}
	}
	return ""
}

// SetCharacter gives a terrain the settings of a character.
func (t *Terrain) SetCharacter(c Character) {
	t.CriticalSlope, t.Rounding, t.Erodibility = c.CriticalSlope, c.Rounding, c.Erodibility
}

// characterNamed is a character by its name.
func characterNamed(name string) Character {
	for _, c := range Characters {
		if c.Name == name {
			return c
		}
	}
	panic("no character " + name)
}

// landTerrain is a default land terrain, of a character.
func landTerrain(name string, color Color, height float64, detail int, character string) Terrain {
	t := *NewTerrain(name, color)
	t.Height, t.Detail = height, detail
	t.SetCharacter(characterNamed(character))
	return t
}

// Default terrains of new projects
var (
	DefaultSea  = Terrain{Name: SeaName, Color: Color{66, 66, 125}, Kind: Sea, Erodibility: 1, CriticalSlope: Inherit, Rounding: Inherit, Detail: DetailAuto}
	DefaultLake = Terrain{Name: LakeName, Color: Color{109, 148, 194}, Kind: Lake, Erodibility: 1, CriticalSlope: Inherit, Rounding: Inherit, Detail: DetailAuto}
	DefaultLand = []Terrain{
		landTerrain("plains", Color{135, 168, 81}, 400, 1, "Plains"),
		landTerrain("hills", Color{209, 184, 134}, 1500, 2, "Hills"),
		landTerrain("mountains", Color{101, 72, 31}, 4500, DetailAuto, "Young mountains"),
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
