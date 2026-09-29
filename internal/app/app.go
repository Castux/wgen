// Package app is the wgen app: a window showing the generated landscape in 3D
// or as a 2D map, where the map is painted, with a panel to edit the terrains
// and the parameters, and menus for projects (new, open, save, export).
//
// It runs on the main thread (GLFW and OpenGL require it), and is redrawn on
// input, and when the engine or the background rendering have something new.
// Otherwise it sleeps.
package app

import (
	"fmt"
	"image"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/AllenDang/cimgui-go/imgui"
	implglfw "github.com/AllenDang/cimgui-go/impl/glfw"
	implgl "github.com/AllenDang/cimgui-go/impl/opengl3"
	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/go-gl/glfw/v3.4/glfw"
	"golang.org/x/image/draw"

	"github.com/Castux/wgen/assets"
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
	start   time.Time

	settingsPath string
	settings     Settings
	recentPath   string
	recent       []string // projects, most recent first

	terrain  *terrainView
	mapView  *mapView
	editor   editor
	inspect  inspector
	dialogs  dialogs
	autosave autosaver

	menuHeight    float32
	title         string
	suggestedPath string    // where to save a new project, by default
	fitMap        bool      // fit the map view to the next world
	quitting      bool      // confirmed
	overlay       imageSlot // rivers, contours and grid, as a texture of the 3D view
	mapImage      imageSlot // the 2D map

	world   *gen.World // displayed
	version int

	message   *message // result of the last action
	exporting atomic.Bool
	results   chan message // of background actions

	// Panel state
	params     []param
	paramsConf *config.Config // params are of this config
	widget     widgetEdit     // being edited

	input input

	wake   atomic.Bool // something happened in the background
	redraw int         // frames to draw before sleeping

	dev devHooks
}

// Version is the app's version, shown in the Help menu.
var Version = "dev"

// Frames drawn after an event, before sleeping: ImGui can take a couple of
// frames to settle.
const settleFrames = 3

// Run opens the app window, until it is closed.
func Run(session *engine.Session, path string) error {
	// Paths are relative to where the app was started (on macOS, GLFW would
	// move to the bundle's resources)
	if path != "" {
		if abs, err := filepath.Abs(path); err == nil {
			path = abs
		}
	}
	glfw.InitHint(glfw.CocoaChdirResources, glfw.False)
	if err := glfw.Init(); err != nil {
		return err
	}
	defer glfw.Terminate()

	window, err := createWindow()
	if err != nil {
		return err
	}
	defer window.Destroy()

	a := &app{
		session: session,
		window:  window,
		start:   time.Now(),
		version: -1,
		results: make(chan message, 1),
		inspect: inspector{selected: -1},
		redraw:  settleFrames,
		dev:     loadDevHooks(),
	}
	a.overlay.wake = a.wakeUp
	a.mapImage.wake = a.wakeUp

	dir := settingsDir()
	a.initSettings(dir)
	a.setCallbacks()

	imgui.CreateContext()
	defer imgui.DestroyContext()
	a.setupImGui(dir)

	implglfw.InitForOpenGL(implglfw.NewGLFWwindowFromC(window.Handle()), true)
	defer implglfw.Shutdown()
	implgl.InitV("#version 330")
	defer implgl.Shutdown()

	a.applyWatch()
	a.startup(path)
	a.initAutosave(dir)
	a.devDialog()

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

	a.loop()

	// Quitting was confirmed: the recovery copy is no longer needed
	if a.autosave.written.Load() {
		a.removeRecovery()
	}
	return nil
}

// createWindow opens the window, with an OpenGL 3.3 core context.
func createWindow() (*glfw.Window, error) {
	glfw.WindowHint(glfw.ContextVersionMajor, 3)
	glfw.WindowHint(glfw.ContextVersionMinor, 3)
	glfw.WindowHint(glfw.OpenGLProfile, glfw.OpenGLCoreProfile)
	glfw.WindowHint(glfw.OpenGLForwardCompatible, glfw.True)
	glfw.WindowHint(glfw.Samples, 4)
	glfw.WindowHint(glfw.ScaleToMonitor, glfw.True)

	window, err := glfw.CreateWindow(1280, 800, "wgen", nil, nil)
	if err != nil {
		return nil, err
	}
	window.SetIcon(icons()) // Windows and Linux; macOS uses the bundle's

	window.MakeContextCurrent()
	glfw.SwapInterval(1)
	if err := gl.Init(); err != nil {
		window.Destroy()
		return nil, fmt.Errorf("OpenGL: %w", err)
	}
	slog.Debug("OpenGL", "version", gl.GoStr(gl.GetString(gl.VERSION)), "renderer", gl.GoStr(gl.GetString(gl.RENDERER)))
	return window, nil
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

	contentScale, _ := a.window.GetContentScale()
	a.uiScale = max(contentScale, 1)
	if runtime.GOOS == "darwin" {
		// Window sizes are in points on macOS, scaled by the system
		a.uiScale = 1
	}
	style := imgui.CurrentStyle()
	style.ScaleAllSizes(a.uiScale)
	style.SetFontScaleDpi(a.uiScale)
}

// icons is the app icon at the usual window icon sizes.
func icons() []image.Image {
	src := assets.Icon()
	var images []image.Image
	for _, size := range []int{16, 32, 48, 64, 128} {
		img := image.NewNRGBA(image.Rect(0, 0, size, size))
		draw.CatmullRom.Scale(img, img.Bounds(), src, src.Bounds(), draw.Src, nil)
		images = append(images, img)
	}
	return images
}

// loop draws frames until the window is closed: a few after any input or
// background event, then it sleeps until the next one.
func (a *app) loop() {
	for !a.window.ShouldClose() {
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
}

// activity requests drawing a few frames.
func (a *app) activity() { a.redraw = settleFrames }

// wakeUp wakes the main loop, from any goroutine.
func (a *app) wakeUp() {
	a.wake.Store(true)
	glfw.PostEmptyEvent()
}

// viewSize is the size of the view, the window left of the panel, in
// window coordinates (those of the mouse), and the number of framebuffer
// pixels per window unit.
func (a *app) viewSize() (width, height, pixelRatio float64) {
	windowWidth, windowHeight := a.window.GetSize()
	framebufferWidth, _ := a.window.GetFramebufferSize()
	pixelRatio = float64(max(framebufferWidth, 1)) / float64(max(windowWidth, 1))
	width = math.Max(float64(windowWidth)-float64(a.panelWidth()), 1)
	return width, float64(max(windowHeight, 1)), pixelRatio
}

func (a *app) frame() {
	implgl.NewFrame()
	implglfw.NewFrame()
	a.devClick()
	imgui.NewFrame()

	state := a.session.Engine.State()
	a.update()
	a.updateInspector()
	a.handleInput()
	a.drawMenu(state)
	a.drawPanel()
	a.drawStatus(state)
	a.drawLegend()
	a.drawHover()
	a.drawProfile()
	a.drawRulers()
	a.drawDialogs()
	if !imgui.IsAnyItemActive() {
		// An edit left without committing, as a closed menu's
		a.widget.key = ""
	}
	imgui.Render()
	a.updateTitle()
	a.autosaveTick()

	framebufferWidth, framebufferHeight := a.window.GetFramebufferSize()
	width, height, ratio := a.viewSize()
	gl.Viewport(0, 0, int32(math.Round(width*ratio)), int32(framebufferHeight))
	a.updatePaintTexture()
	a.mapView.highlight, a.terrain.highlight = nil, nil
	if a.inspect.basinVisible {
		a.mapView.highlight, a.terrain.highlight = &a.inspect.basinMask, &a.inspect.basinMask
	}
	if a.settings.View == viewMap {
		a.mapView.draw(width, height, ratio)
	} else {
		a.terrain.draw(&a.settings, width/height)
	}

	implgl.RenderDrawData(imgui.CurrentDrawData())
	a.devScreenshot(state, framebufferWidth, framebufferHeight)

	a.window.SwapBuffers()
}

// update shows the latest world and rendered images, and requests the images
// needed by the current settings.
func (a *app) update() {
	width, height, ratio := a.viewSize()

	if world, version := a.session.Engine.Snapshot(); world != nil && version != a.version {
		a.showWorld(world, version, width, height)
	}

	if a.world != nil {
		a.requestImages(ratio)
	}
	if img := a.overlay.take(); img != nil {
		a.terrain.overlay.upload(img)
		a.terrain.hasOverlay = true
	}
	if img := a.mapImage.take(); img != nil {
		a.mapView.image.upload(img)
	}

	select {
	case m := <-a.results:
		a.message = &m
	default:
	}
}

// showWorld displays a new world, in a view of the given size.
func (a *app) showWorld(world *gen.World, version int, width, height float64) {
	a.world, a.version = world, version
	a.message = nil
	a.syncCanvas(world)
	if a.terrain.setMesh(world) || a.fitMap {
		a.terrain.resetCamera()
		a.devCamera()
	}
	a.mapView.setSize(float64(world.Width), float64(world.Height), width, height)
	if a.fitMap {
		a.mapView.camera.fit(width, height)
		a.fitMap = false
	}
}

// requestImages asks for the rendered images the current settings need.
func (a *app) requestImages(pixelRatio float64) {
	options := a.settings.overlayOptions()
	options.Base = render.BaseNone
	options.Scale = a.terrain.overlayScale()
	a.overlay.request(a.world, a.version, options)

	// Only when visible, since it depends on the zoom
	if a.settings.View == viewMap {
		options := a.settings.overlayOptions()
		options.Base = render.Base(a.settings.Color)
		options.Shading = a.settings.Shading == shadingLit
		options.Scale = a.mapView.camera.imageScale(pixelRatio)
		a.mapImage.request(a.world, a.version, options)
	}
}

func (a *app) loading() bool { return a.overlay.busy() || a.mapImage.busy() }

// updateTitle shows the project in the window title, with a star if it has
// unsaved changes.
func (a *app) updateTitle() {
	title := "wgen"
	if a.hasProject() {
		title = a.projectName() + " - wgen"
	}
	if a.unsaved() {
		title = "*" + title
	}
	if title != a.title {
		a.title = title
		a.window.SetTitle(title)
	}
}
