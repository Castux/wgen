// Package config loads, validates, patches and saves wgen configuration files.
//
// The JSON format is the one used by the original D implementation, with two
// additions: "seed" (the random generator seed, so that runs are reproducible)
// and "exportHeightmap" being honoured.
package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"slices"
	"strconv"
	"strings"
)

type Color [3]uint8

func (c Color) String() string { return fmt.Sprintf("(%d,%d,%d)", c[0], c[1], c[2]) }

type Terrain struct {
	Name     string
	Color    Color
	Gradient float64

	// Elevation of the shore line for sea terrains. NaN for everything else:
	// water terrains without a fixed shore are lakes.
	FixedShore float64

	Smoothing bool
	Erosion   bool

	// Uplift model: rate of rock uplift in mm per year, erodibility
	// multiplier, and number of mesh refinement levels (DetailAuto: all
	// levels on land, none on water).
	Uplift      float64
	Erodibility float64
	Detail      int
}

// DetailAuto is the default terrain detail.
const DetailAuto = -1

func (t *Terrain) IsSeaTerrain() bool { return !math.IsNaN(t.FixedShore) }

type Config struct {
	// Path of the config file itself, not serialized.
	ConfigPath string `json:"-"`

	Path            string  `json:"path"`
	Resolution      float64 `json:"resolution"`
	Grid            string  `json:"grid"`
	Jitter          float64 `json:"jitter"`
	Relax           bool    `json:"relax"`
	Seed            uint64  `json:"seed"`
	SmoothingRadius float64 `json:"smoothingRadius"`
	ErosionMinFlow  int     `json:"erosionMinFlow"`
	ErosionFactor   float64 `json:"erosionFactor"`
	MaxHeight       float64 `json:"maxHeight"`
	BlurRadius      int     `json:"blurRadius"`

	// In file order
	Terrains []*Terrain `json:"-"`

	ExportOBJ       bool `json:"exportOBJ"`
	ExportSVG       bool `json:"exportSVG"`
	ExportHeightmap bool `json:"exportHeightmap"`
	PNG16           bool `json:"png16"`

	Erosion ErosionModel `json:"-"`
	Noise   SlopeNoise   `json:"-"`
	Uplift  UpliftModel  `json:"-"`
}

// UpliftModel is the elevation model. "slope" (the default) builds
// elevation up from the shores, with the slopes given by the terrains.
// "uplift" simulates the landscape: terrains give the rate at which the land
// rises, rivers erode it (stream power law, solved with the implicit scheme
// of Braun and Willett 2013), and hillslopes collapse beyond a critical
// slope. Elevations are then in meters.
//
// The simulation runs on a mesh refined Levels times: coarse first, until
// the landscape settles, then on meshes twice as fine, each starting from
// the previous result. Terrains choose how many levels refine them.
type UpliftModel struct {
	Model         string  `json:"elevationModel"`
	MapWidth      float64 `json:"mapWidth"`              // km
	Levels        int     `json:"levels"`                // refinements of the coarse mesh; resolution is that of the finest
	UpliftBlur    float64 `json:"upliftBlur"`            // km, smoothing of the uplift map
	Erodibility   float64 `json:"erodibility"`           // K, per year (for m = 0.5)
	StreamM       float64 `json:"streamExponent"`        // m, exponent of the drainage area
	CriticalSlope float64 `json:"criticalSlope"`         // degrees
	TimeStep      float64 `json:"timeStep"`              // thousands of years
	Steps         int     `json:"steps"`                 // on the coarse mesh
	RefineSteps   int     `json:"refineSteps"`           // on each finer mesh
	NoiseScale    float64 `json:"erodibilityNoiseScale"` // km
	NoiseAmount   float64 `json:"erodibilityNoise"`      // 0..1
}

// Elevation models
const (
	ModelSlope  = "slope"
	ModelUplift = "uplift"
)

var defaultUplift = UpliftModel{
	Model: ModelSlope, MapWidth: 1000, Levels: 3, UpliftBlur: 30,
	Erodibility: 2e-6, StreamM: 0.5, CriticalSlope: 30,
	TimeStep: 50, Steps: 300, RefineSteps: 60,
	NoiseScale: 30, NoiseAmount: 0.3,
}

// ErosionModel is how slopes are reduced along rivers.
//
// "step" (the default) reduces them by ErosionFactor where the flow is above
// ErosionMinFlow, once. "power" is a stream power law: the slope is
// multiplied by (A / ChannelArea) ^ -Theta where the drainage area A is
// above ChannelArea, and at least by Floor. The drainage changes with the
// elevation, so the elevation and the rivers are then computed again,
// Iterations times.
type ErosionModel struct {
	Model       string  `json:"erosionModel"`
	Theta       float64 `json:"erosionTheta"`
	ChannelArea float64 `json:"channelArea"` // square pixels
	Floor       float64 `json:"erosionFloor"`
	Iterations  int     `json:"erosionIterations"`
}

// SlopeNoise multiplies land slopes by 1 + Amplitude * n, n between -1 and 1
// from a noise of the given type: "fbm" (smooth), "ridged" (low along thin
// lines) or "worley" (low along the boundaries of cells), or "none".
type SlopeNoise struct {
	Type      string  `json:"noiseType"`
	Scale     float64 `json:"noiseScale"` // pixels, of the largest octave
	Amplitude float64 `json:"noiseAmplitude"`
	Octaves   int     `json:"noiseOctaves"`
	Stretch   float64 `json:"noiseStretch"` // elongation along Angle
	Angle     float64 `json:"noiseAngle"`   // degrees, counterclockwise from x
}

// Erosion models
const (
	ErosionStep  = "step"
	ErosionPower = "power"
)

// Noise types
const (
	NoiseNone   = "none"
	NoiseFBM    = "fbm"
	NoiseRidged = "ridged"
	NoiseWorley = "worley"
)

var defaultErosion = ErosionModel{Model: ErosionStep, Theta: 0.5, ChannelArea: 20000, Floor: 0.05, Iterations: 10}

var defaultNoise = SlopeNoise{Type: NoiseNone, Scale: 256, Amplitude: 0.5, Octaves: 3, Stretch: 1}

// Grid types
const (
	GridHex    = "hex"
	GridSquare = "square"
)

var required = []string{"path", "resolution", "grid", "jitter", "relax",
	"smoothingRadius", "erosionMinFlow", "erosionFactor", "terrains"}

var optional = []string{"seed", "maxHeight", "blurRadius",
	"exportOBJ", "exportSVG", "exportHeightmap", "png16",
	"erosionModel", "erosionTheta", "channelArea", "erosionFloor", "erosionIterations",
	"noiseType", "noiseScale", "noiseAmplitude", "noiseOctaves", "noiseStretch", "noiseAngle",
	"elevationModel", "mapWidth", "levels", "upliftBlur", "erodibility", "streamExponent", "criticalSlope",
	"timeStep", "steps", "refineSteps", "erodibilityNoiseScale", "erodibilityNoise"}

var terrainKeys = []string{"r", "g", "b", "gradient", "fixedShore", "smoothing", "erosion", "uplift", "erodibility", "detail"}

// Load reads and validates a config file. Warnings are non fatal problems,
// such as unknown keys.
func Load(path string) (conf *Config, warnings []string, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}

	conf, warnings, err = Parse(data)
	if err != nil {
		return nil, warnings, fmt.Errorf("%s: %w", path, err)
	}

	conf.ConfigPath = path
	return conf, warnings, nil
}

// Parse decodes and validates a config.
func Parse(data []byte) (*Config, []string, error) {
	conf := &Config{ExportHeightmap: true, Erosion: defaultErosion, Noise: defaultNoise, Uplift: defaultUplift}
	warnings, err := conf.apply(data, true)
	if err != nil {
		return nil, warnings, err
	}

	return conf, warnings, conf.Validate()
}

// Patch returns a copy of the config with the given partial JSON applied on
// top. Terrains are patched by name and cannot be added or removed.
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
		for _, key := range required {
			if _, ok := raw[key]; !ok {
				return nil, fmt.Errorf("missing required key %q", key)
			}
		}
	}

	for key, value := range raw {
		if !slices.Contains(required, key) && !slices.Contains(optional, key) {
			warnings = append(warnings, fmt.Sprintf("unknown key %q ignored", key))
			continue
		}

		if key == "terrains" {
			if full {
				err = c.parseTerrains(value)
			} else {
				err = c.patchTerrains(value)
			}
		} else {
			err = c.setField(key, value)
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
func (c *Config) setField(key string, value json.RawMessage) error {
	var buf bytes.Buffer
	buf.WriteString("{")
	buf.WriteString(strconv.Quote(key))
	buf.WriteString(":")
	buf.Write(value)
	buf.WriteString("}")

	// Keys of the parameter groups
	var target any = c
	switch {
	case slices.Contains(erosionKeys, key):
		target = &c.Erosion
	case slices.Contains(noiseKeys, key):
		target = &c.Noise
	case slices.Contains(upliftKeys, key):
		target = &c.Uplift
	}

	dec := json.NewDecoder(&buf)
	dec.DisallowUnknownFields()
	return dec.Decode(target)
}

var (
	erosionKeys = []string{"erosionModel", "erosionTheta", "channelArea", "erosionFloor", "erosionIterations"}
	noiseKeys   = []string{"noiseType", "noiseScale", "noiseAmplitude", "noiseOctaves", "noiseStretch", "noiseAngle"}
	upliftKeys  = []string{"elevationModel", "mapWidth", "levels", "upliftBlur", "erodibility", "streamExponent",
		"criticalSlope", "timeStep", "steps", "refineSteps", "erodibilityNoiseScale", "erodibilityNoise"}
)

type rawTerrain struct {
	R          *uint8   `json:"r"`
	G          *uint8   `json:"g"`
	B          *uint8   `json:"b"`
	Gradient   *float64 `json:"gradient"`
	FixedShore *float64 `json:"fixedShore"`
	Smoothing  *bool    `json:"smoothing"`
	Erosion    *bool    `json:"erosion"`

	Uplift      *float64 `json:"uplift"`
	Erodibility *float64 `json:"erodibility"`
	Detail      *int     `json:"detail"`
}

func (c *Config) parseTerrains(data json.RawMessage) error {
	c.Terrains = nil

	return forEachOrdered(data, func(name string, value json.RawMessage) error {
		var rt rawTerrain
		if err := decodeStrict(value, &rt); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}

		if rt.R == nil || rt.G == nil || rt.B == nil || rt.Gradient == nil {
			return fmt.Errorf("%s: r, g, b and gradient are required", name)
		}

		t := &Terrain{
			Name:        name,
			FixedShore:  math.NaN(),
			Smoothing:   true,
			Erosion:     true,
			Erodibility: 1,
			Detail:      DetailAuto,
		}
		t.update(&rt)
		c.Terrains = append(c.Terrains, t)
		return nil
	})
}

func (c *Config) patchTerrains(data json.RawMessage) error {
	return forEachOrdered(data, func(name string, value json.RawMessage) error {
		t := c.Terrain(name)
		if t == nil {
			return fmt.Errorf("unknown terrain %q", name)
		}

		var rt rawTerrain
		if err := decodeStrict(value, &rt); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}

		if rt.FixedShore != nil && !t.IsSeaTerrain() {
			return fmt.Errorf("%s: cannot add a fixed shore to a non sea terrain", name)
		}

		t.update(&rt)
		return nil
	})
}

func (t *Terrain) update(rt *rawTerrain) {
	if rt.R != nil {
		t.Color[0] = *rt.R
	}
	if rt.G != nil {
		t.Color[1] = *rt.G
	}
	if rt.B != nil {
		t.Color[2] = *rt.B
	}
	if rt.Gradient != nil {
		t.Gradient = *rt.Gradient
	}
	if rt.FixedShore != nil {
		t.FixedShore = *rt.FixedShore
	}
	if rt.Smoothing != nil {
		t.Smoothing = *rt.Smoothing
	}
	if rt.Erosion != nil {
		t.Erosion = *rt.Erosion
	}
	if rt.Uplift != nil {
		t.Uplift = *rt.Uplift
	}
	if rt.Erodibility != nil {
		t.Erodibility = *rt.Erodibility
	}
	if rt.Detail != nil {
		t.Detail = *rt.Detail
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

	if c.Resolution <= 0 {
		errs = append(errs, "resolution must be positive")
	}
	if c.Grid != GridHex && c.Grid != GridSquare {
		errs = append(errs, fmt.Sprintf("unknown grid type %q (expected hex or square)", c.Grid))
	}
	if c.Jitter < 0 {
		errs = append(errs, "jitter must be positive")
	}
	if c.SmoothingRadius < 0 {
		errs = append(errs, "smoothingRadius must be positive")
	}
	if c.BlurRadius < 0 {
		errs = append(errs, "blurRadius must be positive")
	}
	if c.ErosionFactor < 0 {
		errs = append(errs, "erosionFactor must be positive")
	}

	e := c.Erosion
	if e.Model != ErosionStep && e.Model != ErosionPower {
		errs = append(errs, fmt.Sprintf("unknown erosionModel %q (expected step or power)", e.Model))
	}
	if e.Theta < 0 {
		errs = append(errs, "erosionTheta must be positive")
	}
	if e.ChannelArea <= 0 {
		errs = append(errs, "channelArea must be positive")
	}
	if e.Floor < 0 || e.Floor > 1 {
		errs = append(errs, "erosionFloor must be between 0 and 1")
	}
	if e.Iterations < 1 {
		errs = append(errs, "erosionIterations must be at least 1")
	}

	u := c.Uplift
	if u.Model != ModelSlope && u.Model != ModelUplift {
		errs = append(errs, fmt.Sprintf("unknown elevationModel %q (expected slope or uplift)", u.Model))
	}
	if u.MapWidth <= 0 {
		errs = append(errs, "mapWidth must be positive")
	}
	if u.Levels < 0 || u.Levels > 6 {
		errs = append(errs, "levels must be between 0 and 6")
	}
	if u.UpliftBlur < 0 || u.NoiseScale <= 0 {
		errs = append(errs, "upliftBlur and erodibilityNoiseScale must be positive")
	}
	if u.Erodibility <= 0 || u.StreamM <= 0 {
		errs = append(errs, "erodibility and streamExponent must be positive")
	}
	if u.CriticalSlope <= 0 || u.CriticalSlope >= 90 {
		errs = append(errs, "criticalSlope must be between 0 and 90 degrees")
	}
	if u.TimeStep <= 0 || u.Steps < 1 || u.RefineSteps < 0 {
		errs = append(errs, "timeStep and steps must be positive")
	}
	if u.NoiseAmount < 0 || u.NoiseAmount > 1 {
		errs = append(errs, "erodibilityNoise must be between 0 and 1")
	}
	for _, t := range c.Terrains {
		if t.Erodibility < 0 {
			errs = append(errs, fmt.Sprintf("terrain %s: erodibility must be positive", t.Name))
		}
		if t.Detail < DetailAuto {
			errs = append(errs, fmt.Sprintf("terrain %s: detail must be positive (or -1, automatic)", t.Name))
		}
	}

	n := c.Noise
	if !slices.Contains([]string{NoiseNone, NoiseFBM, NoiseRidged, NoiseWorley}, n.Type) {
		errs = append(errs, fmt.Sprintf("unknown noiseType %q (expected none, fbm, ridged or worley)", n.Type))
	}
	if n.Scale <= 0 {
		errs = append(errs, "noiseScale must be positive")
	}
	if n.Amplitude < 0 || n.Amplitude > 1 {
		errs = append(errs, "noiseAmplitude must be between 0 and 1")
	}
	if n.Octaves < 1 {
		errs = append(errs, "noiseOctaves must be at least 1")
	}
	if n.Stretch < 1 {
		errs = append(errs, "noiseStretch must be at least 1")
	}
	if len(c.Terrains) == 0 {
		errs = append(errs, "no terrains defined")
	}

	seen := map[Color]string{}
	for _, t := range c.Terrains {
		if other, ok := seen[t.Color]; ok {
			errs = append(errs, fmt.Sprintf("terrains %s and %s have the same color", other, t.Name))
		}
		seen[t.Color] = t.Name
	}

	if len(errs) > 0 {
		return fmt.Errorf("invalid config: %s", strings.Join(errs, ", "))
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
	if len(a.Terrains) != len(b.Terrains) {
		return false
	}

	bm := b.TerrainsByColor()
	for _, ta := range a.Terrains {
		tb, ok := bm[ta.Color]
		if !ok || !ta.equal(tb) {
			return false
		}
	}
	return true
}

// TerrainMeshesEqual tells whether terrains give the same mesh with the
// uplift model: same colors, water or land, and details.
func TerrainMeshesEqual(a, b *Config) bool {
	if len(a.Terrains) != len(b.Terrains) {
		return false
	}

	bm := b.TerrainsByColor()
	for _, ta := range a.Terrains {
		tb, ok := bm[ta.Color]
		if !ok || ta.Detail != tb.Detail || (ta.Gradient < 0) != (tb.Gradient < 0) {
			return false
		}
	}
	return true
}

func (t *Terrain) equal(o *Terrain) bool {
	sameShore := t.FixedShore == o.FixedShore ||
		(math.IsNaN(t.FixedShore) && math.IsNaN(o.FixedShore))

	return t.Name == o.Name &&
		t.Color == o.Color &&
		t.Gradient == o.Gradient &&
		sameShore &&
		t.Smoothing == o.Smoothing &&
		t.Erosion == o.Erosion &&
		t.Uplift == o.Uplift &&
		t.Erodibility == o.Erodibility &&
		t.Detail == o.Detail
}

// Marshal serializes the config in a stable, human friendly layout (one
// terrain per line, in their original order).
func (c *Config) Marshal() []byte {
	var b bytes.Buffer
	num := func(f float64) string { return strconv.FormatFloat(f, 'g', -1, 64) }

	b.WriteString("{\n")
	fmt.Fprintf(&b, "\t\"path\": %s,\n", strconv.Quote(c.Path))
	fmt.Fprintf(&b, "\t\"resolution\": %s,\n", num(c.Resolution))
	fmt.Fprintf(&b, "\t\"grid\": %s,\n", strconv.Quote(c.Grid))
	fmt.Fprintf(&b, "\t\"jitter\": %s,\n", num(c.Jitter))
	fmt.Fprintf(&b, "\t\"relax\": %t,\n", c.Relax)
	fmt.Fprintf(&b, "\t\"seed\": %d,\n", c.Seed)
	fmt.Fprintf(&b, "\t\"smoothingRadius\": %s,\n", num(c.SmoothingRadius))
	fmt.Fprintf(&b, "\t\"erosionMinFlow\": %d,\n", c.ErosionMinFlow)
	fmt.Fprintf(&b, "\t\"erosionFactor\": %s,\n", num(c.ErosionFactor))
	fmt.Fprintf(&b, "\t\"maxHeight\": %s,\n", num(c.MaxHeight))
	fmt.Fprintf(&b, "\t\"blurRadius\": %d,\n", c.BlurRadius)

	// Experimental parameters, only when not the defaults
	var extra []string
	if e := c.Erosion; e != defaultErosion {
		extra = append(extra,
			fmt.Sprintf("\"erosionModel\": %s", strconv.Quote(e.Model)),
			fmt.Sprintf("\"erosionTheta\": %s", num(e.Theta)),
			fmt.Sprintf("\"channelArea\": %s", num(e.ChannelArea)),
			fmt.Sprintf("\"erosionFloor\": %s", num(e.Floor)),
			fmt.Sprintf("\"erosionIterations\": %d", e.Iterations))
	}
	if n := c.Noise; n != defaultNoise {
		extra = append(extra,
			fmt.Sprintf("\"noiseType\": %s", strconv.Quote(n.Type)),
			fmt.Sprintf("\"noiseScale\": %s", num(n.Scale)),
			fmt.Sprintf("\"noiseAmplitude\": %s", num(n.Amplitude)),
			fmt.Sprintf("\"noiseOctaves\": %d", n.Octaves),
			fmt.Sprintf("\"noiseStretch\": %s", num(n.Stretch)),
			fmt.Sprintf("\"noiseAngle\": %s", num(n.Angle)))
	}
	if u := c.Uplift; u != defaultUplift {
		extra = append(extra,
			fmt.Sprintf("\"elevationModel\": %s", strconv.Quote(u.Model)),
			fmt.Sprintf("\"mapWidth\": %s", num(u.MapWidth)),
			fmt.Sprintf("\"levels\": %d", u.Levels),
			fmt.Sprintf("\"upliftBlur\": %s", num(u.UpliftBlur)),
			fmt.Sprintf("\"erodibility\": %s", num(u.Erodibility)),
			fmt.Sprintf("\"streamExponent\": %s", num(u.StreamM)),
			fmt.Sprintf("\"criticalSlope\": %s", num(u.CriticalSlope)),
			fmt.Sprintf("\"timeStep\": %s", num(u.TimeStep)),
			fmt.Sprintf("\"steps\": %d", u.Steps),
			fmt.Sprintf("\"refineSteps\": %d", u.RefineSteps),
			fmt.Sprintf("\"erodibilityNoiseScale\": %s", num(u.NoiseScale)),
			fmt.Sprintf("\"erodibilityNoise\": %s", num(u.NoiseAmount)))
	}
	if len(extra) > 0 {
		b.WriteString("\n")
		for _, line := range extra {
			fmt.Fprintf(&b, "\t%s,\n", line)
		}
	}

	b.WriteString("\n\t\"terrains\": {\n")
	for i, t := range c.Terrains {
		fmt.Fprintf(&b, "\t\t%s: { \"r\": %d, \"g\": %d, \"b\": %d, \"gradient\": %s",
			strconv.Quote(t.Name), t.Color[0], t.Color[1], t.Color[2], num(t.Gradient))
		if t.IsSeaTerrain() {
			fmt.Fprintf(&b, ", \"fixedShore\": %s", num(t.FixedShore))
		}
		if !t.Smoothing {
			b.WriteString(", \"smoothing\": false")
		}
		if !t.Erosion {
			b.WriteString(", \"erosion\": false")
		}
		if t.Uplift != 0 {
			fmt.Fprintf(&b, ", \"uplift\": %s", num(t.Uplift))
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

	fmt.Fprintf(&b, "\t\"exportOBJ\": %t,\n", c.ExportOBJ)
	fmt.Fprintf(&b, "\t\"exportSVG\": %t,\n", c.ExportSVG)
	fmt.Fprintf(&b, "\t\"exportHeightmap\": %t,\n", c.ExportHeightmap)
	fmt.Fprintf(&b, "\t\"png16\": %t\n", c.PNG16)
	b.WriteString("}\n")

	return b.Bytes()
}

// Save writes the config back to its file.
func (c *Config) Save() error {
	return os.WriteFile(c.ConfigPath, c.Marshal(), 0o644)
}
