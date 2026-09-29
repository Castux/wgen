package app

import (
	"fmt"
	"image"
	"image/png"
	"log/slog"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/AllenDang/cimgui-go/imgui"
	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/go-gl/mathgl/mgl64"

	"github.com/Castux/wgen/internal/engine"
)

// Development aids, to check the app without looking at it, set by
// environment variables:
//
//   - WGEN_SCREENSHOT=path: save a screenshot once the world and its images
//     are ready, and quit
//   - WGEN_SCREENSHOT_AT=seconds: take it at that time instead
//   - WGEN_CAMERA=x,y,distance,tilt,turn: place the orbit camera
//   - WGEN_DIALOG=new|open|export|help|welcome: open that dialog at startup
//   - WGEN_AUTOSAVE=seconds: the interval of autosaves, instead of a minute
//   - WGEN_CLICK=x,y;x,y...: click there (window coordinates) once the
//     world is ready, one click every 10 frames (the same point twice is a
//     double click), to open a menu or draw a ruler; the screenshot waits
//     for them

type devHooks struct {
	screenshot   string        // save a screenshot there once ready, and quit
	screenshotAt time.Duration // or at that time

	autosaveSeconds float64

	clicks      []imgui.Vec2 // where to click, in turn
	clicksStart time.Time    // once the world is ready
	clickFrame  int          // frames since the clicks started
}

func loadDevHooks() devHooks {
	d := devHooks{screenshot: os.Getenv("WGEN_SCREENSHOT")}
	if at, err := strconv.ParseFloat(os.Getenv("WGEN_SCREENSHOT_AT"), 64); err == nil {
		d.screenshotAt = time.Duration(at * float64(time.Second))
	}
	d.autosaveSeconds, _ = strconv.ParseFloat(os.Getenv("WGEN_AUTOSAVE"), 64)
	for _, spec := range strings.Split(os.Getenv("WGEN_CLICK"), ";") {
		var x, y float32
		if _, err := fmt.Sscanf(spec, "%g,%g", &x, &y); err == nil {
			d.clicks = append(d.clicks, imgui.Vec2{X: x, Y: y})
		}
	}
	return d
}

// Frames between the clicks of WGEN_CLICK
const framesPerClick = 10

// devClick clicks where WGEN_CLICK says, once the world is ready: for each
// point, the mouse moves there, the button goes down the next frame and up
// the one after, then the mouse stays there. Called before ImGui's frame starts.
func (a *app) devClick() {
	if len(a.dev.clicks) == 0 {
		return
	}
	if a.devClicksDone() {
		last := a.dev.clicks[len(a.dev.clicks)-1]
		imgui.CurrentIO().AddMousePosEvent(last.X, last.Y)
		return
	}
	a.activity()
	if a.dev.clicksStart.IsZero() {
		if a.world != nil && !a.session.Engine.State().Busy {
			a.dev.clicksStart = time.Now()
		}
		return
	}
	if time.Since(a.dev.clicksStart) < time.Second/2 {
		return
	}
	click := min(a.dev.clickFrame/framesPerClick, len(a.dev.clicks)-1)
	io := imgui.CurrentIO()
	io.AddMousePosEvent(a.dev.clicks[click].X, a.dev.clicks[click].Y)
	if a.dev.clickFrame < framesPerClick*len(a.dev.clicks) {
		// The mouse moves first: a move with the button down is a drag
		switch a.dev.clickFrame % framesPerClick {
		case 1:
			io.AddMouseButtonEvent(int32(imgui.MouseButtonLeft), true)
		case 2:
			io.AddMouseButtonEvent(int32(imgui.MouseButtonLeft), false)
		}
	}
	a.dev.clickFrame++
}

// devClicksDone tells whether the clicks of WGEN_CLICK are done, and shown.
func (a *app) devClicksDone() bool {
	return len(a.dev.clicks) == 0 || a.dev.clickFrame >= framesPerClick*(len(a.dev.clicks)+1)
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
	case "welcome":
		a.openWelcome()
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
	a.terrain.eye.fromOrbit(camera)
}

// devScreenshot saves the screenshot asked by WGEN_SCREENSHOT when it is
// time: once everything is drawn, or at WGEN_SCREENSHOT_AT.
func (a *app) devScreenshot(state engine.State, framebufferWidth, framebufferHeight int) {
	ready := a.world != nil && !state.Busy && !a.loading() && a.redraw == 0 && a.devClicksDone()
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
