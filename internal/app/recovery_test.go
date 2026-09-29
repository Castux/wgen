package app

import (
	"errors"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/Castux/wgen/internal/config"
	"github.com/Castux/wgen/internal/engine"
)

func TestRecoveryRoundTrip(t *testing.T) {
	dir := t.TempDir() + "/recovery"
	if _, _, _, err := readRecovery(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("no copy: %v", err)
	}

	conf := config.Default("somewhere.png")
	conf.MapWidth = 1234
	sea, plains := conf.Terrain(config.SeaName).Color, conf.Land()[0].Color
	paintedMap := &engine.Map{Width: 3, Height: 2, Pixels: []config.Color{sea, plains, sea, plains, plains, sea}}
	details := recoveryDetails{Project: "D:/maps/island.json", Time: time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)}
	if err := writeRecovery(dir, conf, paintedMap, details); err != nil {
		t.Fatal(err)
	}

	recovered, recoveredMap, recoveredDetails, err := readRecovery(dir)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.MapWidth != 1234 || recovered.Path != "" || recovered.Image != "" {
		t.Errorf("recovered config: width %g, path %q, image %q", recovered.MapWidth, recovered.Path, recovered.Image)
	}
	if recoveredMap.Width != 3 || recoveredMap.Height != 2 || !slices.Equal(recoveredMap.Pixels, paintedMap.Pixels) {
		t.Errorf("recovered map %+v, want %+v", recoveredMap, paintedMap)
	}
	if recoveredDetails.Project != details.Project || !recoveredDetails.Time.Equal(details.Time) {
		t.Errorf("recovered details %+v, want %+v", recoveredDetails, details)
	}

	// Incomplete: without its info, a copy isn't one
	os.Remove(dir + "/" + recoveryInfo)
	if _, _, _, err := readRecovery(dir); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("incomplete copy: %v", err)
	}
}
