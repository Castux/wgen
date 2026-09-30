# wgen: UX review and proposals

A review of wgen as a product, as of the app version (projects, painting,
import, terrains, export, release builds): who it is for, what they come to
do, how the app serves each journey today, where it gets in the way, and
what to build next. Proposals are ranked at the end by value and effort.

Method: walking through each journey in the app and its code, from a fresh
start to the files a user takes away. Nothing here was tested with real
users: the first recommendation is to watch a few people use it (see the
end).

## Who wgen is for

wgen's promise: paint where things are, get believable relief, rivers and
valleys, without sculpting. Its strength is the simulation: landscapes that
look eroded because they are, at realistic heights and scales. Its likely
users:

| Who | Wants | Takes away |
|---|---|---|
| Worldbuilders: writers, game masters, hobbyist cartographers | A believable continent for their world, rivers that make sense, a pretty map | A texture or hillshaded image, often repainted in another tool; rivers as vectors |
| Game developers and technical artists | Terrain for Unity, Unreal, Godot, or their own engine, from a design sketch | 16 bits heightmaps at engine sizes, splat/mask maps, a mesh |
| 3D artists (Blender, Houdini) | A large scale base for a render or a matte painting | Mesh or heightmap with a texture, displacement |
| Board and strategy game designers | Relief under an existing political or game map (as the Diplomacy map in `etc/`) | Stylized map images at print resolution |
| The curious: students, geomorphology enthusiasts | See how mountains and rivers form | Watching the simulation, sharing it |

The first three are the core: they have a map (or an idea of one) and need
relief. The design choices so far fit them: height classes rather than
sculpting, painting in the app, realistic scales.

## Journeys

### 1. First launch

Today: the app opens on a random island, generating ("Generating...",
then "Refining...") under a dense panel: terrains, brush, map, simulation,
display. Nothing says what to do next. The Help menu has the controls.

Friction:

- The core loop (paint, see the relief change) isn't suggested: painting is
  a checkbox in the panel, `E`, or Edit, Paint the map. Many will orbit the
  island, open a parameter or two, and leave.
- The panel shows everything at once, including expert simulation
  parameters, at the same level as the terrains.
- The example project ships in the release, but the app doesn't know about
  it.

Proposals:

- A welcome card over the first island (dismissable, "don't show again"):
  three buttons, "Paint this island", "Import a map image", "Open the
  example", and one line on the idea (colors are terrains, each with a
  target height).
- File, Open example: the built-in example project, copied to a new
  untitled project so that it can't be overwritten.
- Collapse the Simulation section by default and label it "Advanced".
- Make painting a mode shown in a toolbar (see Journey 2), not a checkbox.

### 2. Painting a world from scratch

Today: File, New map (size, width in km, island or sea), then paint with
the terrain list as palette, three brushes, `[` `]` for size, shoreline
lock, undo. Each stroke regenerates: a coarse preview, then the detail.

What works: the natural brush makes coasts look drawn, not stamped; the
preview makes the loop feel live; locking the shoreline is the right tool for
relief passes; target heights are an intuitive control.

Friction:

- Painting only in the map view. In 3D, where the relief is judged, a
  stroke means switching views.
- Large areas are painted stroke by stroke: no fill.
- Choosing a terrain means going to the panel. No eyedropper, no keys.
- Brush size is in pixels, while the user thinks in km (the map width is
  known).
- The painted terrains and the generated landscape overlap in the map view
  at an opacity: it's hard to see the relief of what was just painted, or
  the exact edges of the terrains.
- The status is the only feedback on generation cost. At 8192 px and fine
  resolution, each stroke takes long, without warning.
- Undo covers painting (and recoloring), not terrain edits or parameter
  changes.

Proposals:

- A small toolbar in the map view: brushes (natural, round, square, fill),
  eyedropper, the current terrain (click for the palette), size in px and
  km, shoreline lock. Visible while painting only.
- Paint in 3D (done): raycast the cursor onto the terrain mesh (already on the
  CPU), and stamp at the hit point. Painting where the mountains are seen.
- Fill tool (flood fill of a region of one terrain, with the shoreline lock
  respected), eyedropper (`Alt`+click, or `I`), terrain keys `1`..`9`.
- A "show painted terrains" toggle with three states: only the landscape,
  both, only the terrains; `Tab`-like key on a free letter (`T`).
- A generation cost estimate next to the size and resolution fields (vertex
  count, rough seconds, from the last measured rate on this machine), and a
  warning before a change that would take over, say, 30 s.
- A single project history: painting, terrain edits and parameter changes,
  in one undo stack. Parameters are small patches: cheap to keep.

### 3. Importing an existing map

Today: open an image with no project next to it; the import dialog lists
its main colors, guesses the sea (border color) and land, lets the user
pick colors on the image or the palette and choose their terrains, previews
the classification, and cleans antialiased edges by nearest picked color.
Saving creates a project next to the image.

What works: the guess is right for the common two-color case, and the
nearest-color rule handles antialiasing without asking.

Friction:

- Real maps have things that aren't terrains: borders, labels, rivers
  drawn as lines, grid lines, a legend or a title box. Today they must be
  assigned a terrain, which paints them into the landscape (the Diplomacy
  map's black borders become mountain walls).
- The map width is asked at New map, not at import: an imported map gets
  the current width, likely wrong.
- Big images are imported at full size: a 16k scan makes every stroke slow,
  when 2048 would do.
- Picks aren't remembered: re-importing a corrected image means picking
  again.
- Maps with shading or textures (Inkarnate, Wonderdraft exports) have
  thousands of colors: the 24 main ones aren't enough, and nearest color
  gives speckles.

Proposals:

- An "ignore" choice for a picked color: its pixels take the terrain of
  their surroundings (the nearest non-ignored pixel: a distance transform),
  which removes lines and labels.
- Map width and a target size (downscale to the nearest power of two up to
  a limit, or keep) in the import dialog, with the resulting m per pixel.
- A cleanup option: remove specks smaller than N pixels (majority filter
  or connected component size), for textured sources.
- Save the picks in the project (`import` section: source image, picks), and
  offer "Reimport the source image" when it changes.
- A lake guess: water regions not connected to the border, if the user
  picks a water color, become lakes (the user can switch).

### 4. Shaping the look

Today: target heights per terrain, erodibility factor and detail levels per
terrain, and ten simulation parameters (uplift ramp, erodibility, area
exponent, critical slope, time step, steps...), as number fields with
tooltips.

Friction:

- The simulation parameters are a physicist's controls. A user wanting
  "sharper mountains" or "older, rounder hills" can't map that to
  "erodibility 2e-6" or "area exponent 0.5" without trial and error, and
  each trial costs seconds.
- "Detail levels (-1: auto)" is jargon, and its effect (finer valleys,
  slower) isn't shown.
- Changing the seed is typing a number; there's no "try another one".
- There's no way to compare two variants.

Proposals:

- Presets for the simulation ("Young, sharp", "Balanced", "Old, eroded",
  "Badlands") and a couple of plain controls on top that map onto the
  physical ones: ruggedness (erodibility and critical slope), river
  incision (area exponent). The physical parameters stay under Advanced.
- Detail as a choice: "Low, Medium, High, Full" with the level count
  derived, and the vertex estimate from Journey 2.
- A dice button next to the seed.
- Snapshots: keep the last few generated worlds (they are immutable
  already), and a compare slider in the map view (before/after swipe).
- Per terrain: the achieved summit height after generation, next to the
  target (the calibration measures it), so the user sees what the target
  does.

### 5. Exporting for a game engine or a 3D tool

Today: File, Export: 16 bits heightmap (meters or normalized), water mask,
texture (x0.5 to x4), OBJ with UVs, SVG. Next to the project, by base name.

Friction:

- Engines want specific sizes and formats: Unity RAW 16 bits at 2^n + 1
  (513, 1025, 2049, 4097), Unreal PNG or RAW at its recommended sizes
  (e.g. 1009, 2017, 4033), Godot EXR or PNG. Today, the map size is the
  export size.
- To import a heightmap, the user needs its height range and horizontal
  scale (Unity's terrain height, Unreal's Z scale). Meters mode clamps
  below sea level and needs the maximum; normalized needs the min and max.
  Neither is written anywhere.
- Texturing in an engine needs masks: one per terrain, and computed maps
  such as slope, flow (rivers), sediment or wetness. Only the water mask
  exists.
- OBJ of a large map is huge (every vertex of the finest mesh), and only
  OBJ.
- Exports don't say what they contain; there's no "export again with the
  same settings" after a change.

Proposals:

- An export "target" choice: Generic, Unity, Unreal, Godot, Blender, which
  sets size, format (PNG, RAW, EXR), and resampling; the heightmap is
  resampled from the mesh at the target size (bilinear on the raster, or
  rasterized directly at that size).
- A metadata sidecar (`<name>.json`): width and height in km, m per pixel,
  min and max elevation, the heightmap encoding (meters, or the normalized
  range), and the engine settings to enter ("Unity terrain height: 5120 m").
  Shown in the dialog too.
- Mask exports: one grayscale mask per terrain (splatmaps, also packed as
  RGBA 4 at a time), slope, river flow (log drainage), and a normal map.
- glTF (GLB) with the texture embedded, and a simplified mesh option
  (target triangle count), or tiles for large maps.
- Remember the last export per project (paths and options in the project),
  and File, Export again (`Ctrl+Shift+E`).

### 6. Making a map to show: cartography and print

Today: the texture export (terrain colors, hillshading, rivers) and the SVG
(cells and rivers) are the closest to a presentable map.

Friction:

- The texture uses the painted terrain colors, which are an editing
  palette, not a cartographic style.
- The rivers in the SVG follow the mesh edges: zigzags at close range.
- No hillshade-only, contours or coastline exports, which cartographers
  compose in their own tools.

Proposals:

- Map styles for the texture export and the map view: hypsometric tints
  (the rainbow or classic atlas green-brown-white, per elevation), parchment,
  grayscale relief; with hillshade strength, contour interval and river
  color. Exported at print resolution (e.g., 300 dpi for a chosen paper
  size).
- Separate layers for compositing: hillshade, coastline (SVG path), rivers
  (SVG smoothed polylines with widths from drainage), contours (SVG).
- A scale bar and a north arrow as optional overlays in exports.

### 7. Coming back, sharing, collaborating

Today: a project is a JSON and a PNG next to it; the app reopens the last
one, the title shows unsaved changes, the app asks before losing them. The
project and image are watched, so edits in another painter are followed.

Friction:

- Only the last project is remembered: no recent files.
- No autosave: a crash loses the painting since the last save.
- Two files to share, easy to separate.
- The project doesn't remember the view (camera, colors), so reopening shows
  the default.

Proposals:

- File, Open recent (last 10), and the welcome card lists them.
- Autosave to the user config directory every minute when there are
  changes, and offer recovery at the next start.
- An optional single file format (`.wgen`: a zip of the JSON, the PNG, and
  a thumbnail), with the loose files still supported.
- Save the view with the project (per project settings: view, colors,
  camera).

### 8. Watching the world form

Today: Watch the simulation (panel or menu) shows the landscape every few
steps with the phase in the status; Replay the simulation runs it again.

It's the most striking part of wgen, and it's hidden in a checkbox.

Proposals:

- A timeline: record the watched frames (heights only, per level), then
  scrub, pause and play at a speed. The recording is small at the coarse
  level.
- Export the timeline as an animated GIF or MP4 (from the map or the 3D
  view), for sharing: this is how a tool like this spreads.
- Show time in years ("2.5 million years") rather than steps.

### 9. Large maps

Today: up to 16384 px. Loading and generating take tens of seconds, each
change seconds, and gigabytes of memory.

Proposals:

- A draft mode toggle (coarser resolution while painting, full resolution
  on demand and for export), shown in the status ("Draft").
- A progress bar with an estimate, instead of text, for long generations.
- The rasters in `float32` and generated only for export and the views that
  need them (see the performance notes in DEVELOPMENT.md).

## Cross-cutting issues

- Information on hover (done): hovering the map or the terrain shows the
  position in km, the terrain, the elevation or water depth, and the
  drainage (river size).
- Measuring (done): rulers, with their length, and the altitude profile of
  the selected one ("how far is the capital from the pass, and how high?").
  Next: saving rulers with the project, and exporting a profile (CSV, SVG).
- Feedback and errors: errors and results appear in the status text, bottom
  left, and are easy to miss ("no coast: the map needs some sea next to
  land"). Short toasts near the top, and errors that point at the fix
  (select the sea terrain, highlight the field) would help.
- Settings mixed with the project: the panel shows app settings (Display,
  Brush) and project data (Terrains, Map, Simulation) together. Grouping
  them (a Project tab and a View tab, or the Display settings in the View
  menu only) makes it clear what is saved with the project.
- Terminology: "outline", "detail levels", "erodibility", "uplift ramp" come
  from the implementation. A pass on labels and tooltips, written for the
  personas above, is cheap.
- Accessibility: the rainbow scale is not colorblind friendly for all
  (Turbo is better than Jet, but still hard for deuteranopes). Offer
  Viridis or Cividis too; a UI scale setting (fonts), independent of the
  monitor's.
- The Shift tap toggling colors is surprising, since Shift is also held for
  panning and scrolling. It works, but a plain letter (`C`) would be more
  discoverable, with Shift kept as an alias.
- Native file dialogs: the ImGui browser works everywhere, but lacks
  favorites, search, thumbnails, and the platform's conventions. A native
  dialog (via a small library such as `sqweek/dialog` or `ncruces/zenity`)
  would feel more like a real app, keeping the ImGui one as a fallback.
- macOS distribution: without notarization, first launch needs a detour
  through System Settings, which loses many users. An Apple Developer ID and
  a notarization step in CI fix it. Opening documents from the Finder (and
  a `.wgen` document type) needs handling Apple events in a small Objective
  C file.
- Windows: SmartScreen warns about unsigned downloads; code signing helps
  once there's an audience.

## New features beyond the journeys

In rough order of how much they'd change what wgen can make:

1. Rivers and lakes as design inputs: paint a river's course (a line
   tool) and have the simulation keep it (low erodibility resistance along
   it, or a fixed channel), mark lakes by clicking a basin. Worldbuilders
   often start from where rivers must be.
2. Climate and biomes: from elevation, latitude (the map's position on the
   globe), prevailing winds and distance to the sea, derive temperature and
   rainfall (rain shadows behind ranges), then biomes (Whittaker diagram),
   for the texture and for masks. Makes the texture a real map.
3. Coastline detail: painted coasts are smooth or pixelated at scale; an
   optional fractal perturbation of the shoreline (keeping the painted shape
   at large scale) adds fjords and bays where the terrain is mountainous.
4. Varied rock: paint erodibility (hard and soft rock) as a separate layer,
   for mesas, escarpments and gorges; strata for cliffs.
5. Glaciers: a glacial erosion pass above a snow line, for U-shaped valleys
   and fjords, where the target heights and latitude call for it.
6. Tectonics lite: fault lines as a paint layer (uplift gradients along a
   line), for ranges with a clear direction.
7. A world view: several maps as regions of one world, or the map wrapped
   on a globe for continents.

## Prioritized proposals

Value: how much it helps the core personas. Effort: rough size in this
codebase (S: hours, M: a few days, L: weeks).

| # | Proposal | Journey | Value | Effort |
|---|---|---|---|---|
| 1 | Hover readout: position, elevation, terrain, drainage (done, with rulers and profiles) | cross | High | S |
| 2 | Welcome card, Open example, Open recent (done) | 1, 7 | High | S |
| 3 | Map width and target size in the import dialog | 3 | High | S |
| 4 | Export metadata sidecar, height range in the dialog | 5 | High | S |
| 5 | Terrain keys `1`..`9`, eyedropper, fill tool | 2 | High | S |
| 6 | Seed dice, simulation section as Advanced, label pass | 4, cross | Medium | S |
| 7 | "Ignore" color at import (fill from surroundings), speck cleanup | 3 | High | M |
| 8 | Engine export targets (sizes, RAW, EXR), terrain masks, normal map | 5 | High | M |
| 9 | Autosave and recovery (done) | 7 | High | M |
| 10 | Painting in the 3D view (done) | 2 | High | M |
| 11 | Simulation presets and plain controls (done: terrain characters with slopes and rounding, quality choices, Landscape and Advanced groups) | 4 | High | M |
| 12 | One undo history for painting, terrains and parameters | 2 | Medium | M |
| 13 | Map styles and cartographic layers (hillshade, smoothed rivers, contours) | 6 | Medium | M |
| 14 | Timeline of the simulation, GIF/MP4 export | 8 | Medium | M |
| 15 | Draft mode, cost estimates, progress bar | 2, 9 | Medium | M |
| 16 | Achieved height per terrain (done), compare snapshots (tried, removed) | 4 | Medium | M |
| 17 | Native file dialogs, macOS notarization and document opening | cross | Medium | M |
| 18 | glTF export, simplified or tiled meshes | 5 | Medium | M |
| 19 | River and lake design inputs | new | High | L |
| 20 | Climate and biomes | new | High | L |
| 21 | Coastline detail, rock layers, glaciers, faults | new | Medium | L |
| 22 | Single file projects (`.wgen`) | 7 | Low | M |

A suggested order: the S items first (1 to 6), as one polish release; then
import and export (7, 8), which decide whether people can bring their maps
in and get them out; then the painting loop (9 to 12). The large features
(19, 20) are what would set wgen apart from heightmap tools, once the basics
are smooth.

## Validating

Before the bigger items: five to eight sessions with people from the core
personas, each bringing a map (or an idea), with a task ("make the terrain
for your world and get it into your tool"), watching without helping. The
journeys above are hypotheses: import and export are where they will most
likely prove wrong, since they depend on the tools on either side.

Simple anonymous usage counters (opt-in: which exports, map sizes, how long
generations take) would tell which of these matter at scale, if wgen gets
an audience.
