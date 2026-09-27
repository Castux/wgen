# wgen development

How wgen is built and organized. See the [README](README.md) for what it
does and how to use it.

## Building

Requirements: Go 1.26, cgo, and a C/C++ compiler (see the README for each
system). The viewer links against C and C++ libraries:

| Module | What | Notes |
|---|---|---|
| `github.com/go-gl/glfw/v3.4/glfw` | window, input, OpenGL context | Pre-release, because ImGui's GLFW backend needs GLFW 3.4 (`glfwGetPlatform`). Compiles GLFW from source. |
| `github.com/go-gl/gl/v3.3-core/gl` | OpenGL 3.3 core bindings | |
| `github.com/AllenDang/cimgui-go` | Dear ImGui, with its GLFW and OpenGL 3 backends | Pinned to v1.5.0: the v1.6.0 module lacks the prebuilt Windows library. Ships prebuilt static libraries for Windows x64, Linux x64 and macOS; cgo still compiles its C++ wrappers, which is why the first build takes minutes. |
| `github.com/go-gl/mathgl` | vector and matrix math | Pure Go. |

The window is created by go-gl/glfw, and ImGui's own GLFW and OpenGL 3
backends (`cimgui-go/impl/glfw`, `impl/opengl3`) are plugged into it. Don't
use cimgui-go's `backend/glfwbackend`: it links its own copy of GLFW, which
would clash with go-gl's.

Cross compiling needs a C cross compiler for the target: it's simpler to
build on each system.

Windows notes:

- Use a 64 bits MinGW-w64 GCC. Check it works with a trivial cgo program
  before suspecting wgen.
- A non-ASCII user name (so `GOPATH`, `GOCACHE` and the Go toolchain under
  `C:\Users\<name>`) is fine with a recent MinGW-w64.
- If cgo builds fail without a message from Git Bash (`cgo.exe: exit status
  2`), try building from PowerShell or `cmd`: this happened with a gcc whose
  `cc1.exe` couldn't run from Git Bash but was fine from Windows shells.

Everything but `internal/viewer` and `cmd/wgen` is pure Go, and can be built
and tested without a C compiler.

## Code layout

| Path | |
|---|---|
| `cmd/wgen` | command line: export, or open the viewer |
| `cmd/wgenlab` | experiments: compare generation variants (see below) |
| `internal/config` | config loading, validation, patching, saving; parameter schema for the panel |
| `internal/mesh` | Delaunay triangulation and dual graph |
| `internal/gen` | the generation pipeline, in stages; immutable `World` snapshots |
| `internal/export` | OBJ, SVG, PNG |
| `internal/render` | CPU drawn images: base colors, hillshade, rivers, contours, grid |
| `internal/engine` | background generation, file watching, the session tying them to a config file |
| `internal/viewer` | the viewer window |
| `test` | sample outline images and config |
| `etc` | source artwork and larger sample maps |
| `lab` | experiments for `cmd/wgenlab` |

## Generation

`gen.World` is an immutable snapshot of a generated world: the outline image,
the mesh, per vertex data (terrain, gradient, elevation, water level, flow,
downhill neighbor), and the rasterized heightmap and water map.

The pipeline is split in stages: image, mesh, terrain (terrain types,
elevation, rivers), erosion (erosion, water depth), raster (rasterize, blur).
`World.Update` reruns the pipeline from the first stage a config change
affects (`gen.ChangedStage`), and returns a new world sharing the unchanged
data with the old one. `World.ReloadImage` does the same for a new outline
image: from the terrain stage if the image size is unchanged, from the mesh
stage otherwise.

The random generator is seeded by the config's `seed`, with a separate stream
per use, so that runs are reproducible and a stage's randomness doesn't
depend on earlier stages.

### Uplift model

`internal/gen/uplift.go`, selected by `elevationModel`. It replaces the
elevation, river and erosion steps; the mesh stage builds its meshes, and
the terrain stage runs the simulation. `changedStageUplift` decides what
reruns: the mesh depends on the terrains' details, the simulation on
everything else.

- Meshes: hex lattices, each level at half the spacing of the previous one,
  and containing its points (unjittered, lattice point (i, j) of a level is
  (2i + j%2, 2j) on the next). A point is added at the first level where it
  is on the lattice, if the terrain around wants that much detail, and
  jittered once, so that it keeps its position at every level.
- Per level, `simState` holds the elevation (meters), uplift, erodibility and
  Voronoi cell areas (`CellAreas`) of each vertex. Sea vertices are the fixed
  base level; lakes don't rise and erode fast.
- Each time step (`simState.run`): steepest descent receivers, an ordering
  from the base level up, drainage areas accumulated in reverse, then the
  implicit stream power update of Braun and Willett (2013), for n = 1: in
  order, h = (h + U dt + F h_receiver) / (1 + F), F = K dt A^m / distance.
  Then slopes steeper than the critical slope collapse. Every 10 steps,
  depressions are filled (priority flood with a small slope), so that every
  vertex drains to the sea.
- The first level starts nearly flat and runs `steps`; each next level
  starts from the previous one (common vertices keep their elevation, new
  ones get the mean of their neighbours plus a little noise) and runs
  `refineSteps`.
- Elevations are in meters, positions in pixels: `World.MetersPerPixel`
  converts (1 with the slope model). The hillshading of `render`, the OBJ
  export and the viewer (a `zScale` uniform, with the vertical exaggeration)
  use it. River widths come from `World.Drainage`, the drainage area in
  square pixels, since the mesh isn't uniform.

`lab/uplift.json` and `lab/uplift-exp.json` are the experiments on it. On
the Chasers map at 1000 km wide: 5 s for 160 000 vertices (3 levels), 13 s
for 430 000 (4 levels); peaks up to 9.5 km, Hack exponent 0.56.

## Engine and session

`engine.Engine` owns the current world, and regenerates it in a goroutine.
Requests are coalesced: while a generation runs, only the latest requested
config is kept, and generated next. Readers take the current snapshot
(`Snapshot`), and can subscribe to state changes (`Subscribe`): version
(incremented by each generation that changed the world), busy, error, dirty
(the config differs from its file).

`engine.Session` ties an engine to a config file: it loads it, watches it and
the outline image (`Watcher`: directories are watched, so that editors
replacing files on save are handled, and events are debounced), and applies
edits (`Patch`, a partial config in the file format), `Save` and `Export`.

## Viewer

`viewer.Run` owns the main thread: GLFW and OpenGL must be used from it,
which is why the package locks the main goroutine to its thread in `init`.

The main loop (`viewer.go`) is event driven: it draws a few frames after any
input, then sleeps in `glfw.WaitEventsTimeout`. Background work (the engine,
image rendering, exports) wakes it with `glfw.PostEmptyEvent`. Each frame:

1. `update`: if the engine has a new world version, upload its mesh; request
   the images the current settings need; upload rendered images.
2. `handleInput`: keys and mouse, unless ImGui wants them. A drag that
   started outside of the panel belongs to the view until released.
   Shortcuts are collected from the GLFW key callback (`onKey`), and skipped
   while ImGui has an active widget or an open popup. They avoid ImGui's own
   keys: modified keys are not shortcuts (`Ctrl+Tab` is ImGui's window
   switching, enabled even without keyboard navigation), and Shift only is
   when tapped alone (ImGui scrolls horizontally with Shift+wheel).
3. The panel and the status, then the scene, then ImGui on top.

Files:

- `terrain.go`: the 3D views. One vertex buffer (position, terrain color),
  32 bits indices. Flat normals come from screen space derivatives of the
  view position; lighting and colors reproduce the three.js materials of the
  former web viewer (Lambert with an ambient light of intensity 1 and a
  directional light of intensity 3, in linear colors, sRGB output). The
  margin around the map is discarded in the fragment shader.
- `mapview.go`: the 2D view, a textured quad.
- `camera.go`: orbit, top and map cameras. Pure math, unit tested; speeds
  are those of three.js `OrbitControls`.
- `images.go`: `imageSlot` renders images with `internal/render` in a
  goroutine, one at a time. Only the latest request matters: requests
  arriving while rendering replace each other, stale results are dropped.
  There are two slots: the overlay texture of the 3D views (rivers, contours,
  grid), and the 2D map image (only while the map is shown, at a power of
  two scale following the zoom).
- `panel.go`: the ImGui panel. Generation parameters come from
  `config.Schema`, their values from `config.Value`. Numbers are a slider
  (rounded to the schema step) and a field (as typed), which commit when
  done, not on every change: edits are sent to the session as partial
  configs, unless the value is unchanged. The panel is kept inside the
  window, as its saved position may be off screen in a smaller window.
- `settings.go`: viewer settings, saved as JSON in the user config directory
  (`os.UserConfigDir()/wgen/viewer.json`, with ImGui's `imgui.ini` for the
  panel layout next to it).
- `gl.go`: shader and texture helpers.

### Checking the viewer without looking at it

With `WGEN_SCREENSHOT=shot.png` in the environment, the viewer saves a
screenshot once the world and its images are ready, and quits. Combined with
editing `viewer.json`, it checks every view and mode from a script.
`WGEN_CAMERA=x,y,distance,tilt,turn` places the orbit camera for close
ups: target in image pixels (top left origin), distance in pixels, tilt from
vertical and turn in degrees.

```sh
WGEN_SCREENSHOT=shot.png WGEN_CAMERA=200,1600,150,60,20 bin/wgen --interactive lab/uplift.json
```

## Experiments

`cmd/wgenlab` compares variants of the generation, to judge algorithm
changes on more than a glance at the viewer. An experiment file names a base
config, cases and variants (partial configs, all combinations are run), a
terrain to measure, and image regions to render at full scale:

```sh
go run ./cmd/wgenlab -out lab/out2 lab/erosion2.json
```

It writes renders (elevation, hillshading, rivers, contours) of the whole map
and of the regions, and `report.md`: per variant, generation time, and
measures on the chosen terrain: peaks, drainage density, bifurcation ratio of
the river network (3 to 5 in nature) and Hack exponent (length of rivers
against drainage area, about 0.6 in nature, 1 for the parallel drainage of a
single crest). `lab/` holds the experiments on the erosion models; outputs go
to `lab/out*`, which is not versioned.

## Tests

```sh
go test ./...
go test ./internal/gen -run Golden -update   # after an intended change of results
```

- `internal/gen`: invariants of the results (such as rivers flowing
  downhill), reproducibility, incremental updates giving the same results as
  full runs, image reloading, and golden results.
- `internal/engine`: patching, saving, exporting, hot reload of the config and
  the image, through a session on a temporary directory.
- `internal/viewer`: cameras, settings, image slots, panel helpers. The
  package links ImGui, so it needs the C toolchain even for tests, but no
  window or GPU.

## Performance

Measured on a 16384 x 16384 image at resolution 32 (312k vertices), on the
development machine (16 GB, GeForce GTX 980 Ti):

| Step | Time |
|---|---|
| Load the image | 12 s |
| Triangulate, assign terrains | 1.5 s |
| Elevation, rivers, erosion, water depth | 0.4 s |
| Rasterize the heightmap and water map | 4.5 s |
| Overlay texture (scale 0.25) | 0.4 s |

A parameter change reruns rasterization, which dominates. The rasters are
`float64` at the image resolution: several GB of memory at this size. Ideas,
if large maps matter more:

- Rasterize only when exporting: the viewer only uses the mesh (and the
  overlay, which samples the heightmap for contours and hillshading).
- Draw contours, hillshading and the grid in shaders, and rivers as lines,
  instead of CPU images.
- Store the outline as a terrain index per pixel instead of a color.

## Known limitations

- On Windows, the window content isn't redrawn while it is being resized:
  GLFW blocks in the system's resize loop. Drawing from the refresh callback
  would fix it.
- The overlay and map images are limited to 8192 pixels (`render.MaxSize`),
  the map image to 4096.
