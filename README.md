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
| 3D orbit | rotate (pan with `Ctrl`) | pan | zoom |
| 3D top | pan | pan | zoom |
| 2D map | pan (any button) | pan | zoom at the cursor |

Double click the map to fit it in the window. "Reset view" in the panel
reframes the current view.

Keys: `Shift` switches between terrain and height colors, `q` between lit and
unlit, `w` toggles the wireframe.

The panel:

- View: the settings above, and the overlay drawn on the terrain: rivers,
  contour lines, grid. The overlay is drawn on the CPU, so its sliders only
  apply when released. View settings are remembered between sessions.
- Actions: "Save config" writes the parameters back to the config file
  (reformatted, and without unknown keys). "Export files" writes the exports
  enabled in the config.
- Generation: every parameter of the config. A change only reruns the
  generation stages it affects, and applies when the slider is released.
  `Ctrl` + click a slider to type a value.

The status in the bottom left corner shows what is running (generating,
rendering the overlay, exporting), errors, and whether the config has unsaved
changes.

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

Heightmap PNGs contain the raw elevations, clamped to the pixel range (0..255
or 0..65535), so water is 0. Use `maxHeight` to choose the scale.

Mesh points on a color that matches no terrain are reported as warnings in
the log, and get no terrain, as if they were outside the map.
