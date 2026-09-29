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
		number(top("mapWidth"), "Map width (km)", "Map", 1, 20000, 1,
			"Real width of the map: sets the scale of everything"),
		number(top("resolution"), "Resolution (px)", "Map", 0.5, 64, 0.5,
			"Mesh spacing of the finest level, in image pixels"),
		integer(top("levels"), "Refinement levels", "Map", 0, 6,
			"The coarse mesh is 2^levels coarser than the finest; terrains choose how many levels refine them"),
		integer(top("seed"), "Seed", "Map", 0, 1e9,
			"Randomness of the mesh and of the rock hardness"),

		number(simulation("upliftBlur"), "Uplift ramp (km)", "Simulation", 0, 500, 1,
			"Distance over which the uplift of a region ramps up from its border with lower terrains"),
		number(simulation("erodibility"), "Erodibility", "Simulation", 1e-8, 1e-4, 1e-7,
			"How fast rivers erode (K, per year). Lower gives steeper relief"),
		number(simulation("streamExponent"), "Area exponent", "Simulation", 0.2, 0.8, 0.01,
			"How much faster big rivers erode (m in the stream power law)"),
		number(simulation("criticalSlope"), "Critical slope (deg)", "Simulation", 5, 60, 1,
			"Steeper hillslopes collapse"),
		number(simulation("timeStep"), "Time step (kyr)", "Simulation", 1, 1000, 1, ""),
		integer(simulation("steps"), "Steps (coarse)", "Simulation", 1, 5000,
			"Time steps on the coarse mesh, for the landscape to settle"),
		integer(simulation("refineSteps"), "Steps (each refinement)", "Simulation", 0, 5000, ""),
		number(simulation("erodibilityNoise"), "Rock hardness variation", "Simulation", 0, 1, 0.01, ""),
		number(simulation("noiseScale"), "Variation scale (km)", "Simulation", 1, 1000, 1, ""),
		number(simulation("floorSlope"), "Sea floor slope", "Simulation", 0, 1, 0.001,
			"Slope of the sea and lake floors, meters per meter"),
		number(simulation("rounding"), "Rounding (0-1)", "Simulation", 0, 1, 0.05,
			"How rounded the tops are: soil creeping downhill smooths them, as on old mountains. 0: crisp"),
	}
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
