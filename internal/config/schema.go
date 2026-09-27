package config

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

// Param describes an editable config value, for building a UI. Path is the
// location of the value in the JSON config, Stage the first generation stage
// that a change reruns (see gen.Stage), empty for values that don't affect
// generation.
type Param struct {
	Path    []string `json:"path"`
	Label   string   `json:"label"`
	Group   string   `json:"group"`
	Type    string   `json:"type"` // number, int, bool, enum
	Min     float64  `json:"min"`
	Max     float64  `json:"max"`
	Step    float64  `json:"step"`
	Options []string `json:"options,omitempty"`
	Stage   string   `json:"stage,omitempty"`
}

// Schema lists the editable parameters of the config. Terrain parameters
// depend on the terrains defined.
func (c *Config) Schema() []Param {
	num := func(key, label, group, stage string, min, max, step float64) Param {
		return Param{Path: []string{key}, Label: label, Group: group, Type: "number",
			Min: min, Max: max, Step: step, Stage: stage}
	}
	integer := func(key, label, group, stage string, min, max float64) Param {
		return Param{Path: []string{key}, Label: label, Group: group, Type: "int",
			Min: min, Max: max, Step: 1, Stage: stage}
	}
	boolean := func(key, label, group, stage string) Param {
		return Param{Path: []string{key}, Label: label, Group: group, Type: "bool", Stage: stage}
	}

	uplift := c.Uplift.Model == ModelUplift

	params := []Param{
		{Path: []string{"elevationModel"}, Label: "Elevation model", Group: "Mesh", Type: "enum",
			Options: []string{ModelSlope, ModelUplift}, Stage: "mesh"},
		num("resolution", "Resolution", "Mesh", "mesh", 1, 64, 0.5),
		{Path: []string{"grid"}, Label: "Grid", Group: "Mesh", Type: "enum",
			Options: []string{GridHex, GridSquare}, Stage: "mesh"},
		num("jitter", "Jitter", "Mesh", "mesh", 0, 1, 0.01),
		boolean("relax", "Relax", "Mesh", "mesh"),
		integer("seed", "Seed", "Mesh", "mesh", 0, 9999),

		num("smoothingRadius", "Smoothing radius", "Elevation", "terrain", 0, 100, 1),
		num("maxHeight", "Max height (0: off)", "Elevation", "terrain", 0, 65535, 1),
		integer("erosionMinFlow", "Erosion min flow", "Elevation", "erosion", 0, 500),
		num("erosionFactor", "Erosion factor", "Elevation", "erosion", 0, 1, 0.01),
		integer("blurRadius", "Blur radius", "Elevation", "raster", 0, 20),
	}

	if uplift {
		const group = "Uplift model"
		params = append(params,
			num("mapWidth", "Map width (km)", group, "terrain", 10, 10000, 10),
			integer("levels", "Refinement levels", group, "mesh", 0, 6),
			num("upliftBlur", "Uplift blur (km)", group, "terrain", 0, 200, 1),
			num("erodibility", "Erodibility (K)", group, "terrain", 1e-7, 1e-5, 1e-7),
			num("streamExponent", "Area exponent (m)", group, "terrain", 0.3, 0.7, 0.01),
			num("criticalSlope", "Critical slope (deg)", group, "terrain", 5, 60, 1),
			num("timeStep", "Time step (kyr)", group, "terrain", 1, 500, 1),
			integer("steps", "Steps (coarse)", group, "terrain", 1, 2000),
			integer("refineSteps", "Steps (each refinement)", group, "terrain", 0, 1000),
			num("erodibilityNoise", "Erodibility noise", group, "terrain", 0, 1, 0.01),
			num("erodibilityNoiseScale", "Noise scale (km)", group, "terrain", 1, 500, 1),
		)
	}

	params = append(params,
		Param{Path: []string{"erosionModel"}, Label: "Model", Group: "Erosion (experimental)", Type: "enum",
			Options: []string{ErosionStep, ErosionPower}, Stage: "erosion"},
		num("erosionTheta", "Theta (power)", "Erosion (experimental)", "erosion", 0, 1, 0.01),
		num("channelArea", "Channel area (power)", "Erosion (experimental)", "erosion", 100, 200000, 100),
		num("erosionFloor", "Floor (power)", "Erosion (experimental)", "erosion", 0, 1, 0.01),
		integer("erosionIterations", "Iterations (power)", "Erosion (experimental)", "erosion", 1, 50),

		Param{Path: []string{"noiseType"}, Label: "Type", Group: "Slope noise (experimental)", Type: "enum",
			Options: []string{NoiseNone, NoiseFBM, NoiseRidged, NoiseWorley}, Stage: "terrain"},
		num("noiseScale", "Scale", "Slope noise (experimental)", "terrain", 16, 2048, 1),
		num("noiseAmplitude", "Amplitude", "Slope noise (experimental)", "terrain", 0, 1, 0.01),
		integer("noiseOctaves", "Octaves", "Slope noise (experimental)", "terrain", 1, 8),
		num("noiseStretch", "Stretch", "Slope noise (experimental)", "terrain", 1, 8, 0.1),
		num("noiseAngle", "Angle", "Slope noise (experimental)", "terrain", -90, 90, 1),
	)

	for _, t := range c.Terrains {
		group := "Terrain: " + t.Name
		path := func(key string) []string { return []string{"terrains", t.Name, key} }

		// Keep the sign of the gradient: it decides between land and water
		gradient := Param{Path: path("gradient"), Label: "Gradient", Group: group,
			Type: "number", Step: 0.001, Stage: "terrain"}
		if t.Gradient < 0 {
			gradient.Min, gradient.Max = math.Min(-1, 2*t.Gradient), -0.001
		} else {
			gradient.Min, gradient.Max = 0, math.Max(5, 2*t.Gradient)
		}
		params = append(params, gradient)

		if t.IsSeaTerrain() {
			params = append(params, Param{Path: path("fixedShore"), Label: "Shore elevation",
				Group: group, Type: "number", Min: -50, Max: 50, Step: 0.1, Stage: "terrain"})
		}

		params = append(params,
			Param{Path: path("smoothing"), Label: "Smoothing", Group: group, Type: "bool", Stage: "terrain"},
			Param{Path: path("erosion"), Label: "Erosion", Group: group, Type: "bool", Stage: "terrain"},
		)

		if uplift {
			params = append(params,
				Param{Path: path("height"), Label: "Target height (m, 0: off)", Group: group, Type: "number",
					Min: 0, Max: 9000, Step: 10, Stage: "terrain"},
				Param{Path: path("uplift"), Label: "Uplift (mm/yr, without height)", Group: group, Type: "number",
					Min: 0, Max: 10, Step: 0.01, Stage: "terrain"},
				Param{Path: path("erodibility"), Label: "Erodibility factor", Group: group, Type: "number",
					Min: 0, Max: 10, Step: 0.01, Stage: "terrain"},
				Param{Path: path("detail"), Label: "Detail levels (-1: auto)", Group: group, Type: "int",
					Min: -1, Max: 6, Step: 1, Stage: "mesh"},
			)
		}
	}

	params = append(params,
		boolean("exportOBJ", "OBJ", "Export", ""),
		boolean("exportSVG", "SVG", "Export", ""),
		boolean("exportHeightmap", "Heightmaps", "Export", ""),
		boolean("png16", "16 bits PNG", "Export", ""),
	)

	return params
}

// Value returns the current value of a parameter, given its schema path:
// float64 for numbers, bool or string.
func (c *Config) Value(path []string) (any, error) {
	if len(path) == 3 && path[0] == "terrains" {
		t := c.Terrain(path[1])
		if t == nil {
			return nil, fmt.Errorf("no terrain %q", path[1])
		}
		switch path[2] {
		case "gradient":
			return t.Gradient, nil
		case "fixedShore":
			return t.FixedShore, nil
		case "smoothing":
			return t.Smoothing, nil
		case "erosion":
			return t.Erosion, nil
		case "height":
			return t.Height, nil
		case "uplift":
			return t.Uplift, nil
		case "erodibility":
			return t.Erodibility, nil
		case "detail":
			return float64(t.Detail), nil
		}
	}

	if len(path) == 1 && path[0] != "terrains" {
		// The parameter groups are only serialized when not the defaults
		var values map[string]any
		for _, data := range [][]byte{c.Marshal(), jsonOf(c.Erosion), jsonOf(c.Noise), jsonOf(c.Uplift)} {
			if err := json.Unmarshal(data, &values); err != nil {
				return nil, err
			}
		}
		if v, ok := values[path[0]]; ok {
			return v, nil
		}
	}

	return nil, fmt.Errorf("unknown parameter %s", strings.Join(path, "."))
}

func jsonOf(v any) []byte {
	data, _ := json.Marshal(v)
	return data
}
