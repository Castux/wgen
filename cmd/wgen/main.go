// Command wgen generates terrain heightmaps from terrain outline images.
//
// Usage:
//
//	wgen [flags] <config.json>
//
// Without flags, generates the world and writes the exports enabled in the
// config. With --interactive, opens the viewer instead, regenerating whenever
// the config or the outline image change.
package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/Castux/wgen/internal/config"
	"github.com/Castux/wgen/internal/engine"
	"github.com/Castux/wgen/internal/export"
	"github.com/Castux/wgen/internal/gen"
	"github.com/Castux/wgen/internal/viewer"
)

func main() {
	verbose := flag.Bool("v", false, "verbose logging (stage timings)")
	interactive := flag.Bool("interactive", false, "open the viewer")

	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: wgen [flags] <config.json>")
		flag.PrintDefaults()
	}

	// Allow flags after the config path too
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

	if path == "" {
		flag.Usage()
		os.Exit(2)
	}

	level := slog.LevelInfo
	if *verbose {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))

	var err error
	if *interactive {
		err = view(path)
	} else {
		err = run(path)
	}

	if err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
}

func run(path string) error {
	conf, err := loadConfig(path)
	if err != nil {
		return err
	}

	w, err := gen.New(conf)
	if err != nil {
		return err
	}

	_, err = export.All(w)
	return err
}

func view(path string) error {
	session, err := engine.Open(path)
	if err != nil {
		return err
	}
	defer session.Close()

	return viewer.Run(session)
}

func loadConfig(path string) (*config.Config, error) {
	conf, warnings, err := config.Load(path)
	for _, warning := range warnings {
		slog.Warn(warning, "config", path)
	}
	return conf, err
}
