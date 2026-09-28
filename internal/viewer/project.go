package viewer

import (
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strings"

	"github.com/Castux/wgen/internal/config"
	"github.com/Castux/wgen/internal/export"
	"github.com/Castux/wgen/internal/gen"
)

// Projects: new, open, save, export. A project is its file (the config) and
// its map image, next to it; a new project has no file until saved as.

// Files the app opens
var (
	projectExts = []string{".json"}
	imageExts   = []string{".png", ".jpg", ".jpeg"}
	openExts    = append(append([]string{}, projectExts...), imageExts...)
)

// startup opens what the app was given, else the last project, else a new
// one.
func (a *app) startup(path string) {
	switch {
	case path != "" && strings.ToLower(filepath.Ext(path)) != ".json" && !isFile(strings.TrimSuffix(path, filepath.Ext(path))+".json"):
		// An image to import: over a new map, if canceled
		a.newProject(2048, 2048, 1000, true, false)
		a.open(path)
	case path != "":
		a.open(path)
	case a.settings.LastProject != "" && isFile(a.settings.LastProject):
		a.open(a.settings.LastProject)
	default:
		a.newProject(2048, 2048, 1000, true, false)
	}

	// Development: a dialog opened, for screenshots (see WGEN_SCREENSHOT)
	switch os.Getenv("WGEN_DIALOG") {
	case "new":
		a.openNewMap()
	case "open":
		a.openFile("Open a project or an image", "", "", false, openExts, a.open)
	case "export":
		a.openExport()
	case "help":
		a.dialogs.help = true
	}
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// unsaved tells whether the project has changes not saved.
func (a *app) unsaved() bool {
	return a.editor.dirty || a.session.Engine.State().Dirty
}

// projectName is the name of the project, for the window title.
func (a *app) projectName() string {
	if p := a.session.ConfigPath(); p != "" {
		return strings.TrimSuffix(filepath.Base(p), filepath.Ext(p))
	}
	return "Untitled"
}

// newProject starts a new map of the given size, all sea or with an island,
// with the default terrains or the current ones.
func (a *app) newProject(width, height int, mapWidth float64, island, keepTerrains bool) {
	conf := config.Default("")
	if current := a.session.Engine.Config(); keepTerrains && current != nil {
		conf.Terrains = current.Clone().Terrains
		conf.Simulation = current.Simulation
	}
	conf.MapWidth = mapWidth

	pixels := make([]config.Color, width*height)
	sea := conf.Terrain(config.SeaName).Color
	for i := range pixels {
		pixels[i] = sea
	}
	c := newCanvas(width, height, pixels)
	if island {
		var land []config.Color
		for _, t := range conf.Land() {
			land = append(land, t.Color)
		}
		c.island(land, uint64(a.start.UnixNano()))
	}
	a.startProject(conf, c, "")
}

// startProject switches to a new project: a config and its map, not saved
// yet. suggested is where to save it, by default.
func (a *app) startProject(conf *config.Config, c *canvas, suggested string) {
	a.setCanvas(c, true)
	a.session.New(conf, a.canvasOutline())
	a.editor.sent = nil
	a.suggestedPath = suggested
	a.version = -1
	a.fitMap = true
	a.message = nil
	a.paramsConf = nil
}

// open opens a project file, or an image: with its project if there is one
// next to it (same name, .json), else to import it.
func (a *app) open(path string) {
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".json" {
		if project := strings.TrimSuffix(path, filepath.Ext(path)) + ".json"; isFile(project) {
			a.openProject(project)
			return
		}
		a.importImage(path)
		return
	}
	a.openProject(path)
}

func (a *app) openProject(path string) {
	if err := a.session.Load(path); err != nil {
		a.message = &message{text: err.Error(), error: true}
		return
	}
	a.editor.canvas = nil
	a.editor.dirty = false
	a.editor.sent = nil
	a.version = -1
	a.fitMap = true
	a.message = nil
	a.paramsConf = nil
	a.suggestedPath = ""
	a.rememberProject(path)
}

// importImage reads an image, to import it as a map.
func (a *app) importImage(path string) {
	f, err := os.Open(path)
	if err != nil {
		a.message = &message{text: err.Error(), error: true}
		return
	}
	img, _, err := image.Decode(f)
	f.Close()
	if err != nil {
		a.message = &message{text: fmt.Sprintf("could not read %s: %v", filepath.Base(path), err), error: true}
		return
	}
	width, height, pixels := gen.Outline(img)
	a.openImport(path, width, height, pixels)
}

func (a *app) rememberProject(path string) {
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	s := a.settings
	s.LastProject = path
	a.setSettings(s)
}

// saveProject saves the project, then does then (if not nil). A new
// project is saved as.
func (a *app) saveProject(then func()) {
	if a.session.ConfigPath() == "" {
		a.saveProjectAs(then)
		return
	}
	path, err := a.session.Save(a.canvasOutline())
	if err != nil {
		a.message = &message{text: err.Error(), error: true}
		return
	}
	a.saved(path)
	if then != nil {
		then()
	}
}

// saveProjectAs asks where to save the project, and saves it there.
func (a *app) saveProjectAs(then func()) {
	dir, name := "", "map.json"
	switch p := a.session.ConfigPath(); {
	case p != "":
		dir, name = filepath.Dir(p), filepath.Base(p)
	case a.suggestedPath != "":
		dir, name = filepath.Dir(a.suggestedPath), filepath.Base(a.suggestedPath)
	}
	a.openFile("Save the project as", dir, name, true, projectExts, func(path string) {
		if err := a.session.SaveAs(path, a.canvasOutline()); err != nil {
			a.message = &message{text: err.Error(), error: true}
			return
		}
		a.saved(a.session.ConfigPath())
		if then != nil {
			then()
		}
	})
}

func (a *app) saved(path string) {
	a.editor.dirty = false
	a.suggestedPath = ""
	a.rememberProject(path)
	a.message = &message{text: "Saved " + path}
}

// openDialog asks for a project or an image to open.
func (a *app) openDialog() {
	a.unsavedThen("open another map", func() {
		dir := ""
		if p := a.session.ConfigPath(); p != "" {
			dir = filepath.Dir(p)
		} else if p := a.settings.LastProject; p != "" {
			dir = filepath.Dir(p)
		}
		a.openFile("Open a project or an image", dir, "", false, openExts, a.open)
	})
}

func (a *app) newDialog() {
	a.unsavedThen("start a new map", a.openNewMap)
}

// exportFiles writes the chosen files in the background.
func (a *app) exportFiles(base string, o export.Options) {
	a.exporting.Store(true)
	go func() {
		defer a.wakeUp()
		defer a.exporting.Store(false)

		files, err := a.session.Export(base, o)
		switch {
		case err != nil:
			a.results <- message{text: err.Error(), error: true}
		case len(files) == 0:
			a.results <- message{text: "Nothing to export"}
		default:
			a.results <- message{text: "Exported " + strings.Join(files, ", ")}
		}
	}()
}

// quit closes the app, after asking about unsaved changes.
func (a *app) quit() {
	a.unsavedThen("quit", func() {
		a.quitting = true
		a.window.SetShouldClose(true)
	})
}
