package viewer

import (
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/go-gl/glfw/v3.4/glfw"

	"github.com/Castux/wgen/internal/config"
	"github.com/Castux/wgen/internal/gen"
	"github.com/Castux/wgen/internal/render"
)

func TestSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wgen", "viewer.json")

	if s := loadSettings(path); s != defaultSettings {
		t.Errorf("without file: %+v", s)
	}

	s := defaultSettings
	s.View, s.Wireframe, s.Contours = "map", true, 20
	saveSettings(path, s)
	if loaded := loadSettings(path); loaded != s {
		t.Errorf("loaded %+v, saved %+v", loaded, s)
	}

	// Invalid values get their default, the others are kept
	os.WriteFile(path, []byte(`{"view": "sideways", "grid": 50}`), 0o644)
	if loaded := loadSettings(path); loaded.View != "orbit" || loaded.Grid != 50 {
		t.Errorf("loaded %+v", loaded)
	}

	os.WriteFile(path, []byte(`{"view": `), 0o644)
	if loaded := loadSettings(path); loaded != defaultSettings {
		t.Errorf("broken file: %+v", loaded)
	}

	if cycle(views, "map") != "orbit" || cycle(views, "orbit") != "map" {
		t.Error("cycle")
	}
}

func TestRoundToStep(t *testing.T) {
	for _, test := range []struct{ v, step, want float64 }{
		{0.123456, 0.001, 0.123},
		{-0.0996, 0.001, -0.1},
		{31.74, 0.5, 31.5},
		{0.7, 0.1, 0.7},
		{17, 10, 20},
		{3.3, 1, 3},
	} {
		if got := roundToStep(test.v, test.step); got != test.want {
			t.Errorf("roundToStep(%v, %v) = %v, expected %v", test.v, test.step, got, test.want)
		}
	}
}

func island(t *testing.T) *gen.World {
	t.Chdir(t.TempDir())

	img := image.NewNRGBA(image.Rect(0, 0, 60, 40))
	for y := range 40 {
		for x := range 60 {
			c := color.NRGBA{66, 66, 125, 255}
			if math.Hypot(float64(x-30), float64(y-20)) < 15 {
				c = color.NRGBA{135, 168, 81, 255}
			}
			img.Set(x, y, c)
		}
	}
	f, _ := os.Create("island.png")
	png.Encode(f, img)
	f.Close()

	conf, _, err := config.Parse([]byte(`{
		"image": "island.png",
		"mapWidth": 50, "resolution": 2, "levels": 1,
		"terrains": {
			"sea": { "color": "#42427d" },
			"land": { "color": "#87a851", "height": 500 }
		},
		"simulation": { "steps": 60, "refineSteps": 10 }
	}`))
	if err != nil {
		t.Fatal(err)
	}
	w, err := gen.New(conf)
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func TestImageSlot(t *testing.T) {
	w := island(t)

	var mu sync.Mutex
	wakes := 0
	s := imageSlot{wake: func() { mu.Lock(); wakes++; mu.Unlock() }}

	wait := func() *image.RGBA {
		t.Helper()
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			if img := s.take(); img != nil {
				return img
			}
			time.Sleep(time.Millisecond)
		}
		t.Fatal("no image rendered")
		return nil
	}

	// Only the latest of several requests is delivered
	for scale := 1.0; scale <= 4; scale++ {
		s.request(w, 1, render.Options{Scale: scale, Base: render.BaseTerrain})
	}
	if img := wait(); img.Rect.Dx() != 240 {
		t.Errorf("image width %d, expected the latest request's", img.Rect.Dx())
	}
	if s.busy() || s.take() != nil {
		t.Error("slot still busy, or result delivered twice")
	}

	// The same request again does nothing
	s.request(w, 1, render.Options{Scale: 4, Base: render.BaseTerrain})
	if s.busy() {
		t.Error("rendering the same image again")
	}

	// A new version does
	s.request(w, 2, render.Options{Scale: 4, Base: render.BaseTerrain})
	wait()

	mu.Lock()
	defer mu.Unlock()
	if wakes != 2 {
		t.Errorf("%d wake ups, expected 2", wakes)
	}
}

func TestShortcuts(t *testing.T) {
	const (
		press   = glfw.Press
		release = glfw.Release
		repeat  = glfw.Repeat
	)
	type event struct {
		key    glfw.Key
		name   string
		action glfw.Action
		mods   glfw.ModifierKey
		click  bool // a mouse button or wheel instead
	}
	shiftDown := event{key: glfw.KeyLeftShift, action: press, mods: glfw.ModShift}
	shiftUp := event{key: glfw.KeyLeftShift, action: release}

	for _, test := range []struct {
		name   string
		events []event
		want   []shortcut
	}{
		{"v", []event{{key: glfw.KeyV, name: "v", action: press}}, []shortcut{shortcutView}},
		{"tab is ImGui's", []event{{key: glfw.KeyTab, action: press}}, nil},
		{"ctrl+tab is ImGui's", []event{{key: glfw.KeyTab, action: press, mods: glfw.ModControl}}, nil},
		{"menu commands", []event{
			{key: glfw.KeyN, name: "n", action: press, mods: glfw.ModControl},
			{key: glfw.KeyO, name: "o", action: press, mods: glfw.ModControl},
			{key: glfw.KeyS, name: "s", action: press, mods: glfw.ModControl | glfw.ModShift},
			{key: glfw.KeyE, name: "e", action: press, mods: glfw.ModControl},
			{key: glfw.KeyQ, name: "q", action: press, mods: glfw.ModSuper},
			{key: glfw.KeyR, name: "r", action: press},
		}, []shortcut{shortcutNew, shortcutOpen, shortcutSaveAs, shortcutExport, shortcutQuit, shortcutReset}},
		{"shift tap", []event{shiftDown, shiftUp}, []shortcut{shortcutColor}},
		{"right shift tap", []event{{key: glfw.KeyRightShift, action: press}, {key: glfw.KeyRightShift, action: release}}, []shortcut{shortcutColor}},
		{"shift+drag", []event{shiftDown, {click: true}, shiftUp}, nil},
		{"shift+tab", []event{shiftDown, {key: glfw.KeyTab, action: press, mods: glfw.ModShift}, shiftUp}, nil},
		{"shift held", []event{shiftDown, {key: glfw.KeyLeftShift, action: repeat, mods: glfw.ModShift}, shiftUp}, []shortcut{shortcutColor}},
		{"q and w by name", []event{{key: glfw.KeyA, name: "q", action: press}, {key: glfw.KeyZ, name: "w", action: press}}, []shortcut{shortcutShading, shortcutWireframe}},
		{"held w", []event{{key: glfw.KeyW, name: "w", action: press}, {key: glfw.KeyW, action: repeat}, {key: glfw.KeyW, action: repeat}}, []shortcut{shortcutWireframe}},
		{"ctrl+w", []event{{key: glfw.KeyW, name: "w", action: press, mods: glfw.ModControl}}, nil},
		{"e, l and brackets", []event{{key: glfw.KeyE, name: "e", action: press}, {key: glfw.KeyL, name: "l", action: press},
			{key: glfw.KeyLeftBracket, name: "[", action: press}, {key: glfw.KeyRightBracket, name: "]", action: press}},
			[]shortcut{shortcutEdit, shortcutLockShore, shortcutSmaller, shortcutLarger}},
		{"undo, redo, save", []event{
			{key: glfw.KeyZ, name: "z", action: press, mods: glfw.ModControl},
			{key: glfw.KeyZ, name: "z", action: press, mods: glfw.ModControl | glfw.ModShift},
			{key: glfw.KeyY, name: "y", action: press, mods: glfw.ModSuper},
			{key: glfw.KeyS, name: "s", action: press, mods: glfw.ModControl},
		}, []shortcut{shortcutUndo, shortcutRedo, shortcutRedo, shortcutSave}},
		{"ctrl+alt+z", []event{{key: glfw.KeyZ, name: "z", action: press, mods: glfw.ModControl | glfw.ModAlt}}, nil},
		{"release", []event{{key: glfw.KeyW, action: release}}, nil},
	} {
		a := &app{}
		for _, e := range test.events {
			if e.click {
				a.onMouse()
				continue
			}
			a.onKey(e.key, e.name, e.action, e.mods)
		}
		if !slices.Equal(a.keys, test.want) {
			t.Errorf("%s: %v, expected %v", test.name, a.keys, test.want)
		}
	}
}

func TestShoreLock(t *testing.T) {
	conf, _, err := config.Parse([]byte(`{
		"image": "map.png",
		"terrains": {
			"sea": { "color": "#42427d" },
			"lake": { "color": "#6d94c2" },
			"plains": { "color": "#87a851", "height": 400 },
			"mountains": { "color": "#65481f", "height": 3000 }
		}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	sea, lake, plains, mountains := conf.Terrain("sea"), conf.Terrain("lake"), conf.Terrain("plains"), conf.Terrain("mountains")

	a := &app{settings: defaultSettings}
	if a.shoreLock(conf, mountains) != nil {
		t.Error("locked by default")
	}

	a.settings.LockShore = true
	land := a.shoreLock(conf, mountains)
	if !land(sea.Color) || !land(lake.Color) || land(plains.Color) {
		t.Error("land brush: should protect water only")
	}
	water := a.shoreLock(conf, sea)
	if !water(plains.Color) || water(lake.Color) || water(sea.Color) {
		t.Error("water brush: should protect land only (sea and lake can swap)")
	}
}

func TestImportClassification(t *testing.T) {
	conf := config.Default("map.png")
	white, black := config.Color{255, 255, 255}, config.Color{0, 0, 0}
	gray := config.Color{120, 120, 120} // antialiasing, closer to black

	// A white sea with a black island, gray on its edge
	const w, h = 20, 10
	pixels := make([]config.Color, w*h)
	for i := range pixels {
		x, y := i%w, i/w
		switch {
		case x >= 8 && x < 12 && y >= 3 && y < 7:
			pixels[i] = black
		case x >= 7 && x < 13 && y >= 2 && y < 8:
			pixels[i] = gray
		default:
			pixels[i] = white
		}
	}

	picks := defaultPicks(w, h, pixels, conf)
	if len(picks) != 2 || picks[0] != (pick{white, "sea"}) || picks[1].terrain != "plains" {
		t.Fatalf("picks %v", picks)
	}
	// The most common other color is the gray edge: pick the black instead
	picks[1].color = black

	out := classify(pixels, picks, conf)
	sea, plains := conf.Terrain("sea").Color, conf.Terrain("plains").Color
	if out[0] != sea || out[5*w+10] != plains || out[2*w+7] != plains {
		t.Errorf("classified: corner %v, island %v, edge %v", out[0], out[5*w+10], out[2*w+7])
	}

	// Colors of the terrains are recognized
	pixels = classify(pixels, picks, conf)
	picks = defaultPicks(w, h, pixels, conf)
	if !slices.Contains(picks, pick{sea, "sea"}) || !slices.Contains(picks, pick{plains, "plains"}) {
		t.Errorf("exact picks %v", picks)
	}
}
