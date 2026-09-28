// Command wgen turns painted maps into landscapes.
//
// Usage:
//
//	wgen [flags] [project.json | image.png]
//
// Without -export, opens the app, with the given project or image if any.
// With -export, generates the project and writes the chosen files, without
// opening a window.
package main

import (
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/Castux/wgen/internal/config"
	"github.com/Castux/wgen/internal/engine"
	"github.com/Castux/wgen/internal/export"
	"github.com/Castux/wgen/internal/gen"
	"github.com/Castux/wgen/internal/viewer"
)

func main() {
	verbose := flag.Bool("v", false, "verbose logging (stage timings)")
	exports := flag.String("export", "", "export without opening the app: comma separated list of heightmap, water, texture, obj, svg")
	normalized := flag.Bool("normalized", false, "with -export heightmap: from the lowest to the highest point, instead of meters")
	output := flag.String("o", "", "with -export: output path without extension (default: next to the project)")

	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: wgen [flags] [project.json | image.png]")
		flag.PrintDefaults()
	}

	// Allow flags after the path too
	var path string
	args := os.Args[1:]
	for len(args) > 0 {
		flag.CommandLine.Parse(args)
		args = flag.Args()
		if len(args) > 0 {
			if path != "" {
				flag.Usage()
				os.Exit(2)
			}
			path, args = args[0], args[1:]
		}
	}

	setupLogging(*verbose, *exports == "")

	var err error
	if *exports != "" {
		err = exportProject(path, *exports, *normalized, *output)
	} else {
		err = openApp(path)
	}
	if err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
}

// setupLogging logs to stderr, and for the app to a file as well (a double
// clicked app has no console): wgen.log in the user config directory.
func setupLogging(verbose, app bool) {
	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}

	var out io.Writer = os.Stderr
	if dir, err := os.UserConfigDir(); err == nil && app {
		dir = filepath.Join(dir, "wgen")
		os.MkdirAll(dir, 0o755)
		if f, err := os.Create(filepath.Join(dir, "wgen.log")); err == nil {
			out = io.MultiWriter(os.Stderr, f)
		}
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(out, &slog.HandlerOptions{Level: level})))
}

func openApp(path string) error {
	session, err := engine.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()
	return viewer.Run(session, path)
}

func exportProject(path, list string, normalized bool, output string) error {
	if path == "" {
		return fmt.Errorf("-export needs a project file")
	}
	conf, warnings, err := config.Load(path)
	for _, w := range warnings {
		slog.Warn(w, "project", path)
	}
	if err != nil {
		return err
	}

	o := export.Options{Normalized: normalized}
	for _, name := range strings.Split(list, ",") {
		switch strings.TrimSpace(name) {
		case "heightmap":
			o.Heightmap = true
		case "water":
			o.WaterMask = true
		case "texture":
			o.Texture = true
		case "obj":
			o.OBJ = true
		case "svg":
			o.SVG = true
		default:
			return fmt.Errorf("unknown export %q (expected heightmap, water, texture, obj, svg)", name)
		}
	}

	w, err := gen.New(conf)
	if err != nil {
		return err
	}

	if output == "" {
		output = strings.TrimSuffix(path, filepath.Ext(path))
	}
	files, err := export.All(w, output, o)
	for _, f := range files {
		fmt.Println(f)
	}
	return err
}
