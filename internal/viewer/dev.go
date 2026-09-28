package viewer

import (
	"fmt"
	"image"
	"image/png"
	"log/slog"
	"math"
	"os"
	"strconv"
	"time"

	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/go-gl/mathgl/mgl64"

	"github.com/Castux/wgen/internal/engine"
)

// Development aids, to check the viewer without looking at it, set by
// environment variables:
//
//   - WGEN_SCREENSHOT=path: save a screenshot once the world and its images
//     are ready, and quit
//   - WGEN_SCREENSHOT_AT=seconds: take it at that time instead
//   - WGEN_CAMERA=x,y,distance,tilt,turn: place the orbit camera
//   - WGEN_DIALOG=new|open|export|help: open that dialog at startup

type devHooks struct {
	screenshot   string        // save a screenshot there once ready, and quit
	screenshotAt time.Duration // or at that time
}

func loadDevHooks() devHooks {
	d := devHooks{screenshot: os.Getenv("WGEN_SCREENSHOT")}
	if at, err := strconv.ParseFloat(os.Getenv("WGEN_SCREENSHOT_AT"), 64); err == nil {
		d.screenshotAt = time.Duration(at * float64(time.Second))
	}
	return d
}

// devDialog opens the dialog given by WGEN_DIALOG, for screenshots.
func (a *app) devDialog() {
	switch os.Getenv("WGEN_DIALOG") {
	case "new":
		a.openNewMap()
	case "open":
		a.openFile("Open a project or an image", "", "", false, openExts, a.open)
	case "export":
		a.openExport()
	case "help":
		a.dialogs.help = true
	}
}

// devCamera places the orbit camera as given by WGEN_CAMERA, for
// screenshots of close ups: "x,y,distance,tilt,turn", the target in image
// pixels (top left origin), the tilt from vertical and the turn around it
// in degrees.
func (a *app) devCamera() {
	spec := os.Getenv("WGEN_CAMERA")
	if spec == "" {
		return
	}
	var x, y, distance, tilt, turn float64
	if _, err := fmt.Sscanf(spec, "%g,%g,%g,%g,%g", &x, &y, &distance, &tilt, &turn); err != nil {
		slog.Warn("bad WGEN_CAMERA", "value", spec, "err", err)
		return
	}
	camera := &a.terrain.orbit
	camera.target = mgl64.Vec3{x - a.terrain.width/2, a.terrain.height/2 - y, 0}
	camera.radius, camera.phi, camera.theta = distance, tilt*math.Pi/180, turn*math.Pi/180
}

// devScreenshot saves the screenshot asked by WGEN_SCREENSHOT when it is
// time: once everything is drawn, or at WGEN_SCREENSHOT_AT.
func (a *app) devScreenshot(state engine.State, framebufferWidth, framebufferHeight int) {
	ready := a.world != nil && !state.Busy && !a.loading() && a.redraw == 0
	if a.dev.screenshotAt > 0 {
		ready = time.Since(a.start) > a.dev.screenshotAt
		a.activity() // keep drawing until then
	}
	if a.dev.screenshot != "" && ready {
		a.saveScreenshot(framebufferWidth, framebufferHeight)
	}
}

// saveScreenshot writes the framebuffer to the screenshot path and closes
// the window.
func (a *app) saveScreenshot(width, height int) {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	gl.PixelStorei(gl.PACK_ALIGNMENT, 1)
	gl.ReadPixels(0, 0, int32(width), int32(height), gl.RGBA, gl.UNSIGNED_BYTE, gl.Ptr(img.Pix))

	// OpenGL rows are bottom first
	row := make([]byte, img.Stride)
	for y := range height / 2 {
		top, bottom := img.Pix[y*img.Stride:(y+1)*img.Stride], img.Pix[(height-1-y)*img.Stride:(height-y)*img.Stride]
		copy(row, top)
		copy(top, bottom)
		copy(bottom, row)
	}
	for i := 3; i < len(img.Pix); i += 4 {
		img.Pix[i] = 255
	}

	path := a.dev.screenshot
	f, err := os.Create(path)
	if err == nil {
		err = png.Encode(f, img)
		f.Close()
	}
	if err != nil {
		slog.Error("screenshot", "err", err)
	} else {
		slog.Info("screenshot saved", "path", path)
	}
	a.dev.screenshot = ""
	a.window.SetShouldClose(true)
}
