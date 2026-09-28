package config

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// Color is an sRGB color, "#rrggbb" in project files.
type Color [3]uint8

func (c Color) String() string { return fmt.Sprintf("#%02x%02x%02x", c[0], c[1], c[2]) }

// ParseColor reads a "#rrggbb" color.
func ParseColor(s string) (Color, error) {
	var c Color
	if len(s) != 7 || s[0] != '#' {
		return c, fmt.Errorf("bad color %q (expected #rrggbb)", s)
	}
	for i := range 3 {
		v, err := strconv.ParseUint(s[1+2*i:3+2*i], 16, 8)
		if err != nil {
			return c, fmt.Errorf("bad color %q (expected #rrggbb)", s)
		}
		c[i] = uint8(v)
	}
	return c, nil
}

func (c Color) MarshalJSON() ([]byte, error) { return json.Marshal(c.String()) }

func (c *Color) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	parsed, err := ParseColor(s)
	*c = parsed
	return err
}
