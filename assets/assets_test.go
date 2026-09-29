package assets

import (
	"testing"

	"github.com/Castux/wgen/internal/config"
)

func TestExample(t *testing.T) {
	project, mapImage := Example()
	conf, warnings, err := config.Parse(project)
	if err != nil || len(warnings) > 0 {
		t.Fatalf("example project: %v, warnings %v", err, warnings)
	}
	if b := mapImage.Bounds(); b.Dx() == 0 || b.Dy() == 0 {
		t.Fatal("empty example map")
	}
	if conf.Terrain(config.SeaName) == nil || len(conf.Land()) == 0 {
		t.Error("example without sea or land terrains")
	}
	if Icon().Bounds().Dx() != 512 {
		t.Error("icon size")
	}
}
