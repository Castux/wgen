// Package viewer is the interactive viewer: a window showing the generated
// terrain in 3D (orbit and top views) or as a 2D map, with a panel to edit
// the viewer settings and the generation parameters.
//
// It runs on the main thread (GLFW and OpenGL require it), and is redrawn on
// input, and when the engine or the background rendering have something new.
// Otherwise it sleeps.
package viewer

import (
	"fmt"
	"image"
	"image/png"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"

	"github.com/AllenDang/cimgui-go/imgui"
	implglfw "github.com/AllenDang/cimgui-go/impl/glfw"
	implgl "github.com/AllenDang/cimgui-go/impl/opengl3"
	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/go-gl/glfw/v3.4/glfw"
	"github.com/go-gl/mathgl/mgl64"

	"github.com/Castux/wgen/internal/config"
	"github.com/Castux/wgen/internal/engine"
	"github.com/Castux/wgen/internal/gen"
	"github.com/Castux/wgen/internal/render"
)

func init() {
	// GLFW must be used from the main thread
	runtime.LockOSThread()
}

type app struct {
	session *engine.Session
	window  *glfw.Window
	uiScale float32

	settingsPath string
	settings     Settings

	terrain *terrainView
	mapView *mapView
	overlay imageSlot // rivers, contours and grid, as a texture of the 3D views
	mapImg  imageSlot // the 2D map

	world   *gen.World // displayed
	version int

	message   *message // result of the last action
	exporting atomic.Bool
	results   chan message // of background actions

	// Panel state
	params     []param
	paramsConf *config.Config // params are of this config
	editKey    string         // slider being dragged
	editValue  float64

	// Input
	drag struct {
		active bool
		button imgui.MouseButton
	}
	keys     []shortcut // pressed since the last frame
	shiftTap bool       // Shift is down, and nothing else happened since

	wake   atomic.Bool // something happened in the background
	redraw int         // frames to draw before sleeping

	screenshot string // development: save a screenshot there once ready, and quit
}

type message struct {
	text  string
	error bool
}

// Frames drawn after an event, before sleeping: ImGui can take a couple of
// frames to settle.
const settleFrames = 3

// Run opens the viewer window, until it is closed.
func Run(session *engine.Session) error {
	if err := glfw.Init(); err != nil {
		return err
	}
	defer glfw.Terminate()

	glfw.WindowHint(glfw.ContextVersionMajor, 3)
	glfw.WindowHint(glfw.ContextVersionMinor, 3)
	glfw.WindowHint(glfw.OpenGLProfile, glfw.OpenGLCoreProfile)
	glfw.WindowHint(glfw.OpenGLForwardCompatible, glfw.True)
	glfw.WindowHint(glfw.Samples, 4)
	glfw.WindowHint(glfw.ScaleToMonitor, glfw.True)

	window, err := glfw.CreateWindow(1280, 800, "wgen - "+session.ConfigPath(), nil, nil)
	if err != nil {
		return err
	}
	defer window.Destroy()

	window.MakeContextCurrent()
	glfw.SwapInterval(1)
	if err := gl.Init(); err != nil {
		return fmt.Errorf("OpenGL: %w", err)
	}
	slog.Debug("OpenGL", "version", gl.GoStr(gl.GetString(gl.VERSION)), "renderer", gl.GoStr(gl.GetString(gl.RENDERER)))

	a := &app{
		session: session,
		window:  window,
		version: -1,
		results: make(chan message, 1),
		redraw:  settleFrames,

		screenshot: os.Getenv("WGEN_SCREENSHOT"),
	}
	a.overlay.wake = a.wakeUp
	a.mapImg.wake = a.wakeUp

	dir := settingsDir()
	if dir != "" {
		a.settingsPath = filepath.Join(dir, "viewer.json")
		a.settings = loadSettings(a.settingsPath)
	} else {
		a.settings = defaultSettings
	}

	// Installed before ImGui's, which call them in turn
	window.SetKeyCallback(func(_ *glfw.Window, key glfw.Key, scancode int, action glfw.Action, mods glfw.ModifierKey) {
		name := ""
		if action == glfw.Press {
			name = glfw.GetKeyName(key, scancode)
		}
		a.onKey(key, name, action, mods)
		a.activity()
	})
	window.SetMouseButtonCallback(func(*glfw.Window, glfw.MouseButton, glfw.Action, glfw.ModifierKey) {
		a.onMouse()
		a.activity()
	})
	window.SetCursorPosCallback(func(*glfw.Window, float64, float64) { a.activity() })
	window.SetScrollCallback(func(*glfw.Window, float64, float64) {
		a.onMouse()
		a.activity()
	})
	window.SetFocusCallback(func(*glfw.Window, bool) { a.activity() })
	window.SetSizeCallback(func(*glfw.Window, int, int) { a.activity() })
	window.SetRefreshCallback(func(*glfw.Window) { a.activity() })

	imgui.CreateContext()
	defer imgui.DestroyContext()
	a.setupImGui(dir)

	implglfw.InitForOpenGL(implglfw.NewGLFWwindowFromC(window.Handle()), true)
	defer implglfw.Shutdown()
	implgl.InitV("#version 330")
	defer implgl.Shutdown()

	if a.terrain, err = newTerrainView(); err != nil {
		return err
	}
	if a.mapView, err = newMapView(); err != nil {
		return err
	}

	states, cancel := session.Engine.Subscribe()
	defer cancel()
	go func() {
		for range states {
			a.wakeUp()
		}
	}()

	for !window.ShouldClose() {
		if a.redraw > 0 {
			glfw.PollEvents()
			a.redraw--
		} else {
			glfw.WaitEventsTimeout(0.5)
		}
		if a.wake.Swap(false) {
			a.activity()
		}
		a.frame()
	}

	return nil
}

func (a *app) setupImGui(settingsDir string) {
	io := imgui.CurrentIO()
	if settingsDir != "" {
		os.MkdirAll(settingsDir, 0o755)
		io.SetIniFilename(filepath.Join(settingsDir, "imgui.ini"))
	} else {
		io.SetIniFilename("")
	}

	imgui.StyleColorsDark()

	sx, _ := a.window.GetContentScale()
	a.uiScale = max(sx, 1)
	style := imgui.CurrentStyle()
	style.ScaleAllSizes(a.uiScale)
	style.SetFontScaleDpi(a.uiScale)
}

// activity requests drawing a few frames.
func (a *app) activity() { a.redraw = settleFrames }

// wakeUp wakes the main loop, from any goroutine.
func (a *app) wakeUp() {
	a.wake.Store(true)
	glfw.PostEmptyEvent()
}

// viewSize is the size of the window, in window coordinates (those of the
// mouse), and the number of framebuffer pixels per window unit.
func (a *app) viewSize() (width, height, pixelRatio float64) {
	w, h := a.window.GetSize()
	fw, _ := a.window.GetFramebufferSize()
	width, height = float64(max(w, 1)), float64(max(h, 1))
	return width, height, float64(max(fw, 1)) / width
}

func (a *app) frame() {
	implgl.NewFrame()
	implglfw.NewFrame()
	imgui.NewFrame()

	state := a.session.Engine.State()
	a.update()
	a.handleInput()
	a.drawPanel(state)
	a.drawStatus(state)
	imgui.Render()

	fw, fh := a.window.GetFramebufferSize()
	gl.Viewport(0, 0, int32(fw), int32(fh))
	width, height, ratio := a.viewSize()
	if a.settings.View == "map" {
		a.mapView.draw(width, height, ratio)
	} else {
		a.terrain.draw(&a.settings, width/height)
	}

	implgl.RenderDrawData(imgui.CurrentDrawData())

	if a.screenshot != "" && a.world != nil && !state.Busy && !a.loading() && a.redraw == 0 {
		a.saveScreenshot(fw, fh)
	}

	a.window.SwapBuffers()
}

// update shows the latest world and rendered images, and requests the images
// needed by the current settings.
func (a *app) update() {
	width, height, ratio := a.viewSize()

	if world, version := a.session.Engine.Snapshot(); world != nil && version != a.version {
		a.world, a.version = world, version
		a.message = nil
		if a.terrain.setMesh(world) {
			a.devCamera()
		}
		a.mapView.setSize(float64(world.Width), float64(world.Height), width, height)
	}

	if a.world != nil {
		o := a.settings.overlayOptions()
		o.Base = render.BaseNone
		o.Scale = a.terrain.overlayScale()
		a.overlay.request(a.world, a.version, o)

		// Only when visible, since it depends on the zoom
		if a.settings.View == "map" {
			o := a.settings.overlayOptions()
			o.Base = render.Base(a.settings.Color)
			o.Shading = a.settings.Shading == "lit"
			o.Scale = a.mapView.camera.imageScale(ratio)
			a.mapImg.request(a.world, a.version, o)
		}
	}

	if img := a.overlay.take(); img != nil {
		a.terrain.overlay.upload(img)
		a.terrain.hasOverlay = true
	}
	if img := a.mapImg.take(); img != nil {
		a.mapView.image.upload(img)
	}

	select {
	case m := <-a.results:
		a.message = &m
	default:
	}
}

// devCamera places the orbit camera as given by WGEN_CAMERA, for
// screenshots of close ups (see WGEN_SCREENSHOT): "x,y,distance,tilt,turn",
// the target in image pixels (top left origin), the tilt from vertical and
// the turn around it in degrees.
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
	c := &a.terrain.orbit
	c.target = mgl64.Vec3{x - a.terrain.width/2, a.terrain.height/2 - y, 0}
	c.radius, c.phi, c.theta = distance, tilt*math.Pi/180, turn*math.Pi/180
}

func (a *app) loading() bool { return a.overlay.busy() || a.mapImg.busy() }

func (a *app) setSettings(s Settings) {
	a.settings = s
	if a.settingsPath != "" {
		saveSettings(a.settingsPath, s)
	}
}

// Keyboard shortcuts
type shortcut int

const (
	shortcutView      shortcut = iota // Tab
	shortcutColor                     // Shift
	shortcutShading                   // q
	shortcutWireframe                 // w
)

// onKey turns key events into shortcuts.
//
// Shift is only a shortcut when tapped alone, since it is also a modifier:
// horizontal scrolling in the panel, panning in the orbit view. The other
// shortcuts are keys pressed without modifiers (Ctrl+Tab switches ImGui
// windows), not repeated when held, and q and w are recognized by name
// (glfw.GetKeyName, given for key presses), to follow the keyboard layout.
func (a *app) onKey(key glfw.Key, name string, action glfw.Action, mods glfw.ModifierKey) {
	shift := key == glfw.KeyLeftShift || key == glfw.KeyRightShift

	switch {
	case shift && action == glfw.Press:
		a.shiftTap = true

	case shift && action == glfw.Release:
		if a.shiftTap {
			a.keys = append(a.keys, shortcutColor)
		}
		a.shiftTap = false

	case action == glfw.Press:
		a.shiftTap = false
		if mods&(glfw.ModShift|glfw.ModControl|glfw.ModAlt|glfw.ModSuper) != 0 {
			return
		}
		switch {
		case key == glfw.KeyTab:
			a.keys = append(a.keys, shortcutView)
		case name == "q":
			a.keys = append(a.keys, shortcutShading)
		case name == "w":
			a.keys = append(a.keys, shortcutWireframe)
		}
	}
}

// onMouse is called on mouse buttons and wheel: Shift is being used as a
// modifier.
func (a *app) onMouse() { a.shiftTap = false }

func (a *app) handleInput() {
	io := imgui.CurrentIO()

	// Not while typing in a field or using a widget, nor while a combo is
	// open, where they would change its value behind it
	popup := imgui.IsPopupOpenStrV("", imgui.PopupFlagsAnyPopupId|imgui.PopupFlagsAnyPopupLevel)
	if !io.WantCaptureKeyboard() && !popup {
		s := a.settings
		for _, k := range a.keys {
			switch k {
			case shortcutView:
				s.View = cycle(views, s.View)
			case shortcutColor:
				s.Color = cycle(colors, s.Color)
			case shortcutShading:
				s.Shading = cycle(shadings, s.Shading)
			case shortcutWireframe:
				s.Wireframe = !s.Wireframe
			}
		}
		if s != a.settings {
			a.setSettings(s)
		}
	}
	a.keys = a.keys[:0]

	// A drag belongs to the view if it started outside of the panel
	if !a.drag.active && !io.WantCaptureMouse() {
		for _, b := range []imgui.MouseButton{imgui.MouseButtonLeft, imgui.MouseButtonRight, imgui.MouseButtonMiddle} {
			if imgui.IsMouseClickedBool(b) {
				a.drag.active, a.drag.button = true, b
				break
			}
		}
	}
	if a.drag.active && !imgui.IsMouseDown(a.drag.button) {
		a.drag.active = false
	}

	width, height, _ := a.viewSize()
	if d := io.MouseDelta(); a.drag.active && (d.X != 0 || d.Y != 0) {
		a.dragView(float64(d.X), float64(d.Y), width, height, io.KeyShift() || io.KeyCtrl() || io.KeySuper())
	}

	if io.WantCaptureMouse() && !a.drag.active {
		return
	}
	if wheel := float64(io.MouseWheel()); wheel != 0 {
		a.wheelView(wheel, width, height)
	}
	if a.settings.View == "map" {
		if imgui.IsMouseDoubleClicked(imgui.MouseButtonLeft) {
			a.mapView.camera.fit(width, height)
		}
		imgui.SetMouseCursor(imgui.MouseCursorHand)
	}
}

// dragView moves the camera of the current view for a mouse drag.
//
//   - orbit: left rotates (pans with shift, ctrl or cmd), right pans, middle
//     zooms
//   - top: left and right pan, middle zooms
//   - map: every button pans
func (a *app) dragView(dx, dy, width, height float64, modifier bool) {
	// Drag zoom: 0.95 per 100 pixels, zooming out when dragging down
	zoom := math.Pow(0.95, -dy/100)

	switch a.settings.View {
	case "orbit":
		c := &a.terrain.orbit
		switch {
		case a.drag.button == imgui.MouseButtonMiddle:
			c.dolly(zoom)
		case a.drag.button == imgui.MouseButtonRight || modifier:
			c.pan(dx, dy, height)
		default:
			c.rotate(dx, dy, height)
		}

	case "top":
		c := &a.terrain.top
		if a.drag.button == imgui.MouseButtonMiddle {
			c.dolly(zoom)
		} else {
			c.pan(dx, dy, width, height)
		}

	case "map":
		c := &a.mapView.camera
		c.offset[0] += dx
		c.offset[1] += dy
	}
}

// wheelView zooms the current view for a wheel movement (positive when
// scrolling up, one per notch).
func (a *app) wheelView(wheel, width, height float64) {
	switch a.settings.View {
	case "orbit":
		a.terrain.orbit.dolly(math.Pow(0.95, wheel))
	case "top":
		a.terrain.top.dolly(math.Pow(0.95, wheel))
	case "map":
		p := imgui.CurrentIO().MousePos()
		a.mapView.camera.zoomAt(float64(p.X), float64(p.Y), math.Exp(0.2*wheel), width, height)
	}
}

func (a *app) resetView() {
	if a.settings.View == "map" {
		width, height, _ := a.viewSize()
		a.mapView.camera.fit(width, height)
	} else {
		a.terrain.resetCameras()
	}
}

func (a *app) save() {
	a.message = &message{text: "Config saved"}
	if _, err := a.session.Save(); err != nil {
		a.message = &message{text: err.Error(), error: true}
	}
}

// export writes the exports in the background.
func (a *app) export() {
	a.exporting.Store(true)
	go func() {
		defer a.wakeUp()
		defer a.exporting.Store(false)

		files, err := a.session.Export()
		switch {
		case err != nil:
			a.results <- message{text: err.Error(), error: true}
		case len(files) == 0:
			a.results <- message{text: "Exported nothing (all exports disabled)"}
		default:
			a.results <- message{text: "Exported " + strings.Join(files, ", ")}
		}
	}()
}

var (
	busyColor  = imgui.NewVec4(1, 0.88, 0.51, 1)
	errorColor = imgui.NewVec4(1, 0.54, 0.5, 1)
	dirtyColor = imgui.NewVec4(0.56, 0.79, 0.98, 1)
	textColor  = imgui.NewVec4(0.93, 0.93, 0.93, 1)
)

// drawStatus shows the engine state and the result of the last action, in
// the bottom left corner.
func (a *app) drawStatus(state engine.State) {
	type line struct {
		text  string
		color imgui.Vec4
	}
	var lines []line

	switch {
	case state.Busy:
		lines = append(lines, line{"Generating…", busyColor})
	case a.loading():
		lines = append(lines, line{"Loading…", busyColor})
	}
	if a.exporting.Load() {
		lines = append(lines, line{"Exporting…", busyColor})
	}
	if state.Error != "" {
		lines = append(lines, line{state.Error, errorColor})
	}
	if state.Dirty {
		lines = append(lines, line{"Unsaved config changes", dirtyColor})
	}
	if m := a.message; m != nil {
		color := textColor
		if m.error {
			color = errorColor
		}
		lines = append(lines, line{m.text, color})
	}

	if len(lines) == 0 {
		return
	}

	display := imgui.CurrentIO().DisplaySize()
	imgui.SetNextWindowPosV(imgui.NewVec2(8, display.Y-8), imgui.CondAlways, imgui.NewVec2(0, 1))
	imgui.SetNextWindowBgAlpha(0.6)
	flags := imgui.WindowFlagsNoDecoration | imgui.WindowFlagsAlwaysAutoResize | imgui.WindowFlagsNoSavedSettings |
		imgui.WindowFlagsNoFocusOnAppearing | imgui.WindowFlagsNoNav | imgui.WindowFlagsNoInputs

	if imgui.BeginV("##status", nil, flags) {
		imgui.PushTextWrapPosV(display.X * 0.6)
		for _, l := range lines {
			imgui.PushStyleColorVec4(imgui.ColText, l.color)
			imgui.TextUnformatted(l.text)
			imgui.PopStyleColor()
		}
		imgui.PopTextWrapPos()
	}
	imgui.End()
}

// saveScreenshot writes the framebuffer to a.screenshot and closes the
// window: a development aid, to check the viewer without looking at it.
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

	f, err := os.Create(a.screenshot)
	if err == nil {
		err = png.Encode(f, img)
		f.Close()
	}
	if err != nil {
		slog.Error("screenshot", "err", err)
	} else {
		slog.Info("screenshot saved", "path", a.screenshot)
	}
	a.screenshot = ""
	a.window.SetShouldClose(true)
}
