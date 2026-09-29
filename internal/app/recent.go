package app

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

var isDarwin = runtime.GOOS == "darwin"

// Recent projects: the last ones opened or saved, most recent first, saved
// next to the settings (in a file of their own: Settings are compared by
// value).

const maxRecent = 10

func loadRecent(path string) []string {
	var recent []string
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	if err := json.Unmarshal(data, &recent); err != nil {
		slog.Warn("ignoring broken recent projects", "path", path, "err", err)
		return nil
	}
	return recent
}

func saveRecent(path string, recent []string) {
	data, err := json.MarshalIndent(recent, "", "\t")
	if err == nil {
		err = os.WriteFile(path, append(data, '\n'), 0o644)
	}
	if err != nil {
		slog.Warn("cannot save recent projects", "path", path, "err", err)
	}
}

// withRecent puts a project first in a recent list, once.
func withRecent(recent []string, path string) []string {
	same := func(other string) bool { return samePath(other, path) }
	recent = slices.Insert(slices.DeleteFunc(slices.Clone(recent), same), 0, path)
	return recent[:min(len(recent), maxRecent)]
}

// samePath compares paths, ignoring case on Windows and macOS, where file
// names usually do.
func samePath(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if os.PathSeparator == '\\' || isDarwin {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func (a *app) addRecent(path string) {
	a.recent = withRecent(a.recent, path)
	if a.recentPath != "" {
		saveRecent(a.recentPath, a.recent)
	}
}

func (a *app) clearRecent() {
	a.recent = nil
	if a.recentPath != "" {
		saveRecent(a.recentPath, a.recent)
	}
}

// displayName is a project's name, for lists: its file name, without the
// extension.
func displayName(path string) string {
	return strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
}
