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
//		"config": "../assets/example/chasers.json", // base config, relative to the experiment file
//		"cases": {"small": {}, "continent": {"resolution": 4000}},
//		"variants": {"baseline": {}, "soft": {"simulation": {"erodibility": 4e-6}}},
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

// experimentFile is the experiment file.
type experimentFile struct {
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
	var experiment experimentFile
	if err := json.Unmarshal(data, &experiment); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if experiment.ChannelArea == 0 {
		experiment.ChannelArea = 20000
	}

	base, warnings, err := config.Load(filepath.Join(filepath.Dir(path), experiment.Config))
	for _, warning := range warnings {
		slog.Warn(warning)
	}
	if err != nil {
		return err
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}

	cases, variants := sortedKeys(experiment.Cases), sortedKeys(experiment.Variants)
	var report, heights strings.Builder
	fmt.Fprintf(&report, "# %s\n\nRegion: %s. Channels: drainage area above %g px².\n\n", filepath.Base(path), experiment.Region, experiment.ChannelArea)

	for _, caseName := range cases {
		fmt.Fprintf(&report, "## %s\n\n", caseName)
		fmt.Fprintln(&report, "| variant | time | vertices | region max z | peaks | peaks / Mpx² | drainage density | bifurcation ratio | Hack exponent |")
		fmt.Fprintln(&report, "|---|---|---|---|---|---|---|---|---|")

		for _, variant := range variants {
			conf, err := base.Patch(experiment.Cases[caseName])
			if err == nil {
				conf, err = conf.Patch(experiment.Variants[variant])
			}
			if err != nil {
				return fmt.Errorf("%s %s: %w", caseName, variant, err)
			}

			start := time.Now()
			world, err := gen.New(conf)
			if err != nil {
				return fmt.Errorf("%s %s: %w", caseName, variant, err)
			}
			took := time.Since(start)

			m := measure(world, experiment.Region, experiment.ChannelArea)
			fmt.Fprintf(&report, "| %s | %.1fs | %d | %.1f | %d | %.1f | %.2f | %.2f | %.2f |\n",
				variant, took.Seconds(), len(world.Mesh.Points), m.maxZ, m.peaks, m.peakDensity, m.drainageDensity, m.bifurcation, m.hack)

			heights.WriteString(heightTable(world, caseName+" "+variant))

			if err := renders(world, experiment, filepath.Join(out, caseName+"-"+variant)); err != nil {
				return err
			}
			slog.Info("done", "case", caseName, "variant", variant, "took", took.Round(time.Millisecond))
		}
		report.WriteString("\n")
	}

	report.WriteString("## Heights per terrain\n\n")
	report.WriteString("| case, variant | terrain | target | median | 95% | 99% | max |\n|---|---|---|---|---|---|---|\n")
	report.WriteString(heights.String())
	report.WriteString("\n")

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
func renders(w *gen.World, experiment experimentFile, prefix string) error {
	options := func(scale float64) render.Options {
		return render.Options{
			Scale: scale, Base: render.BaseHeight, Shading: true,
			RiverPower: 0.5, RiverWidth: 12, Contours: experiment.Contours,
		}
	}

	scale := 1024 / float64(max(w.Width, w.Height))
	if err := writePNG(prefix+".png", render.Render(w, options(scale))); err != nil {
		return err
	}

	if len(experiment.Crops) == 0 {
		return nil
	}
	full := render.Render(w, options(1))
	for name, bounds := range experiment.Crops {
		rect := image.Rect(bounds[0], bounds[1], bounds[2], bounds[3]).Intersect(full.Bounds())
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

// network is a generated world seen as a river network, for the measures.
type network struct {
	w           *gen.World
	region      string
	channelArea float64
}

// isChannel tells whether a vertex drains enough land to be a channel.
func (n network) isChannel(v int32) bool {
	return n.w.Terrain[v] != nil && n.w.IsLand(v) && n.w.Drainage[v] >= n.channelArea
}

func (n network) inRegion(v int32) bool {
	return n.w.Terrain[v] != nil && n.w.Terrain[v].Name == n.region
}

func measure(w *gen.World, region string, channelArea float64) measures {
	n := network{w: w, region: region, channelArea: channelArea}
	m := n.regionMeasures()

	order := n.landTopDown()
	strahler, length := n.streamOrders(order)
	m.bifurcation = n.bifurcationRatio(order, strahler)
	m.hack = n.hackExponent(order, length)
	return m
}

// regionMeasures measures the heights, peaks and channels of the region.
func (n network) regionMeasures() measures {
	w, mesh := n.w, n.w.Mesh
	cells := gen.CellAreas(mesh)

	var m measures
	m.maxZ = math.Inf(-1)
	regionArea := 0.0
	channelLength := 0.0

	for v := range int32(len(mesh.Points)) {
		if !n.inRegion(v) {
			continue
		}
		regionArea += cells[v]
		if !math.IsNaN(w.Elevation[v]) {
			m.maxZ = math.Max(m.maxZ, w.Elevation[v])
		}

		peak := true
		for _, u := range mesh.Neighbours[v] {
			if !(w.Elevation[u] < w.Elevation[v]) {
				peak = false
				break
			}
		}
		if peak {
			m.peaks++
		}

		if n.isChannel(v) && w.Downhill[v] >= 0 {
			channelLength += mesh.Points[v].Dist(mesh.Points[w.Downhill[v]])
		}
	}

	if regionArea > 0 {
		m.peakDensity = float64(m.peaks) / regionArea * 1e6
		m.drainageDensity = channelLength / regionArea * 1000
	}
	return m
}

// landTopDown returns the land vertices from the top down: upstream before
// downstream.
func (n network) landTopDown() []int32 {
	w := n.w
	var order []int32
	for v := range int32(len(w.Mesh.Points)) {
		if w.Terrain[v] != nil && w.IsLand(v) {
			order = append(order, v)
		}
	}
	slices.SortFunc(order, func(a, b int32) int {
		switch {
		case w.Elevation[a] > w.Elevation[b]:
			return -1
		case w.Elevation[a] < w.Elevation[b]:
			return 1
		}
		return 0
	})
	return order
}

// streamOrders returns the Strahler orders of channels, and the length of
// the longest flow path upstream of every vertex, given the land vertices
// upstream first.
func (n network) streamOrders(order []int32) (strahler []int, length []float64) {
	w, mesh := n.w, n.w.Mesh
	count := len(mesh.Points)
	strahler = make([]int, count)
	maxIn := make([]int, count)      // highest order flowing in
	maxInCount := make([]int, count) // how many streams of that order
	length = make([]float64, count)

	for _, v := range order {
		d := w.Downhill[v]
		if d >= 0 {
			length[d] = math.Max(length[d], length[v]+mesh.Points[v].Dist(mesh.Points[d]))
		}
		if !n.isChannel(v) {
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

		if d < 0 || !n.isChannel(d) {
			continue
		}
		switch {
		case strahler[v] > maxIn[d]:
			maxIn[d], maxInCount[d] = strahler[v], 1
		case strahler[v] == maxIn[d]:
			maxInCount[d]++
		}
	}
	return strahler, length
}

// bifurcationRatio is the geometric mean of the ratios of the number of
// streams of each order to the next.
func (n network) bifurcationRatio(order []int32, strahler []int) float64 {
	// Streams of each order: counted at their outlet, where the order
	// changes or the channel ends
	streams := map[int]int{}
	for _, v := range order {
		if !n.isChannel(v) {
			continue
		}
		d := n.w.Downhill[v]
		if d < 0 || !n.isChannel(d) || strahler[d] != strahler[v] {
			streams[strahler[v]]++
		}
	}

	var ratios []float64
	for k := 1; streams[k+1] > 0; k++ {
		ratios = append(ratios, math.Log(float64(streams[k])/float64(streams[k+1])))
	}
	if len(ratios) == 0 {
		return 0
	}
	return math.Exp(mean(ratios))
}

// hackExponent is the least squares fit of log L against log A, on the
// channels of the region.
func (n network) hackExponent(order []int32, length []float64) float64 {
	var logAreas, logLengths []float64
	for _, v := range order {
		if n.isChannel(v) && n.inRegion(v) && length[v] > 0 {
			logAreas = append(logAreas, math.Log(n.w.Drainage[v]))
			logLengths = append(logLengths, math.Log(length[v]))
		}
	}
	return slope(logAreas, logLengths)
}

func mean(xs []float64) float64 {
	sum := 0.0
	for _, x := range xs {
		sum += x
	}
	return sum / float64(len(xs))
}

// slope is the least squares slope of ys against xs.
func slope(xs, ys []float64) float64 {
	if len(xs) < 2 {
		return math.NaN()
	}
	meanX, meanY := mean(xs), mean(ys)
	var sxy, sxx float64
	for i := range xs {
		sxy += (xs[i] - meanX) * (ys[i] - meanY)
		sxx += (xs[i] - meanX) * (xs[i] - meanX)
	}
	return sxy / sxx
}

// heightTable gives quantiles of the elevation of each land terrain, with
// its target height, as markdown table rows.
func heightTable(w *gen.World, name string) string {
	var b strings.Builder
	for _, t := range w.Config.Terrains {
		if t.IsWater() {
			continue
		}
		var zs []float64
		for v, terrain := range w.Terrain {
			if terrain == t && !math.IsNaN(w.Elevation[v]) {
				zs = append(zs, w.Elevation[v])
			}
		}
		if len(zs) == 0 {
			continue
		}
		slices.Sort(zs)
		quantile := func(f float64) float64 { return zs[int(f*float64(len(zs)-1))] }
		fmt.Fprintf(&b, "| %s | %s | %.0f | %.0f | %.0f | %.0f | %.0f |\n",
			name, t.Name, t.Height, quantile(0.5), quantile(0.95), quantile(0.99), zs[len(zs)-1])
	}
	return b.String()
}
