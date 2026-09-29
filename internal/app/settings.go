package app

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"slices"

	"github.com/Castux/wgen/internal/export"
	"github.com/Castux/wgen/internal/render"
)

// Settings are the app settings: not part of the project, saved in the
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
	BasinColors bool    `json:"basinColors"` // rivers in a color per drainage basin
	Contours    float64 `json:"contours"`
	Grid        float64 `json:"grid"`

	VerticalScale float64 `json:"verticalScale"` // of the 3D view

	// Inspecting: what is under the cursor, measuring with rulers
	HoverInfo bool `json:"hoverInfo"`
	Measuring bool `json:"measuring"`

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

// Values of the settings, as saved
const (
	// Views
	viewOrbit = "orbit"
	viewMap   = "map"

	// Colors: the render bases of the same names
	colorTerrain = string(render.BaseTerrain)
	colorHeight  = string(render.BaseHeight)

	// Shadings
	shadingLit   = "lit"
	shadingUnlit = "unlit"

	// Brush shapes
	brushNatural = "natural"
	brushRound   = "round"
	brushSquare  = "square"
)

var (
	views        = []string{viewOrbit, viewMap}
	colorModes   = []string{colorTerrain, colorHeight}
	shadings     = []string{shadingLit, shadingUnlit}
	heightScales = []string{render.ScaleRainbow, render.ScaleGray}
	brushes      = []string{brushNatural, brushRound, brushSquare}
)

// Brush radius, in map pixels: its limits, and the factor of the [ and ]
// shortcuts
const (
	minBrushRadius  = 1
	maxBrushRadius  = 500
	brushRadiusStep = 1.25
)

var defaultSettings = Settings{
	View:        viewOrbit,
	Color:       colorTerrain,
	HeightScale: render.ScaleRainbow,
	Legend:      true,
	Shading:     shadingLit,
	RiverPower:  0.5,
	RiverWidth:  10,

	VerticalScale: 1,

	HoverInfo: true,

	WatchSteps: 10,

	Brush:        brushNatural,
	BrushRadius:  20,
	PaintOpacity: 0.5,

	Export: export.Options{Heightmap: true, Texture: true, TextureScale: 1},
}

// settingsDir is where the app settings and the panel layout are saved,
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
	settings := defaultSettings
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &settings); err != nil {
			slog.Warn("ignoring broken app settings", "path", path, "err", err)
			return defaultSettings
		}
	}

	valid := func(value *string, values []string, fallback string) {
		if !slices.Contains(values, *value) {
			*value = fallback
		}
	}
	valid(&settings.View, views, defaultSettings.View)
	valid(&settings.Color, colorModes, defaultSettings.Color)
	valid(&settings.Shading, shadings, defaultSettings.Shading)
	valid(&settings.HeightScale, heightScales, defaultSettings.HeightScale)
	valid(&settings.Brush, brushes, defaultSettings.Brush)
	settings.keepConsistent()
	positive := func(value *float64, fallback float64) {
		if !(*value > 0) {
			*value = fallback
		}
	}
	positive(&settings.VerticalScale, defaultSettings.VerticalScale)
	positive(&settings.WatchSteps, defaultSettings.WatchSteps)
	positive(&settings.BrushRadius, defaultSettings.BrushRadius)
	if !(settings.PaintOpacity >= 0 && settings.PaintOpacity <= 1) {
		settings.PaintOpacity = defaultSettings.PaintOpacity
	}
	positive(&settings.Export.TextureScale, 1)

	return settings
}

func saveSettings(path string, settings Settings) {
	data, _ := json.MarshalIndent(settings, "", "\t")
	err := os.MkdirAll(filepath.Dir(path), 0o755)
	if err == nil {
		err = os.WriteFile(path, data, 0o644)
	}
	if err != nil {
		slog.Warn("cannot save app settings", "path", path, "err", err)
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
		BasinColors: s.BasinColors,
		Contours:    s.Contours,
		Grid:        s.Grid,
		HeightScale: s.HeightScale,
	}
}

// painting tells whether the map is being painted: the left button paints,
// in the map or on the terrain in 3D.
func (s *Settings) painting() bool { return s.Editing }

// keepConsistent keeps one use of the left button: painting and measuring
// both take it, painting wins.
func (s *Settings) keepConsistent() {
	if s.Editing {
		s.Measuring = false
	}
}

// setEditing turns painting on or off. Painting stops measuring.
func (s *Settings) setEditing(editing bool) {
	s.Editing = editing
	if editing {
		s.Measuring = false
	}
}

// setMeasuring turns measuring on or off. Measuring stops painting.
func (s *Settings) setMeasuring(measuring bool) {
	s.Measuring = measuring
	if measuring {
		s.Editing = false
	}
}

// initSettings loads the settings saved in dir, if any.
func (a *app) initSettings(dir string) {
	if dir == "" {
		a.settings = defaultSettings
		return
	}
	a.settingsPath = filepath.Join(dir, "viewer.json") // named when the app was a viewer
	a.settings = loadSettings(a.settingsPath)
	a.recentPath = filepath.Join(dir, "recent.json")
	a.recent = loadRecent(a.recentPath)
	if len(a.recent) == 0 && a.settings.LastProject != "" {
		a.recent = []string{a.settings.LastProject}
	}
}

// setSettings changes the settings, and saves them.
func (a *app) setSettings(settings Settings) {
	settings.keepConsistent()
	watch := settings.Watch != a.settings.Watch || settings.WatchSteps != a.settings.WatchSteps
	a.settings = settings
	if watch {
		a.applyWatch()
	}
	if a.settingsPath != "" {
		saveSettings(a.settingsPath, settings)
	}
}

// applyWatch tells the engine whether to show the simulation as it runs.
func (a *app) applyWatch() {
	steps := 0
	if a.settings.Watch {
		steps = max(1, int(a.settings.WatchSteps))
	}
	a.session.Engine.SetWatch(steps)
}
