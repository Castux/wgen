package viewer

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"slices"

	"github.com/Castux/wgen/internal/export"
	"github.com/Castux/wgen/internal/render"
)

// Settings are the viewer settings: not part of the project, saved in the
// user's config directory.
type Settings struct {
	View        string  `json:"view"`
	Color       string  `json:"color"`
	HeightScale string  `json:"heightScale"`
	Legend      bool    `json:"legend"`
	Shading     string  `json:"shading"`
	Wireframe   bool    `json:"wireframe"`
	RiverPower  float64 `json:"riverPower"`
	RiverWidth  float64 `json:"riverWidth"`
	Contours    float64 `json:"contours"`
	Grid        float64 `json:"grid"`

	VerticalScale float64 `json:"verticalScale"` // of the 3D view

	// Watching the simulation
	Watch      bool    `json:"watch"`
	WatchSteps float64 `json:"watchSteps"` // time steps per frame

	// Map editor
	Editing      bool    `json:"editing"`
	LockShore    bool    `json:"lockShore"` // brushes don't move the shoreline
	Brush        string  `json:"brush"`     // shape
	BrushRadius  float64 `json:"brushRadius"`
	PaintOpacity float64 `json:"paintOpacity"`

	// Projects
	LastProject string         `json:"lastProject"`
	Export      export.Options `json:"export"`
}

var (
	views       = []string{"orbit", "map"}
	colors      = []string{"terrain", "height"}
	shadings    = []string{"lit", "unlit"}
	heightScale = []string{render.ScaleRainbow, render.ScaleGray}
	brushes     = []string{brushNatural, brushRound, brushSquare}
)

// Brush shapes
const (
	brushNatural = "natural"
	brushRound   = "round"
	brushSquare  = "square"
)

var defaultSettings = Settings{
	View:        "orbit",
	Color:       "terrain",
	HeightScale: render.ScaleRainbow,
	Legend:      true,
	Shading:     "lit",
	RiverPower:  0.5,
	RiverWidth:  10,

	VerticalScale: 1,

	WatchSteps: 10,

	Brush:        brushNatural,
	BrushRadius:  20,
	PaintOpacity: 0.5,

	Export: export.Options{Heightmap: true, Texture: true, TextureScale: 1},
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
	valid(&s.HeightScale, heightScale, defaultSettings.HeightScale)
	valid(&s.Brush, brushes, defaultSettings.Brush)
	positive := func(value *float64, def float64) {
		if !(*value > 0) {
			*value = def
		}
	}
	positive(&s.VerticalScale, defaultSettings.VerticalScale)
	positive(&s.WatchSteps, defaultSettings.WatchSteps)
	positive(&s.BrushRadius, defaultSettings.BrushRadius)
	if !(s.PaintOpacity >= 0 && s.PaintOpacity <= 1) {
		s.PaintOpacity = defaultSettings.PaintOpacity
	}
	positive(&s.Export.TextureScale, 1)

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
		RiverPower:  s.RiverPower,
		RiverWidth:  s.RiverWidth,
		Contours:    s.Contours,
		Grid:        s.Grid,
		HeightScale: s.HeightScale,
	}
}
