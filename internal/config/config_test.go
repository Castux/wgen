package config

import (
	"math"
	"strings"
	"testing"
)

const sample = `{
	"path": "map.png",
	"resolution": 8,
	"grid": "hex",
	"jitter": 1.0,
	"relax": false,
	"smoothingRadius": 20,
	"erosionMinFlow": 10,
	"erosionFactor": 0.5,
	"somethingElse": 3,
	"terrains": {
		"sea": { "r": 66, "g": 66, "b": 125, "gradient": -0.1, "fixedShore": 0.0 },
		"plains": { "r": 135, "g": 168, "b": 81, "gradient": 0.2, "erosion": true},
		"lake": { "r": 109, "g": 148, "b": 194, "gradient": 0.0001, "smoothing": false, "erosion": false}
	}
}`

func TestParse(t *testing.T) {
	c, warnings, err := Parse([]byte(sample))
	if err != nil {
		t.Fatal(err)
	}

	if len(warnings) != 1 || !strings.Contains(warnings[0], "somethingElse") {
		t.Errorf("warnings: %v", warnings)
	}

	if c.Resolution != 8 || c.Grid != GridHex || c.ErosionMinFlow != 10 || !c.ExportHeightmap {
		t.Errorf("bad values: %+v", c)
	}

	var names []string
	for _, t := range c.Terrains {
		names = append(names, t.Name)
	}
	if strings.Join(names, ",") != "sea,plains,lake" {
		t.Errorf("terrain order not preserved: %v", names)
	}

	sea, lake := c.Terrain("sea"), c.Terrain("lake")
	if !sea.IsSeaTerrain() || sea.FixedShore != 0 || !sea.Smoothing || !sea.Erosion {
		t.Errorf("bad sea: %+v", sea)
	}
	if lake.IsSeaTerrain() || lake.Smoothing || lake.Erosion || lake.Color != (Color{109, 148, 194}) {
		t.Errorf("bad lake: %+v", lake)
	}
}

func TestErrors(t *testing.T) {
	for _, tc := range []struct{ json, want string }{
		{`{}`, "missing required key"},
		{strings.Replace(sample, `"hex"`, `"tri"`, 1), "unknown grid type"},
		{strings.Replace(sample, `"r": 135, "g": 168, "b": 81`, `"r": 66, "g": 66, "b": 125`, 1), "same color"},
		{strings.Replace(sample, `"gradient": 0.2,`, ``, 1), "gradient are required"},
		{strings.Replace(sample, `"erosion": true`, `"erosoin": true`, 1), "unknown field"},
		{strings.Replace(sample, `"resolution": 8`, `"resolution": "8"`, 1), "resolution"},
	} {
		_, _, err := Parse([]byte(tc.json))
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("expected error containing %q, got %v", tc.want, err)
		}
	}
}

func TestRoundTrip(t *testing.T) {
	c, _, _ := Parse([]byte(sample))
	c2, warnings, err := Parse(c.Marshal())
	if err != nil || len(warnings) > 0 {
		t.Fatal(err, warnings)
	}

	c2.ConfigPath = c.ConfigPath
	if string(c.Marshal()) != string(c2.Marshal()) || !TerrainsEqual(c, c2) {
		t.Errorf("round trip mismatch:\n%s\n%s", c.Marshal(), c2.Marshal())
	}
}

func TestPatch(t *testing.T) {
	c, _, _ := Parse([]byte(sample))

	p, err := c.Patch([]byte(`{"resolution": 16, "terrains": {"plains": {"gradient": 0.5}}}`))
	if err != nil {
		t.Fatal(err)
	}

	if p.Resolution != 16 || p.Terrain("plains").Gradient != 0.5 || p.Terrain("sea").Gradient != -0.1 {
		t.Errorf("patch not applied: %+v", p)
	}
	if c.Resolution != 8 || c.Terrain("plains").Gradient != 0.2 {
		t.Errorf("original modified")
	}
	if TerrainsEqual(c, p) {
		t.Errorf("terrains should differ")
	}

	if _, err := c.Patch([]byte(`{"terrains": {"swamp": {"gradient": 1}}}`)); err == nil {
		t.Errorf("patching an unknown terrain should fail")
	}
	if _, err := c.Patch([]byte(`{"terrains": {"lake": {"fixedShore": 1}}}`)); err == nil {
		t.Errorf("adding a fixed shore should fail")
	}
	if _, err := c.Patch([]byte(`{"resolution": -1}`)); err == nil {
		t.Errorf("invalid patch should fail")
	}
	if !math.IsNaN(c.Terrain("plains").FixedShore) {
		t.Errorf("plains should have no fixed shore")
	}
}

func TestSchemaValues(t *testing.T) {
	c, _, err := Parse([]byte(sample))
	if err != nil {
		t.Fatal(err)
	}

	// Every parameter has a value of its type
	for _, p := range c.Schema() {
		v, err := c.Value(p.Path)
		if err != nil {
			t.Errorf("%v: %v", p.Path, err)
			continue
		}
		var ok bool
		switch p.Type {
		case "bool":
			_, ok = v.(bool)
		case "enum":
			_, ok = v.(string)
		default:
			_, ok = v.(float64)
		}
		if !ok {
			t.Errorf("%v: %T value for a %s", p.Path, v, p.Type)
		}
	}

	for _, test := range []struct {
		path []string
		want any
	}{
		{[]string{"resolution"}, 8.0},
		{[]string{"grid"}, "hex"},
		{[]string{"terrains", "sea", "smoothing"}, true},
		{[]string{"terrains", "lake", "erosion"}, false},
		{[]string{"terrains", "plains", "gradient"}, 0.2},
	} {
		if v, _ := c.Value(test.path); v != test.want {
			t.Errorf("%v: %v, expected %v", test.path, v, test.want)
		}
	}

	if _, err := c.Value([]string{"terrains", "swamp", "gradient"}); err == nil {
		t.Error("value of an unknown terrain")
	}
}
