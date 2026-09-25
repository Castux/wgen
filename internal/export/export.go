package export

import (
	"log/slog"

	"github.com/Castux/wgen/internal/gen"
)

// All writes the outputs enabled in the config next to it: <config>.obj,
// <config>.svg, <config>.png and <config>-w.png (water level).
func All(w *gen.World) error {
	base := w.Conf.ConfigPath

	if w.Conf.ExportSVG {
		slog.Info("exporting SVG", "path", base+".svg")
		if err := SVG(w, base+".svg"); err != nil {
			return err
		}
	}

	if w.Conf.ExportOBJ {
		slog.Info("exporting OBJ", "path", base+".obj")
		if err := OBJ(w, base+".obj"); err != nil {
			return err
		}
	}

	if w.Conf.ExportHeightmap {
		if err := Heightmaps(w, base+".png", base+"-w.png"); err != nil {
			return err
		}
	}

	return nil
}
