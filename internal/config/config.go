// Package config loads, validates, patches and saves wgen projects: a map
// image, whose pixel colors are terrains, and the parameters turning it into
// a landscape.
//
// A project file looks like:
//
//	{
//		"image": "map.png",          // relative to the project file
//		"mapWidth": 1000,            // km
//		"resolution": 2,             // finest mesh spacing, pixels
//		"levels": 3,                 // mesh refinements
//		"seed": 0,
//		"terrains": {
//			"sea": { "color": "#42427d" },
//			"lake": { "color": "#6d94c2" },
//			"plains": { "color": "#87a851", "height": 400, "detail": 1 },
//			"mountains": { "color": "#65481f", "height": 4500 }
//		},
//		"simulation": { "erodibility": 2e-6 }
//	}
//
// Terrains "sea" and "lake" are the water, and always exist; the others are
// land, with a target summit height. The simulation parameters are optional.
package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

type Color [3]uint8

func (c Color) String() string { return fmt.Sprintf("#%02x%02x%02x", c[0], c[1], c[2]) }

// ParseColor reads a "#rrggbb" color.
func ParseColor(s string) (Color, error) {
	var c Color
	if len(s) != 7 || s[0] != '#' {
		return c, fmt.Errorf("bad color %q (expected #rrggbb)", s)
	}
	for i := range 3 {
		v, err := strconv.ParseUint(s[1+2*i:3+2*i], 16, 8)
		if err != nil {
			return c, fmt.Errorf("bad color %q (expected #rrggbb)", s)
		}
		c[i] = uint8(v)
	}
	return c, nil
}

func (c Color) MarshalJSON() ([]byte, error) { return json.Marshal(c.String()) }

func (c *Color) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	parsed, err := ParseColor(s)
	*c = parsed
	return err
}

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

	// Erodibility multiplier (default 1).
	Erodibility float64

	// Number of mesh refinement levels (DetailAuto: all levels on land, one
	// on water).
	Detail int
}

// DetailAuto is the default terrain detail.
const DetailAuto = -1

func (t *Terrain) IsWater() bool { return t.Kind != Land }

// Calibrated tells whether the uplift of a terrain is found from its target
// height.
func (t *Terrain) Calibrated() bool { return t.Kind == Land && t.Height > 0 }

type Config struct {
	// Path of the project file, not serialized. Empty for a project not saved
	// yet.
	ConfigPath string `json:"-"`

	Image      string  `json:"image"`      // map image, relative to the project file
	MapWidth   float64 `json:"mapWidth"`   // km
	Resolution float64 `json:"resolution"` // mesh spacing of the finest level, pixels
	Levels     int     `json:"levels"`     // refinements of the coarse mesh
	Seed       uint64  `json:"seed"`

	// Water first (sea, lake), then land, in file order
	Terrains []*Terrain `json:"-"`

	Simulation Simulation `json:"simulation"`
}

// Simulation are the parameters of the landscape simulation: the land rises
// (at the rate found from the target heights), rivers erode it (stream power
// law), and hillslopes collapse beyond a critical slope.
type Simulation struct {
	UpliftBlur       float64 `json:"upliftBlur"`       // km: uplift ramps up over this distance from lower terrains
	Erodibility      float64 `json:"erodibility"`      // K, per year (for m = 0.5)
	StreamExponent   float64 `json:"streamExponent"`   // m, exponent of the drainage area
	CriticalSlope    float64 `json:"criticalSlope"`    // degrees
	TimeStep         float64 `json:"timeStep"`         // thousands of years
	Steps            int     `json:"steps"`            // on the coarse mesh
	RefineSteps      int     `json:"refineSteps"`      // on each finer mesh
	ErodibilityNoise float64 `json:"erodibilityNoise"` // 0..1: variation of the rock hardness
	NoiseScale       float64 `json:"noiseScale"`       // km, of that variation
	FloorSlope       float64 `json:"floorSlope"`       // of the sea and lake floors, meters per meter
}

// DefaultSimulation are the default simulation parameters.
var DefaultSimulation = Simulation{
	UpliftBlur: 30, Erodibility: 2e-6, StreamExponent: 0.5, CriticalSlope: 30,
	TimeStep: 50, Steps: 300, RefineSteps: 60,
	ErodibilityNoise: 0.3, NoiseScale: 30, FloorSlope: 0.01,
}

// Default terrains of new projects
var (
	DefaultSea  = Terrain{Name: SeaName, Color: Color{66, 66, 125}, Kind: Sea, Erodibility: 1, Detail: DetailAuto}
	DefaultLake = Terrain{Name: LakeName, Color: Color{109, 148, 194}, Kind: Lake, Erodibility: 1, Detail: DetailAuto}
	DefaultLand = []Terrain{
		{Name: "plains", Color: Color{135, 168, 81}, Height: 400, Erodibility: 1, Detail: 1},
		{Name: "hills", Color: Color{209, 184, 134}, Height: 1500, Erodibility: 1, Detail: 2},
		{Name: "mountains", Color: Color{101, 72, 31}, Height: 4500, Erodibility: 1, Detail: DetailAuto},
	}
)

// Default returns the config of a new project, for a map image.
func Default(image string) *Config {
	c := &Config{Image: image, MapWidth: 1000, Resolution: 2, Levels: 3, Simulation: DefaultSimulation}
	for _, t := range append([]Terrain{DefaultSea, DefaultLake}, DefaultLand...) {
		c.Terrains = append(c.Terrains, &t)
	}
	return c
}

// ImagePath is the path of the map image: relative to the project file.
func (c *Config) ImagePath() string {
	if c.ConfigPath == "" || filepath.IsAbs(c.Image) {
		return c.Image
	}
	return filepath.Join(filepath.Dir(c.ConfigPath), c.Image)
}

var topKeys = []string{"image", "mapWidth", "resolution", "levels", "seed", "terrains", "simulation"}

var terrainKeys = []string{"color", "height", "erodibility", "detail"}

// Load reads and validates a project file. Warnings are non fatal problems,
// such as unknown keys.
func Load(path string) (conf *Config, warnings []string, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	conf, warnings, err = Parse(data)
	if conf != nil {
		conf.ConfigPath = path
	}
	return conf, warnings, err
}

// Parse reads and validates a project from its JSON.
func Parse(data []byte) (*Config, []string, error) {
	conf := &Config{MapWidth: 1000, Resolution: 2, Levels: 3, Simulation: DefaultSimulation}
	warnings, err := conf.apply(data, true)
	if err != nil {
		return nil, warnings, err
	}
	conf.addWater()
	return conf, warnings, conf.Validate()
}

// Patch returns a copy of the config with the given partial JSON applied on
// top. Terrains are patched by name, and cannot be added or removed.
func (c *Config) Patch(data []byte) (*Config, error) {
	n := c.Clone()
	if _, err := n.apply(data, false); err != nil {
		return nil, err
	}
	return n, n.Validate()
}

func (c *Config) apply(data []byte, full bool) (warnings []string, err error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	if full {
		for _, key := range []string{"image", "terrains"} {
			if _, ok := raw[key]; !ok {
				return nil, fmt.Errorf("missing required key %q", key)
			}
		}
	}

	for key, value := range raw {
		switch {
		case !slices.Contains(topKeys, key):
			warnings = append(warnings, fmt.Sprintf("unknown key %q ignored", key))
			continue
		case key == "terrains":
			err = c.applyTerrains(value, full)
		case key == "simulation":
			err = decodeStrict(value, &c.Simulation)
		default:
			err = setField(c, key, value)
		}
		if err != nil {
			return warnings, fmt.Errorf("%s: %w", key, err)
		}
	}

	slices.Sort(warnings)
	return warnings, nil
}

// setField decodes a single top level value into the matching struct field,
// by unmarshalling a one key object into the struct (which leaves the other
// fields untouched).
func setField(c *Config, key string, value json.RawMessage) error {
	var buf bytes.Buffer
	buf.WriteString("{")
	buf.WriteString(strconv.Quote(key))
	buf.WriteString(":")
	buf.Write(value)
	buf.WriteString("}")
	return decodeStrict(buf.Bytes(), c)
}

type rawTerrain struct {
	Color       *Color   `json:"color"`
	Height      *float64 `json:"height"`
	Erodibility *float64 `json:"erodibility"`
	Detail      *int     `json:"detail"`
}

func (c *Config) applyTerrains(data json.RawMessage, full bool) error {
	if full {
		c.Terrains = nil
	}

	return forEachOrdered(data, func(name string, value json.RawMessage) error {
		var rt rawTerrain
		if err := decodeStrict(value, &rt); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}

		t := c.Terrain(name)
		switch {
		case t == nil && !full:
			return fmt.Errorf("unknown terrain %q", name)
		case t == nil:
			if rt.Color == nil {
				return fmt.Errorf("%s: color is required", name)
			}
			t = &Terrain{Name: name, Kind: kindOf(name), Erodibility: 1, Detail: DetailAuto}
			c.Terrains = append(c.Terrains, t)
		}

		if rt.Color != nil {
			t.Color = *rt.Color
		}
		if rt.Height != nil {
			t.Height = *rt.Height
		}
		if rt.Erodibility != nil {
			t.Erodibility = *rt.Erodibility
		}
		if rt.Detail != nil {
			t.Detail = *rt.Detail
		}
		return nil
	})
}

func kindOf(name string) Kind {
	switch name {
	case SeaName:
		return Sea
	case LakeName:
		return Lake
	}
	return Land
}

// addWater adds the sea and lake terrains if missing, with their default
// colors (or free ones), and puts them first.
func (c *Config) addWater() {
	for _, def := range []Terrain{DefaultLake, DefaultSea} {
		if c.Terrain(def.Name) != nil {
			continue
		}
		t := def
		t.Color = c.FreeColor(t.Color)
		c.Terrains = append([]*Terrain{&t}, c.Terrains...)
	}
	slices.SortStableFunc(c.Terrains, func(a, b *Terrain) int {
		order := func(t *Terrain) int { return map[Kind]int{Sea: 0, Lake: 1, Land: 2}[t.Kind] }
		return order(a) - order(b)
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

func decodeStrict(data []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

// forEachOrdered iterates over the members of a JSON object in file order.
func forEachOrdered(data []byte, f func(key string, value json.RawMessage) error) error {
	dec := json.NewDecoder(bytes.NewReader(data))

	tok, err := dec.Token()
	if err != nil {
		return err
	}
	if tok != json.Delim('{') {
		return fmt.Errorf("expected an object")
	}

	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return err
		}

		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return err
		}

		if err := f(tok.(string), value); err != nil {
			return err
		}
	}
	return nil
}

func (c *Config) Validate() error {
	var errs []string
	add := func(format string, args ...any) { errs = append(errs, fmt.Sprintf(format, args...)) }

	if c.MapWidth <= 0 {
		add("mapWidth must be positive")
	}
	if c.Resolution <= 0 {
		add("resolution must be positive")
	}
	if c.Levels < 0 || c.Levels > 6 {
		add("levels must be between 0 and 6")
	}

	s := c.Simulation
	if s.UpliftBlur < 0 || s.NoiseScale <= 0 {
		add("upliftBlur and noiseScale must be positive")
	}
	if s.Erodibility <= 0 || s.StreamExponent <= 0 {
		add("erodibility and streamExponent must be positive")
	}
	if s.CriticalSlope <= 0 || s.CriticalSlope >= 90 {
		add("criticalSlope must be between 0 and 90 degrees")
	}
	if s.TimeStep <= 0 || s.Steps < 1 || s.RefineSteps < 0 {
		add("timeStep and steps must be positive")
	}
	if s.ErodibilityNoise < 0 || s.ErodibilityNoise > 1 {
		add("erodibilityNoise must be between 0 and 1")
	}
	if s.FloorSlope < 0 {
		add("floorSlope must be positive")
	}

	if c.Terrain(SeaName) == nil || c.Terrain(LakeName) == nil {
		add("the sea and lake terrains are required")
	}
	colors := map[Color]string{}
	for _, t := range c.Terrains {
		if strings.TrimSpace(t.Name) == "" {
			add("a terrain has no name")
		}
		if other, ok := colors[t.Color]; ok {
			add("terrains %s and %s have the same color", other, t.Name)
		}
		colors[t.Color] = t.Name
		if t.Height < 0 || t.Erodibility < 0 {
			add("terrain %s: height and erodibility must be positive", t.Name)
		}
		if t.Detail < DetailAuto {
			add("terrain %s: detail must be positive (or -1, automatic)", t.Name)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("invalid project: %s", strings.Join(errs, ", "))
	}
	return nil
}

func (c *Config) Clone() *Config {
	n := *c
	n.Terrains = make([]*Terrain, len(c.Terrains))
	for i, t := range c.Terrains {
		tc := *t
		n.Terrains[i] = &tc
	}
	return &n
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
	m := make(map[Color]*Terrain, len(c.Terrains))
	for _, t := range c.Terrains {
		m[t.Color] = t
	}
	return m
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

func terrainsMatch(a, b *Config, same func(x, y *Terrain) bool) bool {
	if len(a.Terrains) != len(b.Terrains) {
		return false
	}
	bm := b.TerrainsByColor()
	for _, ta := range a.Terrains {
		tb, ok := bm[ta.Color]
		if !ok || !same(ta, tb) {
			return false
		}
	}
	return true
}

// Marshal serializes the config in a stable, human friendly layout (one
// terrain per line, in their order).
func (c *Config) Marshal() []byte {
	var b bytes.Buffer
	num := func(f float64) string { return strconv.FormatFloat(f, 'g', -1, 64) }

	b.WriteString("{\n")
	fmt.Fprintf(&b, "\t\"image\": %s,\n", strconv.Quote(filepath.ToSlash(c.Image)))
	fmt.Fprintf(&b, "\t\"mapWidth\": %s,\n", num(c.MapWidth))
	fmt.Fprintf(&b, "\t\"resolution\": %s,\n", num(c.Resolution))
	fmt.Fprintf(&b, "\t\"levels\": %d,\n", c.Levels)
	fmt.Fprintf(&b, "\t\"seed\": %d,\n", c.Seed)

	b.WriteString("\n\t\"terrains\": {\n")
	for i, t := range c.Terrains {
		fmt.Fprintf(&b, "\t\t%s: { \"color\": %q", strconv.Quote(t.Name), t.Color.String())
		if t.Kind == Land {
			fmt.Fprintf(&b, ", \"height\": %s", num(t.Height))
		}
		if t.Erodibility != 1 {
			fmt.Fprintf(&b, ", \"erodibility\": %s", num(t.Erodibility))
		}
		if t.Detail != DetailAuto {
			fmt.Fprintf(&b, ", \"detail\": %d", t.Detail)
		}
		b.WriteString(" }")
		if i < len(c.Terrains)-1 {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}
	b.WriteString("\t},\n\n")

	s := c.Simulation
	b.WriteString("\t\"simulation\": {\n")
	fields := []string{
		fmt.Sprintf("\"upliftBlur\": %s", num(s.UpliftBlur)),
		fmt.Sprintf("\"erodibility\": %s", num(s.Erodibility)),
		fmt.Sprintf("\"streamExponent\": %s", num(s.StreamExponent)),
		fmt.Sprintf("\"criticalSlope\": %s", num(s.CriticalSlope)),
		fmt.Sprintf("\"timeStep\": %s", num(s.TimeStep)),
		fmt.Sprintf("\"steps\": %d", s.Steps),
		fmt.Sprintf("\"refineSteps\": %d", s.RefineSteps),
		fmt.Sprintf("\"erodibilityNoise\": %s", num(s.ErodibilityNoise)),
		fmt.Sprintf("\"noiseScale\": %s", num(s.NoiseScale)),
		fmt.Sprintf("\"floorSlope\": %s", num(s.FloorSlope)),
	}
	b.WriteString("\t\t" + strings.Join(fields, ",\n\t\t") + "\n")
	b.WriteString("\t}\n}\n")

	return b.Bytes()
}

// Save writes the config to its file.
func (c *Config) Save() error {
	if c.ConfigPath == "" {
		return fmt.Errorf("the project has no file yet")
	}
	return os.WriteFile(c.ConfigPath, c.Marshal(), 0o644)
}

// MetersPerPixel is the horizontal scale of a map of the given width in
// pixels.
func (c *Config) MetersPerPixel(width int) float64 {
	return c.MapWidth * 1000 / math.Max(1, float64(width))
}
