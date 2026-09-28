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
	"slices"
	"strings"

	"github.com/Castux/wgen/internal/app"
	"github.com/Castux/wgen/internal/config"
	"github.com/Castux/wgen/internal/engine"
	"github.com/Castux/wgen/internal/export"
	"github.com/Castux/wgen/internal/gen"
)

// version is set by the release builds (see scripts/package.sh).
var version = "dev"

func main() {
	verbose := flag.Bool("v", false, "verbose logging (stage timings)")
	printVersion := flag.Bool("version", false, "print the version")
	exports := flag.String("export", "", "export without opening the app: comma separated list of heightmap, water, texture, obj, svg")
	normalized := flag.Bool("normalized", false, "with -export heightmap: from the lowest to the highest point, instead of meters")
	output := flag.String("o", "", "with -export: output path without extension (default: next to the project)")

	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: wgen [flags] [project.json | image.png]")
		flag.PrintDefaults()
	}

	// Allow flags after the path too. Old macOS versions give apps opened from
	// the Finder a process serial number, -psn_...
	var path string
	args := slices.DeleteFunc(os.Args[1:], func(arg string) bool { return strings.HasPrefix(arg, "-psn_") })
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

	if *printVersion {
		fmt.Println("wgen", version)
		return
	}
	app.Version = version
	setupLogging(*verbose, *exports == "")
	slog.Debug("wgen", "version", version)

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
	return app.Run(session, path)
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

	options, err := exportOptions(list, normalized)
	if err != nil {
		return err
	}

	world, err := gen.New(conf)
	if err != nil {
		return err
	}

	if output == "" {
		output = strings.TrimSuffix(path, filepath.Ext(path))
	}
	files, err := export.All(world, output, options)
	for _, file := range files {
		fmt.Println(file)
	}
	return err
}

// exportOptions reads the -export list.
func exportOptions(list string, normalized bool) (export.Options, error) {
	options := export.Options{Normalized: normalized}
	for _, name := range strings.Split(list, ",") {
		switch strings.TrimSpace(name) {
		case "heightmap":
			options.Heightmap = true
		case "water":
			options.WaterMask = true
		case "texture":
			options.Texture = true
		case "obj":
			options.OBJ = true
		case "svg":
			options.SVG = true
		default:
			return options, fmt.Errorf("unknown export %q (expected heightmap, water, texture, obj, svg)", name)
		}
	}
	return options, nil
}
