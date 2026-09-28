package viewer

import (
	"fmt"
	"path/filepath"
	"slices"

	"github.com/AllenDang/cimgui-go/imgui"

	"github.com/Castux/wgen/internal/config"
)

// The import dialog: picking the colors of an image and their terrains
// (importer.go), with a preview of the result.

type importDialog struct {
	opening       bool
	path          string
	width, height int
	pixels        []config.Color
	colors        []colorCount
	conf          *config.Config
	picks         []pick
	result        bool // show the result instead of the image

	original, preview texture // downscaled
	previewOf         []pick
	previewResult     bool
}

// Preview images are at most this wide
const previewSize = 512

// Colors of the image offered to pick, the most common
const paletteShown = 24

func (a *app) openImport(path string, width, height int, pixels []config.Color) {
	conf := config.Default("")
	if current := a.session.Engine.Config(); current != nil {
		conf.MapWidth = current.MapWidth
	}
	d := &a.dialogs.imports
	d.original.delete()
	d.preview.delete()
	*d = importDialog{opening: true, path: path, width: width, height: height, pixels: pixels,
		colors: palette(pixels), conf: conf, picks: defaultPicks(width, height, pixels, conf)}
	d.original.uploadPixels(previewPixels(width, height, pixels))
}

// previewPixels downscales a map to RGBA pixels of at most previewSize,
// rows from the top.
func previewPixels(width, height int, pixels []config.Color) (int, int, []byte) {
	scale := max(1, (max(width, height)+previewSize-1)/previewSize)
	previewWidth, previewHeight := width/scale, height/scale
	out := make([]byte, 0, 4*previewWidth*previewHeight)
	for row := range previewHeight {
		y := height - 1 - row*scale
		for x := range previewWidth {
			c := pixels[y*width+x*scale]
			out = append(out, c[0], c[1], c[2], 255)
		}
	}
	return previewWidth, previewHeight, out
}

// addPick picks a color, as the first land terrain, unless it is already.
func (d *importDialog) addPick(color config.Color) {
	if slices.ContainsFunc(d.picks, func(p pick) bool { return p.color == color }) {
		return
	}
	terrain := config.SeaName
	if land := d.conf.Land(); len(land) > 0 {
		terrain = land[0].Name
	}
	d.picks = append(d.picks, pick{color, terrain})
}

func (a *app) drawImport() {
	d := &a.dialogs.imports
	if !modal("import", "Import "+filepath.Base(d.path), &d.opening, 980*a.uiScale, 640*a.uiScale) {
		return
	}

	a.drawImportImage()

	imgui.SameLine()
	imgui.BeginGroup()
	imgui.TextWrapped("Pick the colors of the image, and choose their terrains: every pixel takes the terrain of the closest picked color, which also cleans up antialiased edges.")
	imgui.Checkbox("Show the result", &d.result)

	imgui.SeparatorText("Picked colors")
	a.drawPicks()

	imgui.SeparatorText("Colors of the image")
	imgui.TextDisabled("Click one to pick it")
	a.drawImagePalette()

	imgui.Spacing()
	hasSea := slices.ContainsFunc(d.picks, func(p pick) bool { return p.terrain == config.SeaName })
	if !hasSea {
		imgui.TextColored(noticeColor, "Pick the color of the sea")
	}
	imgui.BeginDisabledV(!hasSea)
	if imgui.Button("Import") {
		imgui.CloseCurrentPopup()
		a.finishImport()
	}
	imgui.EndDisabled()
	imgui.SameLine()
	if imgui.Button("Cancel") {
		imgui.CloseCurrentPopup()
	}
	imgui.EndGroup()
	imgui.EndPopup()
}

// drawImportImage shows the image, or the result: click to pick a color.
func (a *app) drawImportImage() {
	d := &a.dialogs.imports
	if d.result && (d.previewResult != d.result || !slices.Equal(d.previewOf, d.picks)) {
		d.preview.uploadPixels(previewPixels(d.width, d.height, classify(d.pixels, d.picks, d.conf)))
		d.previewOf, d.previewResult = slices.Clone(d.picks), d.result
	}
	shown := &d.original
	if d.result {
		shown = &d.preview
	}
	side := 520 * a.uiScale
	scale := side / float32(max(shown.width, shown.height))
	size := imgui.NewVec2(float32(shown.width)*scale, float32(shown.height)*scale)
	origin := imgui.CursorScreenPos()
	imgui.Image(*imgui.NewTextureRefTextureID(imgui.TextureID(shown.id)), size)
	if !imgui.IsItemHovered() {
		return
	}
	mouse := imgui.CurrentIO().MousePos()
	x := int(float32(d.width) * (mouse.X - origin.X) / size.X)
	row := int(float32(d.height) * (mouse.Y - origin.Y) / size.Y)
	if x >= 0 && x < d.width && row >= 0 && row < d.height {
		color := d.pixels[(d.height-1-row)*d.width+x]
		imgui.SetTooltip(color.String() + "\nclick to pick this color")
		if imgui.IsMouseClickedBool(imgui.MouseButtonLeft) {
			d.addPick(color)
		}
	}
}

// drawPicks lists the picked colors, with their terrains.
func (a *app) drawPicks() {
	d := &a.dialogs.imports
	remove := -1
	for i := range d.picks {
		p := &d.picks[i]
		imgui.PushIDInt(int32(i))
		imgui.ColorButtonV("##c", colorVec(p.color), imgui.ColorEditFlagsNoTooltip, imgui.NewVec2(0, 0))
		imgui.SameLine()
		imgui.TextUnformatted(p.color.String() + " is")
		imgui.SameLine()
		imgui.SetNextItemWidth(160 * a.uiScale)
		if imgui.BeginCombo("##terrain", p.terrain) {
			for _, t := range d.conf.Terrains {
				if imgui.SelectableBoolV(t.Name, t.Name == p.terrain, imgui.SelectableFlagsNone, imgui.NewVec2(0, 0)) {
					p.terrain = t.Name
				}
			}
			imgui.EndCombo()
		}
		imgui.SameLine()
		if imgui.SmallButton("Remove") {
			remove = i
		}
		imgui.PopID()
	}
	if remove >= 0 {
		d.picks = slices.Delete(d.picks, remove, remove+1)
	}
}

// drawImagePalette shows the most common colors of the image, to pick.
func (a *app) drawImagePalette() {
	d := &a.dialogs.imports
	for i, c := range d.colors[:min(len(d.colors), paletteShown)] {
		if i%6 != 0 {
			imgui.SameLine()
		}
		imgui.PushIDInt(int32(1000 + i))
		if imgui.ColorButtonV("##p", colorVec(c.color), imgui.ColorEditFlagsNone, imgui.NewVec2(32*a.uiScale, 32*a.uiScale)) {
			d.addPick(c.color)
		}
		imgui.SetItemTooltip(noFormat(fmt.Sprintf("%s: %.1f%%", c.color, 100*float64(c.count)/float64(len(d.pixels)))))
		imgui.PopID()
	}
	if len(d.colors) > paletteShown {
		imgui.TextDisabled(fmt.Sprintf("and %d more", len(d.colors)-paletteShown))
	}
}

// finishImport starts a new project with the imported map.
func (a *app) finishImport() {
	d := &a.dialogs.imports
	pixels := classify(d.pixels, d.picks, d.conf)
	a.startProject(d.conf, newCanvas(d.width, d.height, pixels), withExt(d.path, projectExt))
}
