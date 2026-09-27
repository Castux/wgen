package viewer

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"slices"

	"github.com/Castux/wgen/internal/render"
)

// Settings are the viewer settings: not part of the generation config, saved
// in the user's config directory.
type Settings struct {
	View       string  `json:"view"`
	Color      string  `json:"color"`
	Shading    string  `json:"shading"`
	Wireframe  bool    `json:"wireframe"`
	RiverPower float64 `json:"riverPower"`
	RiverWidth float64 `json:"riverWidth"`
	Contours   float64 `json:"contours"`
	Grid       float64 `json:"grid"`

	VerticalScale float64 `json:"verticalScale"` // of the 3D views
}

var (
	views    = []string{"orbit", "top", "map"}
	colors   = []string{"terrain", "height"}
	shadings = []string{"lit", "unlit"}
)

var defaultSettings = Settings{
	View:       "orbit",
	Color:      "terrain",
	Shading:    "lit",
	RiverPower: 0.5,
	RiverWidth: 10,

	VerticalScale: 1,
}

// settingsDir is where the viewer settings and the panel layout are saved,
// empty if there is no such directory.
func settingsDir() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "wgen")
}

// loadSettings reads the saved settings. Missing or invalid values get their
// default.
func loadSettings(path string) Settings {
	s := defaultSettings
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &s); err != nil {
			slog.Warn("ignoring broken viewer settings", "path", path, "err", err)
			return defaultSettings
		}
	}

	valid := func(value *string, values []string, def string) {
		if !slices.Contains(values, *value) {
			*value = def
		}
	}
	valid(&s.View, views, defaultSettings.View)
	valid(&s.Color, colors, defaultSettings.Color)
	valid(&s.Shading, shadings, defaultSettings.Shading)
	if !(s.VerticalScale > 0) {
		s.VerticalScale = defaultSettings.VerticalScale
	}

	return s
}

func saveSettings(path string, s Settings) {
	data, _ := json.MarshalIndent(s, "", "\t")
	err := os.MkdirAll(filepath.Dir(path), 0o755)
	if err == nil {
		err = os.WriteFile(path, data, 0o644)
	}
	if err != nil {
		slog.Warn("cannot save viewer settings", "path", path, "err", err)
	}
}

// cycle returns the value after value in values, wrapping around.
func cycle(values []string, value string) string {
	i := slices.Index(values, value)
	return values[(i+1)%len(values)]
}

// overlayOptions are the render options for rivers, contour lines and grid.
func (s *Settings) overlayOptions() render.Options {
	return render.Options{
		RiverPower: s.RiverPower,
		RiverWidth: s.RiverWidth,
		Contours:   s.Contours,
		Grid:       s.Grid,
	}
}
