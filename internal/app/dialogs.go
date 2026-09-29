package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/AllenDang/cimgui-go/imgui"

	"github.com/Castux/wgen/internal/export"
)

// Modal dialogs: a file browser (filedialog.go), new map, export, image
// import (importdialog.go), the confirmation of discarding unsaved changes,
// and the controls.

// dialogs holds the state of the dialogs. Opening one sets it up; it is
// drawn every frame while open.
type dialogs struct {
	file     fileDialog
	newMap   newMapDialog
	export   exportDialog
	imports  importDialog
	confirm  confirmDialog
	help     bool
	welcome  bool
	recovery bool
}

func (a *app) drawDialogs() {
	a.drawFileDialog()
	a.drawNewMap()
	a.drawExport()
	a.drawImport()
	a.drawConfirm()
	a.drawHelp()
	a.drawWelcome()
	a.drawRecovery()
}

// modal opens a modal popup (once, when asked) and begins it, centered.
func modal(id, title string, open *bool, width, height float32) bool {
	if *open {
		imgui.OpenPopupStr(id)
		*open = false
	}
	display := imgui.CurrentIO().DisplaySize()
	imgui.SetNextWindowPosV(imgui.NewVec2(display.X/2, display.Y/2), imgui.CondAppearing, imgui.NewVec2(0.5, 0.5))
	if width > 0 {
		imgui.SetNextWindowSizeV(imgui.NewVec2(width, height), imgui.CondAppearing)
	}
	flags := imgui.WindowFlagsNoSavedSettings
	if width == 0 {
		flags |= imgui.WindowFlagsAlwaysAutoResize
	}
	return imgui.BeginPopupModalV(title+"###"+id, nil, flags)
}

// New map

type newMapDialog struct {
	opening       bool
	width, height int32
	island        bool
	keepTerrains  bool
	mapWidth      float64
}

var mapSizes = []int32{256, 512, 1024, 2048, 4096, 8192, 16384}

func (a *app) openNewMap() {
	d := &a.dialogs.newMap
	*d = newMapDialog{opening: true, width: defaultMapSize, height: defaultMapSize, island: true, keepTerrains: false, mapWidth: defaultMapWidth}
	if c := a.editor.canvas; c != nil {
		d.width, d.height = int32(c.width), int32(c.height)
	}
	if conf := a.session.Engine.Config(); conf != nil {
		d.mapWidth = conf.MapWidth
	}
}

func (a *app) drawNewMap() {
	d := &a.dialogs.newMap
	if !modal("newmap", "New map", &d.opening, 0, 0) {
		return
	}

	sizeCombo := func(label string, value *int32) {
		imgui.SetNextItemWidth(120 * a.uiScale)
		if imgui.BeginCombo(label, fmt.Sprint(*value)) {
			for _, size := range mapSizes {
				if imgui.SelectableBoolV(fmt.Sprint(size), size == *value, imgui.SelectableFlagsNone, imgui.NewVec2(0, 0)) {
					*value = size
				}
			}
			imgui.EndCombo()
		}
	}
	sizeCombo("Width (px)", &d.width)
	imgui.SameLine()
	sizeCombo("Height (px)", &d.height)

	imgui.SetNextItemWidth(120 * a.uiScale)
	imgui.InputDoubleV("Map width (km)", &d.mapWidth, 0, 0, "%.0f", imgui.InputTextFlagsNone)
	d.mapWidth = max(1, d.mapWidth)
	imgui.TextDisabled(fmt.Sprintf("%.0f x %.0f km, %.0f m per pixel", d.mapWidth, d.mapWidth*float64(d.height)/float64(d.width),
		d.mapWidth*1000/float64(d.width)))

	imgui.Checkbox("Start with an island", &d.island)
	imgui.SetItemTooltip("Else the map is all sea")
	imgui.Checkbox("Keep the current terrains", &d.keepTerrains)
	imgui.SetItemTooltip("Else plains, hills and mountains")

	if d.width*d.height > 8192*8192 {
		imgui.TextColored(noticeColor, "Large maps take long to generate")
	}

	if imgui.Button("Create") {
		imgui.CloseCurrentPopup()
		a.newProject(int(d.width), int(d.height), d.mapWidth, d.island, d.keepTerrains)
	}
	imgui.SameLine()
	if imgui.Button("Cancel") {
		imgui.CloseCurrentPopup()
	}
	imgui.EndPopup()
}

// Export

type exportDialog struct {
	opening bool
	base    string // path without extension
}

var textureScales = []float64{0.5, 1, 2, 4}

func (a *app) openExport() {
	d := &a.dialogs.export
	d.opening = true
	if path := a.session.ProjectPath(); path != "" {
		if abs, err := filepath.Abs(path); err == nil {
			path = abs
		}
		d.base = strings.TrimSuffix(path, filepath.Ext(path))
	} else if d.base == "" {
		home, _ := os.UserHomeDir()
		d.base = filepath.Join(home, "map")
	}
}

func (a *app) drawExport() {
	d := &a.dialogs.export
	if !modal("export", "Export", &d.opening, 0, 0) {
		return
	}

	options := a.drawExportOptions()

	imgui.Separator()
	imgui.TextWrapped(noFormat("Files: " + d.base + "-height.png, ..."))
	if imgui.Button("Choose...") {
		a.openFile("Export as", filepath.Dir(d.base), filepath.Base(d.base), true, nil, func(path string) {
			d.base = strings.TrimSuffix(path, filepath.Ext(path))
			d.opening = true
		})
		imgui.CloseCurrentPopup()
	}

	imgui.Separator()
	world, _ := a.session.Engine.Snapshot()
	nothing := options == export.Options{TextureScale: options.TextureScale, Normalized: options.Normalized}
	imgui.BeginDisabledV(world == nil || a.exporting.Load() || nothing)
	if imgui.Button("Export") {
		imgui.CloseCurrentPopup()
		a.exportFiles(d.base, options)
	}
	imgui.EndDisabled()
	imgui.SameLine()
	if imgui.Button("Cancel") {
		imgui.CloseCurrentPopup()
	}
	imgui.EndPopup()
}

// drawExportOptions shows the files to export, remembered in the settings,
// and returns them.
func (a *app) drawExportOptions() export.Options {
	options := a.settings.Export
	imgui.Checkbox("Heightmap: 16 bits grayscale PNG", &options.Heightmap)
	if options.Heightmap {
		imgui.Indent()
		if imgui.RadioButtonBool("In meters above sea level", !options.Normalized) {
			options.Normalized = false
		}
		imgui.SetItemTooltip("1 unit per meter, water and below 0 at 0")
		if imgui.RadioButtonBool("Normalized", options.Normalized) {
			options.Normalized = true
		}
		imgui.SetItemTooltip("From the deepest point (0) to the highest (65535)")
		imgui.Unindent()
	}
	imgui.Checkbox("Water mask: white where there is water", &options.WaterMask)
	imgui.Checkbox("Texture: terrain colors, hillshading and rivers", &options.Texture)
	if options.Texture {
		imgui.Indent()
		imgui.SetNextItemWidth(100 * a.uiScale)
		if imgui.BeginCombo("Scale", fmt.Sprintf("x%g", options.TextureScale)) {
			for _, scale := range textureScales {
				if imgui.SelectableBoolV(fmt.Sprintf("x%g", scale), scale == options.TextureScale, imgui.SelectableFlagsNone, imgui.NewVec2(0, 0)) {
					options.TextureScale = scale
				}
			}
			imgui.EndCombo()
		}
		imgui.Unindent()
	}
	imgui.Checkbox("Mesh: OBJ, with UVs for the texture", &options.OBJ)
	imgui.Checkbox("Vector map: SVG of the terrain cells and rivers", &options.SVG)

	if options != a.settings.Export {
		settings := a.settings
		settings.Export = options
		a.setSettings(settings)
	}
	return options
}

// Confirmation of discarding unsaved changes

type confirmDialog struct {
	opening bool
	what    string // "open another map", "quit"...
	then    func()
}

// unsavedThen runs f, after asking what to do with unsaved changes, if any.
func (a *app) unsavedThen(what string, f func()) {
	if !a.unsaved() {
		f()
		return
	}
	a.dialogs.confirm = confirmDialog{opening: true, what: what, then: f}
}

func (a *app) drawConfirm() {
	d := &a.dialogs.confirm
	if !modal("confirm", "Unsaved changes", &d.opening, 0, 0) {
		return
	}
	imgui.TextUnformatted("The map has unsaved changes. Save them before you " + d.what + "?")
	if imgui.Button("Save") {
		imgui.CloseCurrentPopup()
		a.saveProject(d.then)
	}
	imgui.SameLine()
	if imgui.Button("Discard them") {
		imgui.CloseCurrentPopup()
		d.then()
	}
	imgui.SameLine()
	if imgui.Button("Cancel") {
		imgui.CloseCurrentPopup()
	}
	imgui.EndPopup()
}

// Help

var controls = [][2]string{
	{"Left drag", "3D: rotate. Map: pan. Painting: paint, in both views"},
	{"Right, middle drag", "Pan (middle: zoom in 3D). Painting in 3D: right rotates"},
	{"Wheel", "Zoom"},
	{"Double click", "Map: fit the map in the window. Measuring: finish the ruler"},
	{"V", "3D or map view"},
	{"Shift", "Terrain or height colors"},
	{"Q / W", "Lit or unlit / wireframe"},
	{"R", "Reset the view"},
	{"E", "Paint the map"},
	{"[ / ]", "Smaller / larger brush"},
	{"L", "Lock the shoreline"},
	{"M", "Measure with rulers: click to add points"},
	{"Enter / Escape", "Finish the ruler / deselect it"},
	{"Delete", "Delete the selected ruler"},
	{"Ctrl+Z / Ctrl+Y", "Undo / redo painting"},
	{"Ctrl+N / O / S", "New map / open / save"},
	{"Ctrl+Shift+S", "Save as"},
	{"Ctrl+E", "Export"},
}

func (a *app) drawHelp() {
	if !modal("help", "Controls", &a.dialogs.help, 0, 0) {
		return
	}
	if imgui.BeginTableV("controls", 2, imgui.TableFlagsRowBg|imgui.TableFlagsBordersInnerV, imgui.NewVec2(0, 0), 0) {
		for _, control := range controls {
			imgui.TableNextRow()
			imgui.TableNextColumn()
			imgui.TextUnformatted(control[0])
			imgui.TableNextColumn()
			imgui.TextUnformatted(control[1])
		}
		imgui.EndTable()
	}
	imgui.Spacing()
	imgui.TextWrapped("Paint where the sea, lakes and terrains are: each land terrain has a target height for its summits. The landscape is simulated: the land rises, rivers erode it, and valleys and ridges form.")
	if imgui.Button("Close") {
		imgui.CloseCurrentPopup()
	}
	imgui.EndPopup()
}

// noFormat escapes a text for ImGui functions taking a printf format.
func noFormat(s string) string { return strings.ReplaceAll(s, "%", "%%") }
