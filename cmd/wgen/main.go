// Command wgen generates terrain heightmaps from terrain outline images.
//
// Usage:
//
//	wgen [flags] <config.json>
//
// Without flags, generates the world and writes the exports enabled in the
// config.
package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/Castux/wgen/internal/config"
	"github.com/Castux/wgen/internal/export"
	"github.com/Castux/wgen/internal/gen"
)

func main() {
	verbose := flag.Bool("v", false, "verbose logging (stage timings)")

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

	if err := run(path); err != nil {
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

	return export.All(w)
}

func loadConfig(path string) (*config.Config, error) {
	conf, warnings, err := config.Load(path)
	for _, warning := range warnings {
		slog.Warn(warning, "config", path)
	}
	return conf, err
}
