package config

import (
	"path/filepath"
	"strings"
	"testing"
)

const sample = `{
	"image": "map.png",
	"mapWidth": 500,
	"resolution": 4,
	"levels": 2,
	"somethingElse": 3,
	"terrains": {
		"sea": { "color": "#42427d" },
		"plains": { "color": "#87a851", "height": 400, "detail": 1 },
		"mountains": { "color": "#65481f", "height": 4000, "erodibility": 0.5 }
	},
	"simulation": { "erodibility": 3e-6 }
}`

func TestParse(t *testing.T) {
	c, warnings, err := Parse([]byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "somethingElse") {
		t.Errorf("warnings: %v", warnings)
	}

	if c.MapWidth != 500 || c.Resolution != 4 || c.Levels != 2 {
		t.Errorf("parsed %+v", c)
	}
	if c.Simulation.Erodibility != 3e-6 || c.Simulation.Steps != DefaultSimulation.Steps {
		t.Errorf("simulation %+v", c.Simulation)
	}

	// The lake is added, water first
	names := []string{}
	for _, t := range c.Terrains {
		names = append(names, t.Name)
	}
	if strings.Join(names, ",") != "sea,lake,plains,mountains" {
		t.Errorf("terrains %v", names)
	}
	if c.Terrain("sea").Kind != Sea || c.Terrain("lake").Kind != Lake || c.Terrain("plains").Kind != Land {
		t.Error("kinds")
	}
	m := c.Terrain("mountains")
	if m.Color != (Color{101, 72, 31}) || m.Height != 4000 || m.Erodibility != 0.5 || m.Detail != DetailAuto || !m.Calibrated() {
		t.Errorf("mountains %+v", m)
	}
	if c.Terrain("sea").Calibrated() {
		t.Error("sea calibrated")
	}
}

func TestErrors(t *testing.T) {
	for _, bad := range []string{
		`{"terrains": {}}`,
		`{"image": "a.png", "terrains": {"sea": {"color": "red"}}}`,
		`{"image": "a.png", "terrains": {"sea": {"color": "#000000"}, "hills": {"color": "#000000"}}}`,
		`{"image": "a.png", "terrains": {"sea": {"color": "#000000"}, "hills": {"color": "#000001", "height": -1}}}`,
		`{"image": "a.png", "terrains": {"sea": {"color": "#000000", "unknown": 1}}}`,
		`{"image": "a.png", "resolution": 0, "terrains": {"sea": {"color": "#000000"}}}`,
		`{"image": "a.png", "simulation": {"steps": 0}, "terrains": {"sea": {"color": "#000000"}}}`,
	} {
		if _, _, err := Parse([]byte(bad)); err == nil {
			t.Errorf("accepted: %s", bad)
		}
	}
}

func TestRoundTrip(t *testing.T) {
	c, _, err := Parse([]byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	c2, _, err := Parse(c.Marshal())
	if err != nil {
		t.Fatal(err)
	}
	if string(c.Marshal()) != string(c2.Marshal()) || !TerrainsEqual(c, c2) || c.Simulation != c2.Simulation {
		t.Errorf("round trip:\n%s\n%s", c.Marshal(), c2.Marshal())
	}
}

func TestPatch(t *testing.T) {
	c, _, _ := Parse([]byte(sample))

	p, err := c.Patch([]byte(`{"mapWidth": 800, "simulation": {"criticalSlope": 25}, "terrains": {"plains": {"height": 300}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if p.MapWidth != 800 || p.Simulation.CriticalSlope != 25 || p.Simulation.Erodibility != 3e-6 || p.Terrain("plains").Height != 300 {
		t.Errorf("patched %+v %+v", p, p.Simulation)
	}
	if c.Terrain("plains").Height != 400 || c.MapWidth != 500 {
		t.Error("original changed")
	}

	for _, bad := range []string{`{"terrains": {"swamp": {"height": 1}}}`, `{"levels": 9}`, `{"simulation": {"nope": 1}}`} {
		if _, err := c.Patch([]byte(bad)); err == nil {
			t.Errorf("accepted %s", bad)
		}
	}
}

func TestSchemaValues(t *testing.T) {
	c, _, _ := Parse([]byte(sample))
	for _, p := range c.Schema() {
		v, err := c.Value(p.Path)
		if err != nil {
			t.Errorf("%v: %v", p.Path, err)
			continue
		}
		if _, ok := v.(float64); !ok {
			t.Errorf("%v: %T value", p.Path, v)
		}
	}
	if v, _ := c.Value([]string{"simulation", "erodibility"}); v != 3e-6 {
		t.Errorf("erodibility %v", v)
	}
	if _, err := c.Value([]string{"simulation", "nope"}); err == nil {
		t.Error("unknown parameter has a value")
	}
}

func TestDefaultAndPaths(t *testing.T) {
	c := Default("new.png")
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(c.Land()) != 3 || c.Terrain("sea") == nil || c.Terrain("lake") == nil {
		t.Errorf("default terrains %v", c.Terrains)
	}

	// The image is relative to the project file
	c.Path = filepath.Join("some", "dir", "map.json")
	if got := c.ImagePath(); got != filepath.Join("some", "dir", "new.png") {
		t.Errorf("image path %q", got)
	}
	c.Path = ""
	if c.ImagePath() != "new.png" {
		t.Error("image path without a project file")
	}

	// Free colors are free
	free := c.FreeColor(c.Terrain("plains").Color)
	if _, taken := c.TerrainsByColor()[free]; taken {
		t.Errorf("free color %v is taken", free)
	}
}

func TestColors(t *testing.T) {
	c, err := ParseColor("#0a1B2c")
	if err != nil || c != (Color{10, 27, 44}) || c.String() != "#0a1b2c" {
		t.Errorf("%v %v %s", c, err, c)
	}
	for _, bad := range []string{"0a1b2c", "#0a1b2", "#0a1b2g"} {
		if _, err := ParseColor(bad); err == nil {
			t.Errorf("accepted %s", bad)
		}
	}
}

func TestTerrainSlope(t *testing.T) {
	conf, _, err := Parse([]byte(`{
		"image": "map.png",
		"terrains": {
			"plains": { "color": "#87a851", "height": 400 },
			"cliffs": { "color": "#940a00", "height": 2000, "criticalSlope": 60 }
		},
		"simulation": { "criticalSlope": 25 }
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := conf.Terrain("plains").Slope(conf.Simulation); got != 25 {
		t.Errorf("plains: the project's slope, 25, got %g", got)
	}
	if got := conf.Terrain("cliffs").Slope(conf.Simulation); got != 60 {
		t.Errorf("cliffs: its own slope, 60, got %g", got)
	}

	// Written only when set
	again, _, err := Parse(conf.Marshal())
	if err != nil {
		t.Fatal(err)
	}
	if again.Terrain("plains").CriticalSlope != Inherit || again.Terrain("cliffs").CriticalSlope != 60 {
		t.Errorf("round trip: %+v, %+v", again.Terrain("plains"), again.Terrain("cliffs"))
	}

	conf.Terrain("cliffs").CriticalSlope = 95
	if conf.Validate() == nil {
		t.Error("a slope of 95 degrees is valid")
	}
}
