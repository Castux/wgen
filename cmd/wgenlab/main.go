// Command wgenlab compares variants of the generation: it generates a base
// config patched with each combination of cases and variants, renders the
// results, and measures them. A development tool, to judge changes to the
// algorithm on more than looks.
//
// Usage:
//
//	wgenlab [-v] [-out dir] experiment.json
//
// The experiment file:
//
//	{
//		"config": "lab/chasers.json",       // base config, relative to the experiment file
//		"cases": {"small": {}, "continent": {"resolution": 8}},
//		"variants": {"baseline": {}, "worley": {"noiseType": "worley"}},
//		"region": "mountains",              // terrain measured
//		"channelArea": 20000,               // drainage area of channels for the measures, square pixels
//		"contours": 10,                     // contour interval of the renders
//		"crops": {"sw": [80, 1380, 400, 1860]} // image regions also rendered at full scale: x0, y0, x1, y1, top left origin
//	}
//
// Cases and variants are partial configs, applied in this order. Writes
// <case>-<variant>.png (whole map), <case>-<variant>-<crop>.png, and
// report.md with the measures, to the output directory.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Castux/wgen/internal/config"
	"github.com/Castux/wgen/internal/gen"
	"github.com/Castux/wgen/internal/render"
)

type experiment struct {
	Config      string                     `json:"config"`
	Cases       map[string]json.RawMessage `json:"cases"`
	Variants    map[string]json.RawMessage `json:"variants"`
	Region      string                     `json:"region"`
	ChannelArea float64                    `json:"channelArea"`
	Contours    float64                    `json:"contours"`
	Crops       map[string][4]int          `json:"crops"`
}

func main() {
	out := flag.String("out", "lab/out", "output directory")
	verbose := flag.Bool("v", false, "verbose logging (stage timings)")
	flag.Parse()
	if *verbose {
		slog.SetLogLoggerLevel(slog.LevelDebug)
	}
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "Usage: wgenlab [-v] [-out dir] experiment.json")
		os.Exit(2)
	}

	if err := run(flag.Arg(0), *out); err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
}

func run(path, out string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var e experiment
	if err := json.Unmarshal(data, &e); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if e.ChannelArea == 0 {
		e.ChannelArea = 20000
	}

	base, warnings, err := config.Load(filepath.Join(filepath.Dir(path), e.Config))
	for _, w := range warnings {
		slog.Warn(w)
	}
	if err != nil {
		return err
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}

	cases, variants := sortedKeys(e.Cases), sortedKeys(e.Variants)
	var report strings.Builder
	fmt.Fprintf(&report, "# %s\n\nRegion: %s. Channels: drainage area above %g px².\n\n", filepath.Base(path), e.Region, e.ChannelArea)

	for _, c := range cases {
		fmt.Fprintf(&report, "## %s\n\n", c)
		fmt.Fprintln(&report, "| variant | time | vertices | region max z | peaks | peaks / Mpx² | drainage density | bifurcation ratio | Hack exponent |")
		fmt.Fprintln(&report, "|---|---|---|---|---|---|---|---|---|")

		for _, v := range variants {
			conf, err := base.Patch(e.Cases[c])
			if err == nil {
				conf, err = conf.Patch(e.Variants[v])
			}
			if err != nil {
				return fmt.Errorf("%s %s: %w", c, v, err)
			}

			start := time.Now()
			w, err := gen.New(conf)
			if err != nil {
				return fmt.Errorf("%s %s: %w", c, v, err)
			}
			took := time.Since(start)

			m := measure(w, e.Region, e.ChannelArea)
			fmt.Fprintf(&report, "| %s | %.1fs | %d | %.1f | %d | %.1f | %.2f | %.2f | %.2f |\n",
				v, took.Seconds(), len(w.Mesh.Points), m.maxZ, m.peaks, m.peakDensity, m.drainageDensity, m.bifurcation, m.hack)

			name := c + "-" + v
			if err := renders(w, e, filepath.Join(out, name)); err != nil {
				return err
			}
			slog.Info("done", "case", c, "variant", v, "took", took.Round(time.Millisecond))
		}
		report.WriteString("\n")
	}

	report.WriteString(`Measures, on the region's vertices:

- peaks: local maxima of elevation.
- drainage density: length of channels per area, per 1000 px.
- bifurcation ratio: how many streams of each Strahler order there are per
  stream of the next order, on land (3 to 5 in natural river networks).
- Hack exponent: h in L ~ A^h, on the region's channels: L is the length of
  the longest flow path upstream, A the drainage area. About 0.6 in nature;
  1 for parallel, unbranched drainage (area grows like length).
`)

	return os.WriteFile(filepath.Join(out, "report.md"), []byte(report.String()), 0o644)
}

func sortedKeys[T any](m map[string]T) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// renders writes the whole map, and the crops at full scale: elevation,
// hillshading, rivers and contour lines.
func renders(w *gen.World, e experiment, prefix string) error {
	options := func(scale float64) render.Options {
		return render.Options{
			Scale: scale, Base: render.BaseHeight, Shading: true,
			RiverPower: 0.5, RiverWidth: 12, Contours: e.Contours,
		}
	}

	scale := 1024 / float64(max(w.Width, w.Height))
	if err := writePNG(prefix+".png", render.Render(w, options(scale))); err != nil {
		return err
	}

	if len(e.Crops) == 0 {
		return nil
	}
	full := render.Render(w, options(1))
	for name, r := range e.Crops {
		rect := image.Rect(r[0], r[1], r[2], r[3]).Intersect(full.Bounds())
		crop := image.NewRGBA(image.Rect(0, 0, rect.Dx(), rect.Dy()))
		draw.Draw(crop, crop.Bounds(), full, rect.Min, draw.Src)
		if err := writePNG(prefix+"-"+name+".png", crop); err != nil {
			return err
		}
	}
	return nil
}

func writePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

type measures struct {
	maxZ            float64
	peaks           int
	peakDensity     float64 // per million square pixels
	drainageDensity float64 // channel length per 1000 square pixels
	bifurcation     float64
	hack            float64
}

func measure(w *gen.World, region string, channelArea float64) measures {
	m := w.Mesh
	n := len(m.Points)

	cells := gen.CellAreas(m)
	isChannel := func(v int32) bool {
		return w.Terrain[v] != nil && w.IsLand(v) && w.Drainage[v] >= channelArea
	}
	inRegion := func(v int32) bool { return w.Terrain[v] != nil && w.Terrain[v].Name == region }

	var r measures
	r.maxZ = math.Inf(-1)
	regionArea := 0.0
	channelLength := 0.0

	for v := range int32(n) {
		if !inRegion(v) {
			continue
		}
		regionArea += cells[v]
		if !math.IsNaN(w.Z[v]) {
			r.maxZ = math.Max(r.maxZ, w.Z[v])
		}

		peak := true
		for _, u := range m.Neighbours[v] {
			if !(w.Z[u] < w.Z[v]) {
				peak = false
				break
			}
		}
		if peak {
			r.peaks++
		}

		if isChannel(v) && w.Downhill[v] >= 0 {
			channelLength += m.Points[v].Dist(m.Points[w.Downhill[v]])
		}
	}

	if regionArea > 0 {
		r.peakDensity = float64(r.peaks) / regionArea * 1e6
		r.drainageDensity = channelLength / regionArea * 1000
	}

	// Land vertices from the top down: upstream before downstream
	var order []int32
	for v := range int32(n) {
		if w.Terrain[v] != nil && w.IsLand(v) {
			order = append(order, v)
		}
	}
	slices.SortFunc(order, func(a, b int32) int {
		switch {
		case w.Z[a] > w.Z[b]:
			return -1
		case w.Z[a] < w.Z[b]:
			return 1
		}
		return 0
	})

	// Strahler orders of channels, and the longest flow path upstream of every
	// vertex
	strahler := make([]int, n)
	maxIn := make([]int, n)      // highest order flowing in
	maxInCount := make([]int, n) // how many streams of that order
	length := make([]float64, n)
	for _, v := range order {
		d := w.Downhill[v]
		if d >= 0 {
			length[d] = math.Max(length[d], length[v]+m.Points[v].Dist(m.Points[d]))
		}
		if !isChannel(v) {
			continue
		}

		switch {
		case maxIn[v] == 0:
			strahler[v] = 1
		case maxInCount[v] >= 2:
			strahler[v] = maxIn[v] + 1
		default:
			strahler[v] = maxIn[v]
		}

		if d < 0 || !isChannel(d) {
			continue
		}
		switch {
		case strahler[v] > maxIn[d]:
			maxIn[d], maxInCount[d] = strahler[v], 1
		case strahler[v] == maxIn[d]:
			maxInCount[d]++
		}
	}

	// Streams of each order: counted at their outlet, where the order
	// changes or the channel ends
	streams := map[int]int{}
	for _, v := range order {
		if !isChannel(v) {
			continue
		}
		d := w.Downhill[v]
		if d < 0 || !isChannel(d) || strahler[d] != strahler[v] {
			streams[strahler[v]]++
		}
	}
	var ratios []float64
	for k := 1; streams[k+1] > 0; k++ {
		ratios = append(ratios, math.Log(float64(streams[k])/float64(streams[k+1])))
	}
	if len(ratios) > 0 {
		r.bifurcation = math.Exp(mean(ratios))
	}

	// Hack: least squares fit of log L against log A, on the channels of the
	// region
	var xs, ys []float64
	for _, v := range order {
		if isChannel(v) && inRegion(v) && length[v] > 0 {
			xs = append(xs, math.Log(w.Drainage[v]))
			ys = append(ys, math.Log(length[v]))
		}
	}
	r.hack = slope(xs, ys)

	return r
}

func mean(xs []float64) float64 {
	sum := 0.0
	for _, x := range xs {
		sum += x
	}
	return sum / float64(len(xs))
}

func slope(xs, ys []float64) float64 {
	if len(xs) < 2 {
		return math.NaN()
	}
	mx, my := mean(xs), mean(ys)
	var sxy, sxx float64
	for i := range xs {
		sxy += (xs[i] - mx) * (ys[i] - my)
		sxx += (xs[i] - mx) * (xs[i] - mx)
	}
	return sxy / sxx
}
