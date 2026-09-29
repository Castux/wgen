package app

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"log/slog"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/AllenDang/cimgui-go/imgui"

	"github.com/Castux/wgen/internal/config"
	"github.com/Castux/wgen/internal/engine"
	"github.com/Castux/wgen/internal/gen"
)

// Autosave and recovery: while the project has unsaved changes, a copy is
// written every minute to the recovery directory, next to the settings.
// Saving, or quitting, removes it; if the app didn't quit (a crash, killed),
// the next start offers to recover it.

// autosaveInterval is the time between recovery copies, by default.
const autosaveInterval = time.Minute

// Files of a recovery copy; the info is written last: a copy without it is
// incomplete
const (
	recoveryProject = "project.json"
	recoveryMap     = "map.png"
	recoveryInfo    = "info.json"
)

// recoveryDetails tells where a recovery copy comes from.
type recoveryDetails struct {
	Project   string    `json:"project,omitempty"`   // its file, if saved before
	Suggested string    `json:"suggested,omitempty"` // where to save it, if not
	Time      time.Time `json:"time"`
}

// writeRecovery writes a recovery copy of a project to dir.
func writeRecovery(dir string, conf *config.Config, paintedMap *engine.Map, details recoveryDetails) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	os.Remove(filepath.Join(dir, recoveryInfo)) // incomplete while writing

	conf = conf.Clone()
	conf.Image = recoveryMap
	image, err := engine.EncodeMap(paintedMap)
	if err != nil {
		return err
	}
	info, err := json.MarshalIndent(details, "", "\t")
	if err != nil {
		return err
	}
	for _, file := range []struct {
		name string
		data []byte
	}{{recoveryProject, conf.Marshal()}, {recoveryMap, image}, {recoveryInfo, info}} {
		if err := writeFileAtomically(filepath.Join(dir, file.name), file.data); err != nil {
			return err
		}
	}
	return nil
}

// writeFileAtomically writes a file under another name, then renames it.
func writeFileAtomically(path string, data []byte) error {
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, data, 0o644); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}

// readRecovery reads the recovery copy in dir: os.ErrNotExist if none.
func readRecovery(dir string) (*config.Config, *engine.Map, recoveryDetails, error) {
	var details recoveryDetails
	data, err := os.ReadFile(filepath.Join(dir, recoveryInfo))
	if err != nil {
		return nil, nil, details, err
	}
	if err := json.Unmarshal(data, &details); err != nil {
		return nil, nil, details, err
	}

	conf, _, err := config.Load(filepath.Join(dir, recoveryProject))
	if err != nil {
		return nil, nil, details, err
	}
	f, err := os.Open(conf.ImagePath())
	if err != nil {
		return nil, nil, details, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return nil, nil, details, err
	}
	width, height, pixels := gen.MapColors(img)

	conf.Path, conf.Image = "", ""
	return conf, &engine.Map{Width: width, Height: height, Pixels: pixels}, details, nil
}

// autosaver writes the recovery copies of the app's project.
type autosaver struct {
	dir      string // "" without a settings directory: no autosave
	interval time.Duration
	last     time.Time // of the last copy, or check

	writing  atomic.Bool
	written  atomic.Bool // a copy of ours is in dir
	lastHash [sha256.Size]byte

	// A copy left by a previous run, to recover or discard
	recovered struct {
		open       bool
		conf       *config.Config
		paintedMap *engine.Map
		details    recoveryDetails
	}
}

func (a *app) initAutosave(settingsDir string) {
	s := &a.autosave
	s.interval = autosaveInterval
	if seconds := a.dev.autosaveSeconds; seconds > 0 {
		s.interval = time.Duration(seconds * float64(time.Second))
	}
	s.last = time.Now()
	if settingsDir == "" {
		return
	}
	s.dir = filepath.Join(settingsDir, "recovery")

	conf, paintedMap, details, err := readRecovery(s.dir)
	switch {
	case err == nil:
		r := &s.recovered
		r.open, r.conf, r.paintedMap, r.details = true, conf, paintedMap, details
		a.dialogs.recovery = true
		a.dialogs.welcome = false
	case !errors.Is(err, os.ErrNotExist):
		slog.Warn("cannot read the recovery copy", "dir", s.dir, "err", err)
	}
}

// autosaveTick writes a recovery copy when it is time, and removes ours
// once the project is saved.
func (a *app) autosaveTick() {
	s := &a.autosave
	if s.dir == "" || s.writing.Load() {
		return
	}
	if !a.unsaved() {
		if s.written.Load() {
			a.removeRecovery()
		}
		s.last = time.Now()
		return
	}
	if time.Since(s.last) < s.interval {
		return
	}
	s.last = time.Now()

	conf := a.session.Engine.Config()
	paintedMap := a.canvasMap()
	if paintedMap == nil && a.world != nil {
		paintedMap = &engine.Map{Width: a.world.Width, Height: a.world.Height, Pixels: a.world.Map}
	}
	if conf == nil || paintedMap == nil {
		return
	}
	details := recoveryDetails{Project: a.session.ProjectPath(), Suggested: a.suggestedPath, Time: time.Now()}

	s.writing.Store(true)
	go func() {
		defer s.writing.Store(false)

		// Not again if unchanged
		pixels := make([]byte, 0, 3*len(paintedMap.Pixels))
		for _, c := range paintedMap.Pixels {
			pixels = append(pixels, c[0], c[1], c[2])
		}
		hash := sha256.New()
		hash.Write(conf.Marshal())
		hash.Write(pixels)
		sum := [sha256.Size]byte(hash.Sum(nil))
		if sum == s.lastHash && s.written.Load() {
			return
		}

		if err := writeRecovery(s.dir, conf, paintedMap, details); err != nil {
			slog.Warn("cannot write the recovery copy", "dir", s.dir, "err", err)
			return
		}
		s.lastHash = sum
		s.written.Store(true)
		slog.Debug("recovery copy written", "dir", s.dir)
	}()
}

// removeRecovery removes the recovery copy, once no longer needed.
func (a *app) removeRecovery() {
	s := &a.autosave
	if s.dir == "" {
		return
	}
	for deadline := time.Now().Add(5 * time.Second); s.writing.Load() && time.Now().Before(deadline); {
		time.Sleep(10 * time.Millisecond)
	}
	if err := os.RemoveAll(s.dir); err != nil {
		slog.Warn("cannot remove the recovery copy", "dir", s.dir, "err", err)
	}
	s.written.Store(false)
}

// drawRecovery offers to recover the copy left by a previous run.
func (a *app) drawRecovery() {
	r := &a.autosave.recovered
	if !modal("recovery", "Recover unsaved changes", &a.dialogs.recovery, 0, 0) {
		return
	}

	name := "an untitled map"
	if path := r.details.Project; path != "" {
		name = displayName(path)
	}
	imgui.PushTextWrapPosV(imgui.CursorPosX() + 460*a.uiScale)
	imgui.TextUnformatted(fmt.Sprintf("wgen closed without saving the changes to %s, %s. Recover them?",
		name, r.details.Time.Local().Format("2 January at 15:04")))
	imgui.PopTextWrapPos()
	if path := r.details.Project; path != "" {
		imgui.TextDisabled(noFormat(path))
	}
	imgui.Spacing()

	if imgui.Button("Recover") {
		imgui.CloseCurrentPopup()
		a.recoverChanges()
	}
	imgui.SetItemTooltip("Open the recovered map; saving it suggests its file")
	imgui.SameLine()
	if imgui.Button("Discard them") {
		imgui.CloseCurrentPopup()
		a.discardRecovery()
	}
	imgui.EndPopup()
}

func (a *app) recoverChanges() {
	r := &a.autosave.recovered
	if !r.open {
		return
	}
	suggested := r.details.Project
	if suggested == "" {
		suggested = r.details.Suggested
	}
	a.startProject(r.conf, newCanvas(r.paintedMap.Width, r.paintedMap.Height, r.paintedMap.Pixels), suggested)
	a.message = &message{text: "Recovered the unsaved changes: save them to keep them"}
	a.autosave.written.Store(true) // ours now, removed once saved
	r.open, r.conf, r.paintedMap = false, nil, nil
}

func (a *app) discardRecovery() {
	r := &a.autosave.recovered
	a.autosave.written.Store(true)
	a.removeRecovery()
	r.open, r.conf, r.paintedMap = false, nil, nil
}
