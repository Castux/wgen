# wgen

Terrain generator: paint a map where each flat color is a terrain type (sea,
plains, hills, mountains, lakes, cliffs...), and wgen turns it into a 3D
landscape with rivers, exported as OBJ, SVG and 8/16 bits heightmaps.

Elevation is not simulated from the peaks down. It is built from the sea shore
up: each terrain type gives a local slope, and every point is at the lowest
elevation reachable by climbing from the sea. River flow is then computed on
that surface, and elevation is rebuilt with gentler slopes along the bigger
rivers, which carves valleys. The irregular mesh does the rest to make it look
natural.

The interactive viewer shows the result in 3D or as a map, lets you edit
every parameter and see the effect right away, and follows changes made to
the config file and the image in other programs.

## Building

wgen is a single program, built from source with Go 1.26 and a C/C++
compiler (the viewer uses OpenGL, GLFW and Dear ImGui, which are C and C++
libraries):

- Windows: a 64 bits MinGW-w64 GCC on the `PATH`, such as the
  [WinLibs](https://winlibs.com/) or MinGW-Builds distributions.
- macOS: the Xcode command line tools (`xcode-select --install`).
- Linux: GCC and the X11 and OpenGL development packages. On Debian or
  Ubuntu: `sudo apt install build-essential libgl1-mesa-dev xorg-dev`.

```sh
go build -o bin/wgen ./cmd/wgen
```

The first build takes several minutes (compiling the ImGui bindings), later
ones a few seconds. See [DEVELOPMENT.md](DEVELOPMENT.md) if it fails.

## Usage

```sh
bin/wgen test/config.json                  # generate and export
bin/wgen --interactive test/config.json    # open the viewer
```

Flags:

- `--interactive`: open the viewer instead of exporting.
- `-v`: log the duration of each generation stage.

Exports are written next to the config: `<config>.obj`, `<config>.svg`,
`<config>.png` (elevation) and `<config>-w.png` (water level).

## Viewer

Three views, cycled with `Tab`:

| View | Left drag | Right drag | Wheel, middle drag |
|---|---|---|---|
| 3D orbit | rotate (pan with `Shift` or `Ctrl`) | pan | zoom |
| 3D top | pan | pan | zoom |
| 2D map | pan (any button) | pan | zoom at the cursor |

Double click the map to fit it in the window. "Reset view" in the panel
reframes the current view.

Keys: `Shift` switches between terrain and height colors, `q` between lit and
unlit, `w` toggles the wireframe. Shortcuts (these and `Tab`) are ignored
while typing in a field or with a dropdown open, and when combined with
`Ctrl`, `Alt` or `Cmd`. `Shift` only counts when pressed and released alone,
since it is also a modifier: panning, and horizontal scrolling in the panel.

The panel:

- View: the settings above, and the overlay drawn on the terrain: rivers,
  contour lines, grid. View settings are remembered between sessions.
- Actions: "Save config" writes the parameters back to the config file
  (reformatted, and without unknown keys). "Export files" writes the exports
  enabled in the config.
- Generation: every parameter of the config. A change only reruns the
  generation stages it affects.

Numbers have a slider, for quick changes within a typical range, rounded to
a sensible step, and a field, to type a precise value (not rounded, and
possibly outside of the slider range). Changes apply when the slider is
released, or when pressing `Enter` or leaving the field (`Escape` cancels).

- Map editor: see below.

The status in the bottom left corner shows what is running (generating,
refining a preview, rendering the overlay, exporting), errors, and whether
the config or the map have unsaved changes.

### Map editor

The map (the outline image) can be painted in the viewer: check "Paint the
map" (or press `e`), which shows the 2D view with the painted terrains over
the generated map. The left button paints the terrain selected in the panel,
the other buttons pan. Brushes lay stamps with natural looking, irregular
edges, different every time.

- `[` and `]` change the brush size, "Painting opacity" how much the
  painted map shows over the generated one.
- "Lock shoreline" (`l`) keeps the coasts as they are: land brushes leave
  sea and lakes alone, water brushes leave land alone (water can still
  change between sea and lake). To repaint the relief without touching the
  shores.
- `Ctrl+Z` undoes a stroke, `Ctrl+Y` (or `Ctrl+Shift+Z`) redoes it.
- Each stroke regenerates the world. With the uplift model, a coarse preview
  shows first ("Refining..."), and a new stroke cancels the generation in
  progress.
- "Save map" (or `Ctrl+S`, which also saves the config) writes the map to
  the config's image file. "New map..." starts from an empty sea, of a size
  to choose.

The map is only saved when asked: quitting loses unsaved changes. If the
image file changes elsewhere, the editor follows it, unless the map has
unsaved changes.

The config file and the outline image are watched: edit them in any editor
and the viewer updates. If the config file changes, it replaces any unsaved
edits made in the panel. A broken config keeps the last good result and
shows the error.

Large maps work, but take time: a 16384 x 16384 image takes about 20
seconds to load and generate, and each parameter change several seconds.

## Config

```jsonc
{
	"path": "test/Chasers.png",   // outline image, relative to the working directory
	"resolution": 32,             // mesh spacing, in pixels
	"grid": "hex",                // "hex" or "square"
	"jitter": 1.0,                // random displacement of mesh points, 0..1
	"relax": false,               // one relaxation pass for a more even mesh
	"seed": 0,                    // random seed (optional, default 0)
	"smoothingRadius": 20,        // blend slopes across terrain boundaries (0: off)
	"erosionMinFlow": 10,         // flow above which rivers carve valleys
	"erosionFactor": 0.75,        // slope multiplier along those rivers
	"maxHeight": 0,               // rescale so the highest point is this (0: off)
	"blurRadius": 0,              // blur of the heightmap (0: off)

	"terrains": {
		// r, g, b: color in the outline image. gradient: slope, negative for
		// water. fixedShore: marks sea terrains, the elevation of their shore.
		// smoothing, erosion: whether they apply (default true).
		"sea": { "r": 66, "g": 66, "b": 125, "gradient": -0.1, "fixedShore": 0.0 },
		"plains": { "r": 135, "g": 168, "b": 81, "gradient": 0.2 },
		"cliffs": { "r": 148, "g": 10, "b": 0, "gradient": 4.0, "smoothing": false, "erosion": false }
	},

	"exportOBJ": true,
	"exportSVG": false,
	"exportHeightmap": true,      // optional, default true
	"png16": true                 // 16 bits heightmaps (default 8)
}
```

Experimental parameters (optional, only saved when changed):

```jsonc
{
	// Erosion model. "step" (default): slopes along rivers are multiplied by
	// erosionFactor where the flow is above erosionMinFlow. "power": they are
	// multiplied by (drainage area / channelArea) ^ -erosionTheta, at least
	// erosionFloor, and the rivers and elevation are computed again,
	// erosionIterations times, which carves branching valleys into mountains.
	"erosionModel": "power",
	"erosionTheta": 0.5,
	"channelArea": 300,           // square pixels
	"erosionFloor": 0.05,
	"erosionIterations": 10,

	// Noise on land slopes: "none" (default), "fbm", "ridged" (slopes lower
	// along thin lines) or "worley" (lower along the boundaries of cells).
	// Slopes are multiplied by 1 +- noiseAmplitude.
	"noiseType": "ridged",
	"noiseScale": 128,            // pixels, largest octave
	"noiseAmplitude": 0.5,
	"noiseOctaves": 3,
	"noiseStretch": 1,            // elongation of the features along noiseAngle
	"noiseAngle": 0               // degrees
}
```

## Uplift model

With `"elevationModel": "uplift"`, elevation is not built from slopes but
simulated: the land rises, rivers erode it, hillslopes collapse beyond a
critical slope, and the sea stays at its level. River networks grow into the
rising land from the coasts, which gives branching valleys, winding ridges
and many peaks, at realistic heights: elevations are in meters, for a map of
the given width.

Terrains are height classes: each gives a target height for the summits of
its regions ("mountains here, about 4500 m"), and the uplift that reaches
it is found by the simulation, for every region on its own (a small range
needs to rise faster than a large one). Summits end up within about 20% of
their targets: the target is what the high points of a region reach, most of
the region is lower, valleys much lower.

```jsonc
{
	"elevationModel": "uplift",
	"mapWidth": 1000,             // km
	"resolution": 2,              // mesh spacing of the finest level, pixels
	"levels": 3,                  // the coarse mesh is 2^levels coarser

	"terrains": {
		// gradient still tells water (negative) from land, and is the slope
		// of the sea floor, in meters per meter
		"sea": { "r": 66, "g": 66, "b": 125, "gradient": -0.01, "fixedShore": 0.0 },
		// height: target summit height, meters. detail: how many levels
		// refine this terrain (default: all on land, none on water)
		"plains": { "r": 135, "g": 168, "b": 81, "gradient": 0.2, "height": 400, "detail": 1 },
		"hills": { "r": 209, "g": 184, "b": 134, "gradient": 0.8, "height": 1500, "detail": 2 },
		"mountains": { "r": 101, "g": 72, "b": 31, "gradient": 1.2, "height": 4500 },
		// water without fixedShore: a lake, eroded flat down to its outlet
		"lake": { "r": 109, "g": 148, "b": 194, "gradient": -0.001, "detail": 1 }
	}
}
```

`lab/classes.json` is a complete example. Optional parameters, with their
defaults:

| Key | Default | |
|---|---|---|
| `upliftBlur` | 30 | km: uplift ramps up over this distance from the border of a region with lower terrains, so that ranges rise more in their core |
| `erodibility` | 2e-6 | per year: how fast rivers erode. Lower gives higher relief |
| `streamExponent` | 0.5 | how erosion grows with the drainage area |
| `criticalSlope` | 30 | degrees: steeper hillslopes collapse |
| `timeStep` | 50 | thousands of years |
| `steps` | 300 | time steps on the coarse mesh, until the landscape settles |
| `refineSteps` | 60 | time steps on each finer mesh |
| `erodibilityNoise` | 0.3 | 0..1: variation of the rock hardness |
| `erodibilityNoiseScale` | 30 | km |

Terrains can also have an `erodibility` factor (default 1). Instead of a
`height`, a terrain can give its `uplift` directly, in mm per year (as in
`lab/uplift.json`): heights then depend on the size of its regions and on
their neighbours.

The simulation takes seconds: about 8 on the Chasers map (160 000
vertices), with a preview after 3. Each refinement level multiplies the
vertices of the refined terrains by 4.

At true scale, mountains are small on a continent: 10 km high on 1000 km
wide. They show when zooming in, and "Vertical exaggeration" in the viewer
scales them in the 3D views. Contour intervals are in meters. Heightmap
PNGs are in meters too: use `png16`, or `maxHeight` to rescale. OBJ
exports have the elevations converted to pixels, for true proportions.

Heightmap PNGs contain the raw elevations, clamped to the pixel range (0..255
or 0..65535), so water is 0. Use `maxHeight` to choose the scale.

Mesh points on a color that matches no terrain are reported as warnings in
the log, and get no terrain, as if they were outside the map.
