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

## Usage

Requires Go 1.26.

```sh
go build -o bin/wgen ./cmd/wgen

bin/wgen test/config.json                  # generate and export
bin/wgen --interactive test/config.json    # web viewer on http://localhost:8080
```

Flags:

- `--interactive`: serve the web viewer instead of exporting.
- `-addr :8080`: viewer address (all interfaces by default; use
  `-addr localhost:8080` to keep it local).
- `-static web/static`: serve the viewer files from disk instead of the ones
  embedded in the binary, to edit them without rebuilding.
- `-v`: log the duration of each generation stage.

Exports are written next to the config: `<config>.obj`, `<config>.svg`,
`<config>.png` (elevation) and `<config>-w.png` (water level).

### Viewer

- Views: 3D orbit, 3D top, and 2D map (`Tab` cycles). In the 2D map: drag to
  pan, wheel to zoom, double click to fit.
- `Shift`: terrain or height colors. `q`: lit or unlit. `w`: wireframe.
- Rivers, contour lines and the grid are rendered by the server, and used as a
  texture in 3D.
- Every generation parameter can be edited in the panel: only the affected
  stages are rerun. "Save config" writes the parameters back to the config
  file (reformatted, and without unknown keys). "Export files" writes the
  exports enabled in the config.
- The config file and the outline image are watched: edit them in any editor
  and the viewer updates. If the config file changes, it replaces any unsaved
  edits made in the panel. A broken config keeps the last good result and
  shows the error.

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

## Code layout

| Path | |
|---|---|
| `cmd/wgen` | command line |
| `internal/config` | config loading, validation, patching, saving; parameter schema for the UI |
| `internal/mesh` | Delaunay triangulation and dual graph |
| `internal/gen` | the generation pipeline, in stages; immutable `World` snapshots |
| `internal/export` | OBJ, SVG, PNG |
| `internal/render` | server-side images: base colors, hillshade, rivers, contours, grid |
| `internal/server` | HTTP API, background regeneration, file watching |
| `web` | the viewer (plain ES modules, three.js and lil-gui from a CDN) |

A config change reruns the pipeline from the first stage it affects
(`gen.ChangedStage`): image, mesh, terrain (terrain types, elevation, rivers),
erosion (erosion, water depth), raster (rasterize, blur).

## Tests

```sh
go test ./...
go test ./internal/gen -run Golden -update   # after an intended change of results
```

`internal/gen/dref_test.go` compares against the original D implementation
(still on the `web` and `main` branches), given reference outputs: see the
comment at its top.
