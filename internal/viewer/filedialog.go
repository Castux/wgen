package viewer

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/AllenDang/cimgui-go/imgui"
)

// The file browser: drawn with ImGui, with places and drives, rather than a
// native dialog, which would need more C dependencies.

type fileDialog struct {
	opening bool
	title   string
	save    bool
	exts    []string // lower case, with the dot; empty: all files
	dir     string
	name    string // file name, typed or selected
	confirm bool   // save: overwriting asked
	done    func(path string)

	listed  string // dir of the entries
	entries []os.DirEntry
	err     string
}

// place is a shortcut to a common directory.
type place struct {
	name, path string
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

// list reads the directory, if it changed: its subdirectories first, then
// the files with the wanted extensions, hidden ones left out.
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

// places are the home directory and its usual subdirectories, the project's
// directory, and the drives or the root.
func (a *app) places() []place {
	var places []place
	if home, err := os.UserHomeDir(); err == nil {
		places = append(places, place{"Home", home})
		for _, name := range []string{"Desktop", "Documents", "Pictures", "Downloads"} {
			if path := filepath.Join(home, name); isDir(path) {
				places = append(places, place{name, path})
			}
		}
	}
	if path := a.session.ProjectPath(); path != "" {
		places = append(places, place{"Project", filepath.Dir(path)})
	}
	if runtime.GOOS == "windows" {
		for letter := 'A'; letter <= 'Z'; letter++ {
			if drive := string(letter) + `:\`; isDir(drive) {
				places = append(places, place{drive, drive})
			}
		}
	} else {
		places = append(places, place{"/", "/"})
	}
	return places
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
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
		if imgui.SelectableBool(p.name) {
			d.dir = p.path
		}
	}
	imgui.EndChild()
	imgui.SameLine()

	imgui.BeginChildStrV("files", imgui.NewVec2(0, -footer), imgui.ChildFlagsBorders, 0)
	a.drawFileEntries()
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
		imgui.TextColored(noticeColor, noFormat(d.name+" already exists"))
	}
	imgui.EndPopup()
}

// drawFileEntries lists the directory: a click selects a file, a double
// click opens a directory or picks a file.
func (a *app) drawFileEntries() {
	d := &a.dialogs.file
	if d.err != "" {
		imgui.TextColored(errorColor, noFormat(d.err))
	}
	for _, e := range d.entries {
		name := e.Name()
		label := name
		if e.IsDir() {
			label = "[" + name + "]"
		}
		if !imgui.SelectableBoolV(label, !e.IsDir() && name == d.name, imgui.SelectableFlagsAllowDoubleClick, imgui.NewVec2(0, 0)) {
			continue
		}
		doubleClick := imgui.IsMouseDoubleClicked(imgui.MouseButtonLeft)
		if e.IsDir() {
			if doubleClick {
				d.dir = filepath.Join(d.dir, name)
			}
			continue
		}
		d.name = name
		d.confirm = false
		if doubleClick {
			a.fileChosen()
		}
	}
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
