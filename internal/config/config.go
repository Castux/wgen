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
	"fmt"
	"math"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	// Path of the project file, not serialized. Empty for a project not saved
	// yet.
	Path string `json:"-"`

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
	Rounding         float64 `json:"rounding"`         // 0..1: hillslope diffusion, rounding the tops
}

// DefaultSimulation are the default simulation parameters.
var DefaultSimulation = Simulation{
	UpliftBlur: 30, Erodibility: 2e-6, StreamExponent: 0.5, CriticalSlope: 30,
	TimeStep: 50, Steps: 300, RefineSteps: 60,
	ErodibilityNoise: 0.3, NoiseScale: 30, FloorSlope: 0.01, Rounding: 0,
}

// withDefaults returns a config with the default map settings and simulation
// parameters, and no terrains.
func withDefaults(image string) *Config {
	return &Config{Image: image, MapWidth: 1000, Resolution: 2, Levels: 3, Simulation: DefaultSimulation}
}

// Default returns the config of a new project, for a map image.
func Default(image string) *Config {
	c := withDefaults(image)
	for _, t := range append([]Terrain{DefaultSea, DefaultLake}, DefaultLand...) {
		c.Terrains = append(c.Terrains, &t)
	}
	return c
}

// ImagePath is the path of the map image: relative to the project file.
func (c *Config) ImagePath() string {
	if c.Path == "" || filepath.IsAbs(c.Image) {
		return c.Image
	}
	return filepath.Join(filepath.Dir(c.Path), c.Image)
}

// Validate checks the values, and reports all the invalid ones.
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
	if s.TimeStep <= 0 || s.Steps < 1 {
		add("timeStep and steps must be positive")
	}
	if s.RefineSteps < 0 {
		add("refineSteps must not be negative")
	}
	if s.ErodibilityNoise < 0 || s.ErodibilityNoise > 1 {
		add("erodibilityNoise must be between 0 and 1")
	}
	if s.FloorSlope < 0 {
		add("floorSlope must be positive")
	}
	if s.Rounding < 0 || s.Rounding > 1 {
		add("rounding must be between 0 and 1")
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
		if b := t.UpliftBlur; b != Inherit && b < 0 {
			add("terrain %s: upliftBlur must be positive (or -1, the project's)", t.Name)
		}
		if r := t.Rounding; r != Inherit && (r < 0 || r > 1) {
			add("terrain %s: rounding must be between 0 and 1 (or -1, the project's)", t.Name)
		}
		if slope := t.CriticalSlope; slope != Inherit && (slope <= 0 || slope >= 90) {
			add("terrain %s: criticalSlope must be between 0 and 90 degrees (or -1, the project's)", t.Name)
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

// Clone returns a deep copy of the config: its terrains can be changed.
func (c *Config) Clone() *Config {
	clone := *c
	clone.Terrains = make([]*Terrain, len(c.Terrains))
	for i, t := range c.Terrains {
		terrain := *t
		clone.Terrains[i] = &terrain
	}
	return &clone
}

// formatNumber writes a number as short as possible, and exactly.
func formatNumber(f float64) string { return strconv.FormatFloat(f, 'g', -1, 64) }

// Marshal serializes the config in a stable, human friendly layout (one
// terrain per line, in their order).
func (c *Config) Marshal() []byte {
	var buf bytes.Buffer

	buf.WriteString("{\n")
	fmt.Fprintf(&buf, "\t\"image\": %s,\n", strconv.Quote(filepath.ToSlash(c.Image)))
	fmt.Fprintf(&buf, "\t\"mapWidth\": %s,\n", formatNumber(c.MapWidth))
	fmt.Fprintf(&buf, "\t\"resolution\": %s,\n", formatNumber(c.Resolution))
	fmt.Fprintf(&buf, "\t\"levels\": %d,\n", c.Levels)
	fmt.Fprintf(&buf, "\t\"seed\": %d,\n", c.Seed)

	buf.WriteString("\n\t\"terrains\": {\n")
	for i, t := range c.Terrains {
		buf.WriteString("\t\t" + marshalTerrain(t))
		if i < len(c.Terrains)-1 {
			buf.WriteString(",")
		}
		buf.WriteString("\n")
	}
	buf.WriteString("\t},\n\n")

	s := c.Simulation
	buf.WriteString("\t\"simulation\": {\n")
	fields := []string{
		fmt.Sprintf("\"upliftBlur\": %s", formatNumber(s.UpliftBlur)),
		fmt.Sprintf("\"erodibility\": %s", formatNumber(s.Erodibility)),
		fmt.Sprintf("\"streamExponent\": %s", formatNumber(s.StreamExponent)),
		fmt.Sprintf("\"criticalSlope\": %s", formatNumber(s.CriticalSlope)),
		fmt.Sprintf("\"timeStep\": %s", formatNumber(s.TimeStep)),
		fmt.Sprintf("\"steps\": %d", s.Steps),
		fmt.Sprintf("\"refineSteps\": %d", s.RefineSteps),
		fmt.Sprintf("\"erodibilityNoise\": %s", formatNumber(s.ErodibilityNoise)),
		fmt.Sprintf("\"noiseScale\": %s", formatNumber(s.NoiseScale)),
		fmt.Sprintf("\"floorSlope\": %s", formatNumber(s.FloorSlope)),
		fmt.Sprintf("\"rounding\": %s", formatNumber(s.Rounding)),
	}
	buf.WriteString("\t\t" + strings.Join(fields, ",\n\t\t") + "\n")
	buf.WriteString("\t}\n}\n")

	return buf.Bytes()
}

// marshalTerrain is a terrain's member of the "terrains" object, on one line,
// leaving out default values.
func marshalTerrain(t *Terrain) string {
	var buf strings.Builder
	fmt.Fprintf(&buf, "%s: { \"color\": %q", strconv.Quote(t.Name), t.Color.String())
	if t.Kind == Land {
		fmt.Fprintf(&buf, ", \"height\": %s", formatNumber(t.Height))
	}
	if t.Erodibility != 1 {
		fmt.Fprintf(&buf, ", \"erodibility\": %s", formatNumber(t.Erodibility))
	}
	if t.CriticalSlope != Inherit {
		fmt.Fprintf(&buf, ", \"criticalSlope\": %s", formatNumber(t.CriticalSlope))
	}
	if t.Rounding != Inherit {
		fmt.Fprintf(&buf, ", \"rounding\": %s", formatNumber(t.Rounding))
	}
	if t.UpliftBlur != Inherit {
		fmt.Fprintf(&buf, ", \"upliftBlur\": %s", formatNumber(t.UpliftBlur))
	}
	if t.Detail != DetailAuto {
		fmt.Fprintf(&buf, ", \"detail\": %d", t.Detail)
	}
	buf.WriteString(" }")
	return buf.String()
}

// MetersPerPixel is the horizontal scale of a map of the given width in
// pixels.
func (c *Config) MetersPerPixel(width int) float64 {
	return c.MapWidth * 1000 / math.Max(1, float64(width))
}
