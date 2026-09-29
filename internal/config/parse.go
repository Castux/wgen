package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strconv"
)

// Keys of the project file, at the top level
var topKeys = []string{"image", "mapWidth", "resolution", "levels", "seed", "terrains", "simulation"}

// Load reads and validates a project file. Warnings are non fatal problems,
// such as unknown keys.
func Load(path string) (conf *Config, warnings []string, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	conf, warnings, err = Parse(data)
	if conf != nil {
		conf.Path = path
	}
	return conf, warnings, err
}

// Parse reads and validates a project from its JSON.
func Parse(data []byte) (*Config, []string, error) {
	conf := withDefaults("")
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
	patched := c.Clone()
	if _, err := patched.apply(data, false); err != nil {
		return nil, err
	}
	return patched, patched.Validate()
}

// apply sets the values of a project JSON: a whole project file (full), which
// must have the required keys and gives all the terrains, or a patch.
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

// rawTerrain is a terrain in the project file: absent values are nil.
type rawTerrain struct {
	Color         *Color   `json:"color"`
	Height        *float64 `json:"height"`
	Erodibility   *float64 `json:"erodibility"`
	CriticalSlope *float64 `json:"criticalSlope"`
	Detail        *int     `json:"detail"`
}

// applyTerrains sets the terrains of a project JSON, in file order. A full
// project replaces them, a patch changes existing ones.
func (c *Config) applyTerrains(data json.RawMessage, full bool) error {
	if full {
		c.Terrains = nil
	}

	return forEachOrdered(data, func(name string, value json.RawMessage) error {
		var raw rawTerrain
		if err := decodeStrict(value, &raw); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}

		t := c.Terrain(name)
		switch {
		case t == nil && !full:
			return fmt.Errorf("unknown terrain %q", name)
		case t == nil:
			if raw.Color == nil {
				return fmt.Errorf("%s: color is required", name)
			}
			t = NewTerrain(name, *raw.Color)
			c.Terrains = append(c.Terrains, t)
		}

		if raw.Color != nil {
			t.Color = *raw.Color
		}
		if raw.Height != nil {
			t.Height = *raw.Height
		}
		if raw.Erodibility != nil {
			t.Erodibility = *raw.Erodibility
		}
		if raw.CriticalSlope != nil {
			t.CriticalSlope = *raw.CriticalSlope
		}
		if raw.Detail != nil {
			t.Detail = *raw.Detail
		}
		return nil
	})
}

// decodeStrict decodes JSON, rejecting unknown keys.
func decodeStrict(data []byte, v any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	return decoder.Decode(v)
}

// forEachOrdered iterates over the members of a JSON object in file order.
func forEachOrdered(data []byte, f func(key string, value json.RawMessage) error) error {
	decoder := json.NewDecoder(bytes.NewReader(data))

	token, err := decoder.Token()
	if err != nil {
		return err
	}
	if token != json.Delim('{') {
		return fmt.Errorf("expected an object")
	}

	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return err
		}

		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return err
		}

		if err := f(key.(string), value); err != nil {
			return err
		}
	}
	return nil
}
