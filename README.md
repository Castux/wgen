# wgen

Paint a map, get a landscape. Each color of the map is a terrain: sea,
lakes, and land terrains such as plains, hills and mountains, each with a
target height for its summits. wgen simulates the land rising from the sea
and rivers eroding it, over millions of years, which gives branching valleys,
winding ridges and river networks at realistic heights. Export the result as
a heightmap, a textured 3D mesh or a vector map.

- Paint in the app, with natural looking brushes, or import an image painted
  anywhere else.
- See the landscape in 3D or as a map, colored by terrain or by height.
- Every change regenerates the landscape: a coarse preview first, then the
  full detail, in seconds.

## Download

Ready to use builds for Windows, macOS and Linux are on the
[releases page](https://github.com/Castux/wgen/releases). Each has an
example project, in `example/`.

- Windows: unzip, and double click `wgen.exe`. `wgen-cli.exe` is the same
  program, for the command line.
- macOS: unzip, and move `wgen.app` to Applications. The app isn't
  notarized by Apple: the first time, right click it and choose Open (or,
  on recent versions, allow it in System Settings, Privacy & Security).
- Linux: extract, and run `wgen`. It needs OpenGL 3.3, and X11 or Wayland.

## Using the app

The menus have the commands and how the landscape is shown: File, Edit
(undo and redo), View (every display setting), Simulation (watching it),
Help. The panel on the right is the project: its terrains, painting, and
the map and simulation parameters.

At first, wgen starts a new map with a random island, and shows the
welcome card: what it does, buttons to paint this map, open the example,
import an image, start a new map or open a project, and the recent
projects. Uncheck "Show this at startup" to skip it; Help, Welcome shows it
again. From the File menu:

- New map: an empty sea or a random island, of a chosen size in pixels
  (powers of two, 256 to 16384) and a real width in kilometers.
- Open: a project (`.json`), or an image to import as a map (`.png`,
  `.jpg`). Files can also be dropped on the window.
- Open recent: the last 10 projects opened or saved.
- Open the example: a finished map, as a new untitled project (saving it
  asks where, so the example stays as it is).
- Save, Save as: a project is a `.json` file and its map, a `.png` image
  next to it with the same name.
- Export: see below.

The app reopens the last project at startup. The window title shows a star
when there are unsaved changes, and the app asks before losing them.

While there are unsaved changes, a copy of the project is saved every
minute, next to the app's settings. If wgen closes without saving them (a
crash, the computer turned off), the next start offers to recover them: the
map opens as it was, and saving it suggests its file. Saving the project,
or quitting, removes the copy.

### Views and controls

| Control | |
|---|---|
| Left drag | 3D: rotate (pan with `Shift` or `Ctrl`). Map: pan. Painting: paint |
| Right, middle drag | Pan (middle: zoom, in 3D). Painting in 3D: right rotates |
| Wheel | Zoom |
| Double click | Map: fit the map in the window |
| `V` | 3D or map view |
| `Shift` (alone) | Terrain or height colors |
| `Q`, `W` | Lit or unlit, wireframe |
| `R` | Reset the view |
| `E` | Paint the map |
| `[`, `]` | Smaller, larger brush |
| `L` | Lock the shoreline |
| `M` | Measure with rulers |
| `Enter`, `Escape`, `Delete` | Finish the ruler, deselect it, delete it |
| `Ctrl+Z`, `Ctrl+Y` | Undo, redo painting |
| `Ctrl+N`, `O`, `S`, `Shift+S`, `E`, `Q` | New, open, save, save as, export, quit |

On macOS, `Cmd` works as `Ctrl`. Help, Controls lists these in the app.
Keys are ignored while typing in a field.

Height colors are a rainbow scale (Turbo) on land, or gray, and blue in
water, darker when deeper. The legend in the top left corner tells the
heights.

The View menu also has the vertical exaggeration of the 3D view (mountains
are small on a continent: 10 km high on 1000 km wide), and the overlay:
rivers, contour lines and a grid.

### Inspecting

The line at the top of the window tells what is under the cursor, in both
views: the position (km from the top left corner), the terrain, the
elevation (or the water depth, and a lake's surface), and the drainage area
there, which is the size of the river. View, Information under the cursor
turns it off.

Rulers measure distances and show altitude profiles. Press `M` (or View,
Measure with rulers), then click to add points, in the map or in 3D; double
click or press `Enter` to finish. Drags still move the view. The length
shows at the end of each ruler. Click a ruler to select it: the Altitude
profile window shows the ground along it in the height colors, water in
blue, with its length, lowest and highest points, and the total climb and
descent; hovering the graph marks the point on the map. `Delete` removes the
selected ruler, `Escape` deselects it. Rulers aren't saved with the project.

### Painting

Press `E` (or check "Paint the map" in the Painting section of the panel):
the painted terrains show over the generated landscape, and the left button
paints the terrain selected in the panel, in the map view or on the terrain
in 3D, where the brush follows the relief. In the map view, the other
buttons pan; in 3D, the right button rotates (pans with `Shift` or `Ctrl`)
and the middle one zooms.

- Brushes: natural (irregular edges, different every stamp), hard round,
  hard square. `[` and `]` change the size.
- Lock shoreline: land brushes leave the sea and lakes alone, and water
  brushes the land, to repaint the relief without moving the coasts.
- Each stroke regenerates the landscape: a coarse preview shows first, and
  a new stroke cancels the generation in progress.

### Terrains

The Terrains section of the panel lists the terrains, which are also the
brush colors. The sea and lakes are always there: the sea is at sea level,
and lakes are flat, at the level of their lowest shore. Land terrains can be
added, removed, renamed, and recolored (which recolors the map too). For
each:

- Target height: the summits of each region of this terrain reach about
  this height, in meters. Most of a region is lower, and its valleys much
  lower: the heights come from the simulation, the target sets how fast the
  land rises.
- Erodibility factor: how easily it erodes, compared to the others.
- Detail levels: how many times the mesh is refined on it. More detail is
  slower, and gives finer valleys.

### Importing an image

Opening an image that has no project next to it imports it. The import
dialog shows the colors of the image: pick the color of the sea, and of each
terrain, by clicking the image or the list, and choose their terrains. wgen
guesses a first choice (the border color as the sea, the other main colors
as land). Every pixel then takes the terrain of the closest picked color,
which also cleans up antialiased edges: "Show the result" previews it.

A map with only land and sea is fine: pick one color as the sea, the other
as plains, and paint hills and mountains in the app.

Saving the imported map creates a project next to the image, with a
`.json` of the same name. Opening an image that has a project opens the
project.

### Exporting

File, Export writes, next to the project by default:

| File | |
|---|---|
| `<name>-height.png` | Heightmap, 16 bits grayscale: in meters above sea level (water at 0), or normalized from the deepest to the highest point |
| `<name>-water.png` | Water mask: white where there is water |
| `<name>-texture.png` | Texture: terrain colors, hillshading and rivers, at a chosen scale of the map |
| `<name>.obj` | 3D mesh, with UVs for the texture, at true proportions |
| `<name>.svg` | Vector map of the terrain cells and rivers |

### Parameters

The Map section of the panel:

- Map width (km): the real width of the map, which sets the scale of
  everything.
- Resolution (px): the mesh spacing of the finest detail level, in pixels
  of the map. Lower is finer, and slower.
- Refinement levels: the coarse mesh is 2^levels coarser than the finest.
- Seed: the randomness of the mesh and of the rock hardness.

The Simulation section has the parameters of the erosion model (hover them
for help). The Simulation menu has "Watch the simulation", which shows the
landscape as it is simulated: the land rising from the sea, rivers cutting
in, the heights being calibrated, then each finer mesh. "Replay the
simulation" runs it again, to watch it.

The project file and its map are watched: edit them in another program, and
the app follows.

## Command line

```sh
wgen [flags] [project.json | image.png]
```

Without flags, opens the app with the given project or image. With
`-export`, generates the project and writes the files without opening a
window:

```sh
wgen -export heightmap,texture,obj example/chasers.json
wgen -export heightmap -normalized -o out/map example/chasers.json
```

| Flag | |
|---|---|
| `-export list` | comma separated: `heightmap`, `water`, `texture`, `obj`, `svg` |
| `-normalized` | heightmap from the lowest to the highest point, instead of meters |
| `-o path` | output path, without extension (default: next to the project) |
| `-v` | verbose logging (stage timings) |
| `-version` | print the version |

The app also logs to `wgen.log` in the user config directory
(`%APPDATA%\wgen` on Windows, `~/Library/Application Support/wgen` on
macOS, `~/.config/wgen` on Linux), with its settings.

## Project file

```jsonc
{
	"image": "chasers.png",       // the map, relative to the project file
	"mapWidth": 1000,             // km
	"resolution": 2,              // mesh spacing of the finest level, pixels
	"levels": 3,                  // the coarse mesh is 2^levels coarser
	"seed": 0,

	"terrains": {
		// sea and lake are always there. detail: how many levels refine a
		// terrain (default: one on water, all on land)
		"sea": { "color": "#42427d" },
		"lake": { "color": "#6d94c2", "detail": 1 },
		// land: height, the target summit height in meters; erodibility, a
		// factor (default 1)
		"plains": { "color": "#87a851", "height": 400, "detail": 1 },
		"hills": { "color": "#d1b886", "height": 1500, "detail": 2 },
		"mountains": { "color": "#65481f", "height": 4500 }
	},

	// Optional, these are the defaults
	"simulation": {
		"upliftBlur": 30,             // km: uplift ramps up over this distance from lower terrains
		"erodibility": 2e-6,          // per year: how fast rivers erode. Lower gives higher relief
		"streamExponent": 0.5,        // how erosion grows with the drainage area
		"criticalSlope": 30,          // degrees: steeper hillslopes collapse
		"timeStep": 50,               // thousands of years
		"steps": 300,                 // time steps on the coarse mesh
		"refineSteps": 60,            // time steps on each finer mesh
		"erodibilityNoise": 0.3,      // 0..1: variation of the rock hardness
		"noiseScale": 30,             // km, of that variation
		"floorSlope": 0.01            // of the sea and lake floors, meters per meter
	}
}
```

Terrains are listed in the file's order, the sea and lakes first. Pixels
of a color that matches no terrain are reported, and treated as outside of
the map.

## How it works

The map is covered by nested meshes: a coarse one, and finer ones where
terrains want detail. On the coarse mesh, the land starts flat at sea
level, and rises at a rate set per region of each terrain, ramping up from
its border with lower terrains. At each time step, water flows downhill
from vertex to vertex; the drainage area of each vertex (how much land
drains through it) sets how fast its river cuts down (the stream power law,
solved implicitly as in Braun and Willett, 2013); slopes steeper than the
critical slope collapse; depressions are filled so that every river reaches
the sea. River networks grow into the rising land from the coasts.

The uplift rates are calibrated: the summits of each region are measured
and its rate adjusted, until they reach the terrain's target height. Then
each finer mesh starts from the previous one and continues the simulation,
which carves the finer valleys. Lakes are flat, at the level of their lowest
shore, and the sea and lake floors slope down from their shores.

The simulation takes seconds: about 8 on the example map (a 2048 x 2048
image, 160 000 vertices), with a preview after 3.

## Building from source

With Go 1.26 and a C/C++ compiler (the app uses OpenGL, GLFW and Dear
ImGui):

- Windows: a 64 bits MinGW-w64 GCC on the `PATH`, such as
  [WinLibs](https://winlibs.com/).
- macOS: the Xcode command line tools (`xcode-select --install`).
- Linux: GCC and the X11, Wayland and OpenGL development packages. On Debian or
  Ubuntu: `sudo apt install build-essential libgl1-mesa-dev xorg-dev
  libwayland-dev libxkbcommon-dev`.

```sh
go build -o bin/wgen ./cmd/wgen
```

The first build takes several minutes (compiling the ImGui bindings), later
ones a few seconds. `scripts/package.sh` builds the distributed packages.
See [DEVELOPMENT.md](DEVELOPMENT.md) for more.
