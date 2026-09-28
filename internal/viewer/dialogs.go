package viewer

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/AllenDang/cimgui-go/imgui"

	"github.com/Castux/wgen/internal/config"
	"github.com/Castux/wgen/internal/export"
)

// Modal dialogs: a file browser, new map, export, image import, and the
// confirmation of discarding unsaved changes.

// dialogs holds the state of the dialogs. Opening one sets it up; it is
// drawn every frame while open.
type dialogs struct {
	file    fileDialog
	newMap  newMapDialog
	export  exportDialog
	imports importDialog
	confirm confirmDialog
	help    bool
}

func (a *app) drawDialogs() {
	a.drawFileDialog()
	a.drawNewMap()
	a.drawExport()
	a.drawImport()
	a.drawConfirm()
	a.drawHelp()
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

// File browser

type fileDialog struct {
	opening bool
	title   string
	save    bool
	exts    []string // lower case, with the dot; empty: all files
	dir     string
	name    string // file name, typed or selected
	confirm bool   // save: overwriting asked
	done    func(path string)

	listed  string
	entries []os.DirEntry
	err     string
}

// openFile opens the file browser, starting in dir. With save, a file name
// is typed (name is the default), else an existing file is picked.
func (a *app) openFile(title, dir, name string, save bool, exts []string, done func(path string)) {
	if dir == "" {
		dir, _ = os.UserHomeDir()
	}
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	a.dialogs.file = fileDialog{opening: true, title: title, save: save, exts: exts, dir: dir, name: name, done: done}
}

func (d *fileDialog) list() {
	if d.listed == d.dir {
		return
	}
	d.listed, d.err = d.dir, ""
	entries, err := os.ReadDir(d.dir)
	if err != nil {
		d.err = err.Error()
	}
	d.entries = d.entries[:0]
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if e.IsDir() || len(d.exts) == 0 || slices.Contains(d.exts, ext) {
			d.entries = append(d.entries, e)
		}
	}
	slices.SortFunc(d.entries, func(a, b os.DirEntry) int {
		if a.IsDir() != b.IsDir() {
			if a.IsDir() {
				return -1
			}
			return 1
		}
		return strings.Compare(strings.ToLower(a.Name()), strings.ToLower(b.Name()))
	})
}

// places are shortcuts to common directories.
func (a *app) places() [][2]string {
	var places [][2]string
	if home, err := os.UserHomeDir(); err == nil {
		places = append(places, [2]string{"Home", home})
		for _, name := range []string{"Desktop", "Documents", "Pictures", "Downloads"} {
			if p := filepath.Join(home, name); isDir(p) {
				places = append(places, [2]string{name, p})
			}
		}
	}
	if p := a.session.ConfigPath(); p != "" {
		places = append(places, [2]string{"Project", filepath.Dir(p)})
	}
	if runtime.GOOS == "windows" {
		for c := 'A'; c <= 'Z'; c++ {
			if drive := string(c) + `:\`; isDir(drive) {
				places = append(places, [2]string{drive, drive})
			}
		}
	} else {
		places = append(places, [2]string{"/", "/"})
	}
	return places
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func (a *app) drawFileDialog() {
	d := &a.dialogs.file
	if !modal("file", d.title, &d.opening, 760*a.uiScale, 520*a.uiScale) {
		return
	}
	d.list()

	// Path, and up
	if imgui.Button("Up") {
		d.dir = filepath.Dir(d.dir)
	}
	imgui.SameLine()
	dir := d.dir
	imgui.SetNextItemWidth(-1)
	if imgui.InputTextWithHint("##dir", "folder", &dir, imgui.InputTextFlagsEnterReturnsTrue, nil) && isDir(dir) {
		d.dir = dir
	}

	footer := imgui.FrameHeightWithSpacing() * 3
	imgui.BeginChildStrV("places", imgui.NewVec2(140*a.uiScale, -footer), imgui.ChildFlagsBorders, 0)
	for _, p := range a.places() {
		if imgui.SelectableBool(p[0]) {
			d.dir = p[1]
		}
	}
	imgui.EndChild()
	imgui.SameLine()

	imgui.BeginChildStrV("files", imgui.NewVec2(0, -footer), imgui.ChildFlagsBorders, 0)
	if d.err != "" {
		imgui.TextColored(errorColor, noFormat(d.err))
	}
	for _, e := range d.entries {
		name := e.Name()
		label := name
		if e.IsDir() {
			label = "[" + name + "]"
		}
		if imgui.SelectableBoolV(label, !e.IsDir() && name == d.name, imgui.SelectableFlagsAllowDoubleClick, imgui.NewVec2(0, 0)) {
			if e.IsDir() {
				if imgui.IsMouseDoubleClicked(imgui.MouseButtonLeft) {
					d.dir = filepath.Join(d.dir, name)
				}
			} else {
				d.name = name
				d.confirm = false
				if imgui.IsMouseDoubleClicked(imgui.MouseButtonLeft) {
					a.fileChosen()
				}
			}
		}
	}
	imgui.EndChild()

	imgui.SetNextItemWidth(-1)
	if imgui.InputTextWithHint("##name", "file name", &d.name, imgui.InputTextFlagsEnterReturnsTrue, nil) {
		a.fileChosen()
	}
	d.name = strings.TrimSpace(d.name)

	label := "Open"
	if d.save {
		label = "Save"
		if d.confirm {
			label = "Replace it"
		}
	}
	imgui.BeginDisabledV(d.name == "")
	if imgui.Button(label) {
		a.fileChosen()
	}
	imgui.EndDisabled()
	imgui.SameLine()
	if imgui.Button("Cancel") {
		imgui.CloseCurrentPopup()
	}
	if d.confirm {
		imgui.SameLine()
		imgui.TextColored(dirtyColor, noFormat(d.name+" already exists"))
	}
	imgui.EndPopup()
}

// fileChosen finishes the file browser, if the name makes sense.
func (a *app) fileChosen() {
	d := &a.dialogs.file
	if d.name == "" {
		return
	}
	path := filepath.Join(d.dir, d.name)
	if filepath.IsAbs(d.name) {
		path = d.name
	}
	if isDir(path) {
		d.dir, d.name = path, ""
		return
	}
	if d.save && len(d.exts) > 0 && !slices.Contains(d.exts, strings.ToLower(filepath.Ext(path))) {
		path += d.exts[0]
	}
	_, err := os.Stat(path)
	switch {
	case !d.save && err != nil:
		return
	case d.save && err == nil && !d.confirm:
		d.confirm = true
		return
	}
	imgui.CloseCurrentPopup()
	d.done(path)
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
	*d = newMapDialog{opening: true, width: 2048, height: 2048, island: true, keepTerrains: false, mapWidth: 1000}
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

	size := func(label string, v *int32) {
		imgui.SetNextItemWidth(120 * a.uiScale)
		if imgui.BeginCombo(label, fmt.Sprint(*v)) {
			for _, s := range mapSizes {
				if imgui.SelectableBoolV(fmt.Sprint(s), s == *v, imgui.SelectableFlagsNone, imgui.NewVec2(0, 0)) {
					*v = s
				}
			}
			imgui.EndCombo()
		}
	}
	size("Width (px)", &d.width)
	imgui.SameLine()
	size("Height (px)", &d.height)

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
		imgui.TextColored(dirtyColor, "Large maps take long to generate")
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

func (a *app) openExport() {
	d := &a.dialogs.export
	d.opening = true
	if p := a.session.ConfigPath(); p != "" {
		if abs, err := filepath.Abs(p); err == nil {
			p = abs
		}
		d.base = strings.TrimSuffix(p, filepath.Ext(p))
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

	o := a.settings.Export
	imgui.Checkbox("Heightmap: 16 bits grayscale PNG", &o.Heightmap)
	if o.Heightmap {
		imgui.Indent()
		if imgui.RadioButtonBool("In meters above sea level", !o.Normalized) {
			o.Normalized = false
		}
		imgui.SetItemTooltip("1 unit per meter, water and below 0 at 0")
		if imgui.RadioButtonBool("Normalized", o.Normalized) {
			o.Normalized = true
		}
		imgui.SetItemTooltip("From the deepest point (0) to the highest (65535)")
		imgui.Unindent()
	}
	imgui.Checkbox("Water mask: white where there is water", &o.WaterMask)
	imgui.Checkbox("Texture: terrain colors, hillshading and rivers", &o.Texture)
	if o.Texture {
		imgui.Indent()
		imgui.SetNextItemWidth(100 * a.uiScale)
		if imgui.BeginCombo("Scale", fmt.Sprintf("x%g", o.TextureScale)) {
			for _, s := range []float64{0.5, 1, 2, 4} {
				if imgui.SelectableBoolV(fmt.Sprintf("x%g", s), s == o.TextureScale, imgui.SelectableFlagsNone, imgui.NewVec2(0, 0)) {
					o.TextureScale = s
				}
			}
			imgui.EndCombo()
		}
		imgui.Unindent()
	}
	imgui.Checkbox("Mesh: OBJ, with UVs for the texture", &o.OBJ)
	imgui.Checkbox("Vector map: SVG of the terrain cells and rivers", &o.SVG)
	if o != a.settings.Export {
		s := a.settings
		s.Export = o
		a.setSettings(s)
	}

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
	imgui.BeginDisabledV(world == nil || a.exporting.Load() || o == export.Options{TextureScale: o.TextureScale, Normalized: o.Normalized})
	if imgui.Button("Export") {
		imgui.CloseCurrentPopup()
		a.exportFiles(d.base, o)
	}
	imgui.EndDisabled()
	imgui.SameLine()
	if imgui.Button("Cancel") {
		imgui.CloseCurrentPopup()
	}
	imgui.EndPopup()
}

// Import

type importDialog struct {
	opening       bool
	path          string
	width, height int
	pixels        []config.Color
	colors        []colorCount
	conf          *config.Config
	picks         []pick
	result        bool // show the result instead of the image

	original, preview texture // downscaled
	previewOf         []pick
	previewResult     bool
}

// Preview images are at most this wide
const previewSize = 512

func (a *app) openImport(path string, width, height int, pixels []config.Color) {
	conf := config.Default("")
	if current := a.session.Engine.Config(); current != nil {
		conf.MapWidth = current.MapWidth
	}
	d := &a.dialogs.imports
	d.original.delete()
	d.preview.delete()
	*d = importDialog{opening: true, path: path, width: width, height: height, pixels: pixels,
		colors: palette(pixels), conf: conf, picks: defaultPicks(width, height, pixels, conf)}
	d.original.uploadPixels(previewPixels(width, height, pixels))
}

// previewPixels downscales a map to RGBA pixels of at most previewSize,
// rows from the top.
func previewPixels(width, height int, pixels []config.Color) (int, int, []byte) {
	scale := max(1, (max(width, height)+previewSize-1)/previewSize)
	pw, ph := width/scale, height/scale
	out := make([]byte, 0, 4*pw*ph)
	for row := range ph {
		y := height - 1 - row*scale
		for x := range pw {
			c := pixels[y*width+x*scale]
			out = append(out, c[0], c[1], c[2], 255)
		}
	}
	return pw, ph, out
}

func (a *app) drawImport() {
	d := &a.dialogs.imports
	if !modal("import", "Import "+filepath.Base(d.path), &d.opening, 980*a.uiScale, 640*a.uiScale) {
		return
	}

	// The image, or the result: click to pick a color
	if d.result && (d.previewResult != d.result || !slices.Equal(d.previewOf, d.picks)) {
		d.preview.uploadPixels(previewPixels(d.width, d.height, classify(d.pixels, d.picks, d.conf)))
		d.previewOf, d.previewResult = slices.Clone(d.picks), d.result
	}
	tex := &d.original
	if d.result {
		tex = &d.preview
	}
	side := 520 * a.uiScale
	scale := side / float32(max(tex.width, tex.height))
	size := imgui.NewVec2(float32(tex.width)*scale, float32(tex.height)*scale)
	origin := imgui.CursorScreenPos()
	imgui.Image(*imgui.NewTextureRefTextureID(imgui.TextureID(tex.id)), size)
	if imgui.IsItemHovered() {
		m := imgui.CurrentIO().MousePos()
		x := int(float32(d.width) * (m.X - origin.X) / size.X)
		row := int(float32(d.height) * (m.Y - origin.Y) / size.Y)
		if x >= 0 && x < d.width && row >= 0 && row < d.height {
			c := d.pixels[(d.height-1-row)*d.width+x]
			imgui.SetTooltip(c.String() + "\nclick to pick this color")
			if imgui.IsMouseClickedBool(imgui.MouseButtonLeft) && !slices.ContainsFunc(d.picks, func(p pick) bool { return p.color == c }) {
				d.picks = append(d.picks, pick{c, d.conf.Land()[0].Name})
			}
		}
	}

	imgui.SameLine()
	imgui.BeginGroup()
	imgui.TextWrapped("Pick the colors of the image, and choose their terrains: every pixel takes the terrain of the closest picked color, which also cleans up antialiased edges.")
	imgui.Checkbox("Show the result", &d.result)

	imgui.SeparatorText("Picked colors")
	remove := -1
	for i := range d.picks {
		p := &d.picks[i]
		imgui.PushIDInt(int32(i))
		imgui.ColorButtonV("##c", colorVec(p.color), imgui.ColorEditFlagsNoTooltip, imgui.NewVec2(0, 0))
		imgui.SameLine()
		imgui.TextUnformatted(p.color.String() + " is")
		imgui.SameLine()
		imgui.SetNextItemWidth(160 * a.uiScale)
		if imgui.BeginCombo("##terrain", p.terrain) {
			for _, t := range d.conf.Terrains {
				if imgui.SelectableBoolV(t.Name, t.Name == p.terrain, imgui.SelectableFlagsNone, imgui.NewVec2(0, 0)) {
					p.terrain = t.Name
				}
			}
			imgui.EndCombo()
		}
		imgui.SameLine()
		if imgui.SmallButton("Remove") {
			remove = i
		}
		imgui.PopID()
	}
	if remove >= 0 {
		d.picks = slices.Delete(d.picks, remove, remove+1)
	}

	imgui.SeparatorText("Colors of the image")
	imgui.TextDisabled("Click one to pick it")
	for i, c := range d.colors[:min(len(d.colors), 24)] {
		if i%6 != 0 {
			imgui.SameLine()
		}
		imgui.PushIDInt(int32(1000 + i))
		if imgui.ColorButtonV("##p", colorVec(c.color), imgui.ColorEditFlagsNone, imgui.NewVec2(32*a.uiScale, 32*a.uiScale)) &&
			!slices.ContainsFunc(d.picks, func(p pick) bool { return p.color == c.color }) {
			d.picks = append(d.picks, pick{c.color, d.conf.Land()[0].Name})
		}
		imgui.SetItemTooltip(noFormat(fmt.Sprintf("%s: %.1f%%", c.color, 100*float64(c.count)/float64(len(d.pixels)))))
		imgui.PopID()
	}
	if len(d.colors) > 24 {
		imgui.TextDisabled(fmt.Sprintf("and %d more", len(d.colors)-24))
	}

	imgui.Spacing()
	hasSea := slices.ContainsFunc(d.picks, func(p pick) bool { return p.terrain == config.SeaName })
	if !hasSea {
		imgui.TextColored(dirtyColor, "Pick the color of the sea")
	}
	imgui.BeginDisabledV(!hasSea)
	if imgui.Button("Import") {
		imgui.CloseCurrentPopup()
		a.finishImport()
	}
	imgui.EndDisabled()
	imgui.SameLine()
	if imgui.Button("Cancel") {
		imgui.CloseCurrentPopup()
	}
	imgui.EndGroup()
	imgui.EndPopup()
}

// finishImport starts a new project with the imported map.
func (a *app) finishImport() {
	d := &a.dialogs.imports
	pixels := classify(d.pixels, d.picks, d.conf)
	base := strings.TrimSuffix(d.path, filepath.Ext(d.path))
	a.startProject(d.conf, newCanvas(d.width, d.height, pixels), base+".json")
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
	{"Left drag", "3D: rotate. Map: pan, or paint when painting"},
	{"Right, middle drag", "Pan (middle: zoom in 3D)"},
	{"Wheel", "Zoom"},
	{"Double click", "Map: fit the map in the window"},
	{"V", "3D or map view"},
	{"Shift", "Terrain or height colors"},
	{"Q / W", "Lit or unlit / wireframe"},
	{"R", "Reset the view"},
	{"E", "Paint the map"},
	{"[ / ]", "Smaller / larger brush"},
	{"L", "Lock the shoreline"},
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
		for _, c := range controls {
			imgui.TableNextRow()
			imgui.TableNextColumn()
			imgui.TextUnformatted(c[0])
			imgui.TableNextColumn()
			imgui.TextUnformatted(c[1])
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
