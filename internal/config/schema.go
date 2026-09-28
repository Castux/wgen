package config

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Param describes an editable config value, for building a UI. Path is the
// location of the value in the JSON config, Stage the first generation stage
// that a change reruns (see gen.Stage).
type Param struct {
	Path    []string `json:"path"`
	Label   string   `json:"label"`
	Group   string   `json:"group"`
	Type    string   `json:"type"` // number, int
	Min     float64  `json:"min"`
	Max     float64  `json:"max"`
	Step    float64  `json:"step"`
	Tooltip string   `json:"tooltip,omitempty"`
	Stage   string   `json:"stage,omitempty"`
}

// Schema lists the editable parameters of the config, but the terrains.
func (c *Config) Schema() []Param {
	num := func(path []string, label, group, stage string, min, max, step float64, tooltip string) Param {
		return Param{Path: path, Label: label, Group: group, Type: "number",
			Min: min, Max: max, Step: step, Stage: stage, Tooltip: tooltip}
	}
	integer := func(path []string, label, group, stage string, min, max float64, tooltip string) Param {
		return Param{Path: path, Label: label, Group: group, Type: "int",
			Min: min, Max: max, Step: 1, Stage: stage, Tooltip: tooltip}
	}
	top := func(key string) []string { return []string{key} }
	sim := func(key string) []string { return []string{"simulation", key} }

	return []Param{
		num(top("mapWidth"), "Map width (km)", "Map", "terrain", 1, 20000, 1,
			"Real width of the map: sets the scale of everything"),
		num(top("resolution"), "Resolution (px)", "Map", "mesh", 0.5, 64, 0.5,
			"Mesh spacing of the finest level, in image pixels"),
		integer(top("levels"), "Refinement levels", "Map", "mesh", 0, 6,
			"The coarse mesh is 2^levels coarser than the finest; terrains choose how many levels refine them"),
		integer(top("seed"), "Seed", "Map", "mesh", 0, 1e9,
			"Randomness of the mesh and of the rock hardness"),

		num(sim("upliftBlur"), "Uplift ramp (km)", "Simulation", "terrain", 0, 500, 1,
			"Distance over which the uplift of a region ramps up from its border with lower terrains"),
		num(sim("erodibility"), "Erodibility", "Simulation", "terrain", 1e-8, 1e-4, 1e-7,
			"How fast rivers erode (K, per year). Lower gives steeper relief"),
		num(sim("streamExponent"), "Area exponent", "Simulation", "terrain", 0.2, 0.8, 0.01,
			"How much faster big rivers erode (m in the stream power law)"),
		num(sim("criticalSlope"), "Critical slope (deg)", "Simulation", "terrain", 5, 60, 1,
			"Steeper hillslopes collapse"),
		num(sim("timeStep"), "Time step (kyr)", "Simulation", "terrain", 1, 1000, 1, ""),
		integer(sim("steps"), "Steps (coarse)", "Simulation", "terrain", 1, 5000,
			"Time steps on the coarse mesh, for the landscape to settle"),
		integer(sim("refineSteps"), "Steps (each refinement)", "Simulation", "terrain", 0, 5000, ""),
		num(sim("erodibilityNoise"), "Rock hardness variation", "Simulation", "terrain", 0, 1, 0.01, ""),
		num(sim("noiseScale"), "Variation scale (km)", "Simulation", "terrain", 1, 1000, 1, ""),
		num(sim("floorSlope"), "Sea floor slope", "Simulation", "terrain", 0, 1, 0.001,
			"Slope of the sea and lake floors, meters per meter"),
	}
}

// Value returns the current value of a parameter, given its path (in the
// JSON format): float64 for numbers, bool or string.
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
