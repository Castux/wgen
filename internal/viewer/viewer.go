// Package viewer is the app: a window showing the generated landscape in 3D
// or as a 2D map, where the map is painted, with a panel to edit the terrains
// and the parameters, and menus for projects (new, open, save, export).
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
	"strconv"
	"sync/atomic"
	"time"

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
	editor  editor
	dialogs dialogs

	menuHeight    float32
	title         string
	suggestedPath string    // where to save a new project, by default
	fitMap        bool      // fit the map view to the next world
	quitting      bool      // confirmed
	overlay       imageSlot // rivers, contours and grid, as a texture of the 3D views
	mapImg        imageSlot // the 2D map

	world   *gen.World // displayed
	version int

	message   *message // result of the last action
	exporting atomic.Bool
	results   chan message // of background actions

	// Panel state
	params     []param
	paramsConf *config.Config // params are of this config
	editKey    string         // widget being edited
	editValue  float64
	editText   string
	editColor  [3]float32

	// Input
	drag struct {
		active bool
		button imgui.MouseButton
	}
	keys     []shortcut // pressed since the last frame
	shiftTap bool       // Shift is down, and nothing else happened since

	wake   atomic.Bool // something happened in the background
	redraw int         // frames to draw before sleeping

	screenshot   string        // development: save a screenshot there once ready, and quit
	screenshotAt time.Duration // or at that time
	start        time.Time
}

type message struct {
	text  string
	error bool
}

// Frames drawn after an event, before sleeping: ImGui can take a couple of
// frames to settle.
const settleFrames = 3

// Run opens the viewer window, until it is closed.
func Run(session *engine.Session, path string) error {
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

	window, err := glfw.CreateWindow(1280, 800, "wgen", nil, nil)
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
		start:      time.Now(),
	}
	if at, err := strconv.ParseFloat(os.Getenv("WGEN_SCREENSHOT_AT"), 64); err == nil {
		a.screenshotAt = time.Duration(at * float64(time.Second))
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

	// Closing asks about unsaved changes
	window.SetCloseCallback(func(w *glfw.Window) {
		if !a.quitting && a.unsaved() {
			w.SetShouldClose(false)
			a.quit()
		}
		a.activity()
	})
	// Files dropped on the window are opened
	window.SetDropCallback(func(_ *glfw.Window, names []string) {
		if len(names) > 0 {
			path := names[0]
			a.unsavedThen("open another map", func() { a.open(path) })
		}
		a.activity()
	})

	imgui.CreateContext()
	defer imgui.DestroyContext()
	a.setupImGui(dir)

	implglfw.InitForOpenGL(implglfw.NewGLFWwindowFromC(window.Handle()), true)
	defer implglfw.Shutdown()
	implgl.InitV("#version 330")
	defer implgl.Shutdown()

	a.applyWatch()
	a.startup(path)

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
	a.drawMenu(state)
	a.drawPanel(state)
	a.drawStatus(state)
	a.drawLegend()
	a.drawDialogs()
	imgui.Render()
	a.updateTitle()

	fw, fh := a.window.GetFramebufferSize()
	gl.Viewport(0, 0, int32(fw), int32(fh))
	width, height, ratio := a.viewSize()
	a.updatePaintTexture()
	if a.settings.View == "map" {
		a.mapView.draw(width, height, ratio)
	} else {
		a.terrain.draw(&a.settings, width/height)
	}

	implgl.RenderDrawData(imgui.CurrentDrawData())

	ready := a.world != nil && !state.Busy && !a.loading() && a.redraw == 0
	if a.screenshotAt > 0 {
		ready = time.Since(a.start) > a.screenshotAt
		a.activity() // keep drawing until then
	}
	if a.screenshot != "" && ready {
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
		a.syncCanvas(world)
		if a.terrain.setMesh(world) || a.fitMap {
			a.terrain.resetCameras()
			a.devCamera()
		}
		a.mapView.setSize(float64(world.Width), float64(world.Height), width, height)
		if a.fitMap {
			a.mapView.camera.fit(width, height)
			a.fitMap = false
		}
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

// applyWatch tells the engine whether to show the simulation as it runs.
func (a *app) applyWatch() {
	steps := 0
	if a.settings.Watch {
		steps = max(1, int(a.settings.WatchSteps))
	}
	a.session.Engine.SetWatch(steps)
}

func (a *app) loading() bool { return a.overlay.busy() || a.mapImg.busy() }

// updateTitle shows the project in the window title, with a star if it has
// unsaved changes.
func (a *app) updateTitle() {
	title := a.projectName() + " - wgen"
	if a.unsaved() {
		title = "*" + title
	}
	if title != a.title {
		a.title = title
		a.window.SetTitle(title)
	}
}

func (a *app) setSettings(s Settings) {
	watch := s.Watch != a.settings.Watch || s.WatchSteps != a.settings.WatchSteps
	a.settings = s
	if watch {
		a.applyWatch()
	}
	if a.settingsPath != "" {
		saveSettings(a.settingsPath, s)
	}
}

// Keyboard shortcuts
type shortcut int

const (
	shortcutView      shortcut = iota // v
	shortcutColor                     // Shift
	shortcutShading                   // q
	shortcutWireframe                 // w
	shortcutEdit                      // e
	shortcutLockShore                 // l
	shortcutSmaller                   // [, brush
	shortcutLarger                    // ]
	shortcutUndo                      // ctrl+z
	shortcutRedo                      // ctrl+y, ctrl+shift+z
	shortcutSave                      // ctrl+s
	shortcutSaveAs                    // ctrl+shift+s
	shortcutNew                       // ctrl+n
	shortcutOpen                      // ctrl+o
	shortcutExport                    // ctrl+e
	shortcutQuit                      // ctrl+q
	shortcutReset                     // r
)

// onKey turns key events into shortcuts.
//
// Shift is only a shortcut when tapped alone, since it is also a modifier:
// horizontal scrolling in the panel, panning in the orbit view. The other
// shortcuts are keys pressed without modifiers (Tab is ImGui's, to move
// between fields, Ctrl+Tab between windows), not repeated when held, and
// letters are recognized by name (glfw.GetKeyName, given for key presses),
// to follow the keyboard layout. The menu commands are with Ctrl (or Cmd).
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
		if ctrl := mods&(glfw.ModControl|glfw.ModSuper) != 0; ctrl && mods&glfw.ModAlt == 0 {
			shift := mods&glfw.ModShift != 0
			switch {
			case name == "z" && shift, name == "y":
				a.keys = append(a.keys, shortcutRedo)
			case name == "z":
				a.keys = append(a.keys, shortcutUndo)
			case name == "s" && shift:
				a.keys = append(a.keys, shortcutSaveAs)
			case name == "s":
				a.keys = append(a.keys, shortcutSave)
			case name == "n":
				a.keys = append(a.keys, shortcutNew)
			case name == "o":
				a.keys = append(a.keys, shortcutOpen)
			case name == "e":
				a.keys = append(a.keys, shortcutExport)
			case name == "q":
				a.keys = append(a.keys, shortcutQuit)
			}
			return
		}
		if mods&(glfw.ModShift|glfw.ModControl|glfw.ModAlt|glfw.ModSuper) != 0 {
			return
		}
		switch {
		case name == "v":
			a.keys = append(a.keys, shortcutView)
		case name == "r":
			a.keys = append(a.keys, shortcutReset)
		case name == "q":
			a.keys = append(a.keys, shortcutShading)
		case name == "w":
			a.keys = append(a.keys, shortcutWireframe)
		case name == "e":
			a.keys = append(a.keys, shortcutEdit)
		case name == "l":
			a.keys = append(a.keys, shortcutLockShore)
		case name == "[":
			a.keys = append(a.keys, shortcutSmaller)
		case name == "]":
			a.keys = append(a.keys, shortcutLarger)
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
			case shortcutEdit:
				s.Editing = !s.Editing
				if s.Editing {
					s.View = "map"
				}
			case shortcutLockShore:
				s.LockShore = !s.LockShore
			case shortcutSmaller:
				s.BrushRadius = math.Max(1, math.Round(s.BrushRadius/1.25))
			case shortcutLarger:
				s.BrushRadius = math.Min(500, math.Round(s.BrushRadius*1.25+0.5))
			case shortcutUndo:
				a.undo()
			case shortcutRedo:
				a.redo()
			case shortcutSave:
				a.saveProject(nil)
			case shortcutSaveAs:
				a.saveProjectAs(nil)
			case shortcutNew:
				a.newDialog()
			case shortcutOpen:
				a.openDialog()
			case shortcutExport:
				if a.world != nil {
					a.openExport()
				}
			case shortcutQuit:
				a.quit()
			case shortcutReset:
				a.resetView()
			}
		}
		if s != a.settings {
			a.setSettings(s)
		}
	}
	a.keys = a.keys[:0]

	// Painting takes the left button
	painting := a.settings.Editing && a.settings.View == "map"
	if a.paintInput() {
		return
	}

	// A drag belongs to the view if it started outside of the panel
	if !a.drag.active && !io.WantCaptureMouse() {
		for _, b := range []imgui.MouseButton{imgui.MouseButtonLeft, imgui.MouseButtonRight, imgui.MouseButtonMiddle} {
			if painting && b == imgui.MouseButtonLeft {
				continue
			}
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
	if a.settings.View == "map" && !painting {
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
	case state.Busy && state.Progress != "":
		lines = append(lines, line{state.Progress, busyColor})
	case state.Busy && state.Preview:
		lines = append(lines, line{"Refining...", busyColor})
	case state.Busy:
		lines = append(lines, line{"Generating...", busyColor})
	case a.loading():
		lines = append(lines, line{"Loading...", busyColor})
	}
	if a.exporting.Load() {
		lines = append(lines, line{"Exporting...", busyColor})
	}
	if state.Error != "" {
		lines = append(lines, line{state.Error, errorColor})
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
	imgui.SetNextWindowSizeConstraints(imgui.NewVec2(0, 0), imgui.NewVec2(display.X*0.6, display.Y/2))
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
