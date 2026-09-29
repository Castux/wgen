package config

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Param describes an editable config value, for building a UI. Path is the
// location of the value in the JSON config.
type Param struct {
	Path    []string
	Label   string
	Group   string
	Type    string // number, int
	Min     float64
	Max     float64
	Step    float64
	Tooltip string
}

// Schema lists the editable parameters of the config, but the terrains.
func (c *Config) Schema() []Param {
	number := func(path []string, label, group string, low, high, step float64, tooltip string) Param {
		return Param{Path: path, Label: label, Group: group, Type: "number",
			Min: low, Max: high, Step: step, Tooltip: tooltip}
	}
	integer := func(path []string, label, group string, low, high float64, tooltip string) Param {
		return Param{Path: path, Label: label, Group: group, Type: "int",
			Min: low, Max: high, Step: 1, Tooltip: tooltip}
	}
	top := func(key string) []string { return []string{key} }
	simulation := func(key string) []string { return []string{"simulation", key} }

	return []Param{
		number(top("mapWidth"), "Map width (km)", GroupMap, 1, 20000, 1,
			"Real width of the map: sets the scale of everything"),
		integer(top("seed"), "Seed", GroupMap, 0, 1e9,
			"Randomness of the mesh and of the rock hardness: another seed, another landscape of the same map"),

		number(top("resolution"), "Resolution", GroupQuality, 0.5, 64, 0.5,
			"Mesh spacing where the detail is finest: finer is slower"),
		integer(top("levels"), "Refinement levels", GroupQuality, 0, 6,
			"How many times the mesh is refined, where terrains want the detail: each is twice as fine"),
		integer(simulation("steps"), "Time steps", GroupQuality, 1, 5000,
			"Of the simulation on the coarse mesh, for the landscape to settle: fewer is faster, rougher"),
		integer(simulation("refineSteps"), "Time steps per refinement", GroupQuality, 0, 5000,
			"Of the simulation on each finer mesh"),

		number(simulation("criticalSlope"), "Steepest slopes (°)", GroupLandscape, 5, 60, 1,
			"Hillslopes steeper than this collapse: high for sharp mountains, low for gentle ones. Terrains can set their own"),
		number(simulation("rounding"), "Rounding (0-1)", GroupLandscape, 0, 1, 0.05,
			"How rounded the tops are: soil creeping downhill smooths them, as on old mountains. 0: crisp. Terrains can set their own"),
		number(simulation("erodibility"), "River erosion", GroupLandscape, 1e-8, 1e-4, 1e-7,
			"How deep rivers cut: more for deep valleys and gorges, less for broad smooth land. Terrains can multiply it"),
		number(simulation("floorSlope"), "Sea floor slope", GroupLandscape, 0, 1, 0.001,
			"How fast the sea and lake floors deepen away from the shore"),

		number(simulation("upliftBlur"), "Uplift ramp (km)", GroupAdvanced, 0, 500, 1,
			"How gradually ranges rise from the lower land around them"),
		number(simulation("timeStep"), "Time step (kyr)", GroupAdvanced, 1, 1000, 1,
			"Of the simulation: longer is faster, coarser"),
		number(simulation("streamExponent"), "River size exponent", GroupAdvanced, 0.2, 0.8, 0.01,
			"How much faster big rivers cut than small ones: higher gives more concave valleys (m in the stream power law)"),
		number(simulation("erodibilityNoise"), "Rock hardness variation", GroupAdvanced, 0, 1, 0.01,
			"How much the rock's hardness varies from place to place: irregular valleys and summits"),
		number(simulation("noiseScale"), "Variation scale (km)", GroupAdvanced, 1, 1000, 1,
			"Size of the patches of harder and softer rock"),
	}
}

// Groups of the parameters
const (
	GroupMap       = "Map"
	GroupQuality   = "Quality"
	GroupLandscape = "Landscape"
	GroupAdvanced  = "Advanced"
)

// Quality is a choice of the parameters of the Quality group.
type Quality struct {
	Name               string
	Resolution         float64 // map pixels
	Levels             int
	Steps, RefineSteps int
}

// Qualities, from the fastest
var Qualities = []Quality{
	{Name: "Draft", Resolution: 4, Levels: 2, Steps: 150, RefineSteps: 30},
	{Name: "Normal", Resolution: 2, Levels: 3, Steps: 300, RefineSteps: 60},
	{Name: "Fine", Resolution: 1, Levels: 4, Steps: 400, RefineSteps: 80},
}

// QualityOf is the quality a config has, "" if none of them.
func (c *Config) QualityOf() string {
	for _, q := range Qualities {
		if c.Resolution == q.Resolution && c.Levels == q.Levels && c.Simulation.Steps == q.Steps && c.Simulation.RefineSteps == q.RefineSteps {
			return q.Name
		}
	}
	return ""
}

// SetQuality sets the parameters of a quality.
func (c *Config) SetQuality(q Quality) {
	c.Resolution, c.Levels = q.Resolution, q.Levels
	c.Simulation.Steps, c.Simulation.RefineSteps = q.Steps, q.RefineSteps
}

// Value returns the current value of a parameter, given its path (in the
// JSON format): float64 for numbers, or string.
func (c *Config) Value(path []string) (any, error) {
	var value any
	if err := json.Unmarshal(c.Marshal(), &value); err != nil {
		return nil, err
	}
	for _, key := range path {
		object, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("unknown parameter %s", strings.Join(path, "."))
		}
		if value, ok = object[key]; !ok {
			return nil, fmt.Errorf("unknown parameter %s", strings.Join(path, "."))
		}
	}
	return value, nil
}
