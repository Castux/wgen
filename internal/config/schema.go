package config

import "math"

// Param describes an editable config value, for building a UI. Path is the
// location of the value in the JSON config, Stage the first generation stage
// that a change reruns (see gen.Stage), empty for values that don't affect
// generation.
type Param struct {
	Path    []string `json:"path"`
	Label   string   `json:"label"`
	Group   string   `json:"group"`
	Type    string   `json:"type"` // number, int, bool, enum
	Min     float64  `json:"min,omitempty"`
	Max     float64  `json:"max,omitempty"`
	Step    float64  `json:"step,omitempty"`
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

	params := []Param{
		num("resolution", "Resolution", "Mesh", "mesh", 2, 64, 0.5),
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
	}

	params = append(params,
		boolean("exportOBJ", "OBJ", "Export", ""),
		boolean("exportSVG", "SVG", "Export", ""),
		boolean("exportHeightmap", "Heightmaps", "Export", ""),
		boolean("png16", "16 bits PNG", "Export", ""),
	)

	return params
}
