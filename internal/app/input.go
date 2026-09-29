package app

import (
	"math"

	"github.com/AllenDang/cimgui-go/imgui"
	"github.com/go-gl/glfw/v3.4/glfw"
	"github.com/go-gl/mathgl/mgl64"
)

// Keyboard and mouse: shortcuts, and moving the cameras. ImGui gets the
// input first: the views only get what it doesn't want.

// input is the keyboard and mouse state kept between frames.
type input struct {
	shortcuts []shortcut // pressed since the last frame
	shiftTap  bool       // Shift is down, and nothing else happened since

	// A drag that started outside of the panel belongs to the view until
	// released
	drag struct {
		active bool
		button imgui.MouseButton
	}
}

// Keyboard shortcuts
type shortcut int

const (
	shortcutView shortcut = iota
	shortcutColor
	shortcutShading
	shortcutWireframe
	shortcutEdit
	shortcutLockShore
	shortcutSmaller // brush
	shortcutLarger
	shortcutUndo
	shortcutRedo
	shortcutSave
	shortcutSaveAs
	shortcutNew
	shortcutOpen
	shortcutExport
	shortcutQuit
	shortcutReset
	shortcutMeasure
	shortcutBasin
	shortcutFinishRuler // Escape, Enter
	shortcutDeleteRuler // Delete, Backspace
)

// Shortcuts by key name: without modifiers, and the menu commands, with Ctrl
// (or Cmd), and with Shift too (else they are the same as without it). The
// color shortcut is Shift, tapped alone.
var (
	plainKeys = map[string]shortcut{
		"v": shortcutView,
		"r": shortcutReset,
		"q": shortcutShading,
		"w": shortcutWireframe,
		"e": shortcutEdit,
		"l": shortcutLockShore,
		"[": shortcutSmaller,
		"]": shortcutLarger,
		"m": shortcutMeasure,
		"b": shortcutBasin,
	}
	// Keys without names, without modifiers
	namelessKeys = map[glfw.Key]shortcut{
		glfw.KeyEscape:    shortcutFinishRuler,
		glfw.KeyEnter:     shortcutFinishRuler,
		glfw.KeyKPEnter:   shortcutFinishRuler,
		glfw.KeyDelete:    shortcutDeleteRuler,
		glfw.KeyBackspace: shortcutDeleteRuler,
	}
	commandKeys = map[string]shortcut{
		"z": shortcutUndo,
		"y": shortcutRedo,
		"s": shortcutSave,
		"n": shortcutNew,
		"o": shortcutOpen,
		"e": shortcutExport,
		"q": shortcutQuit,
	}
	commandShiftKeys = map[string]shortcut{
		"z": shortcutRedo,
		"s": shortcutSaveAs,
	}
)

// setCallbacks installs the window callbacks. They are installed before
// ImGui's, which call them in turn.
func (a *app) setCallbacks() {
	window := a.window
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
			a.openOther(names[0])
		}
		a.activity()
	})
}

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
		a.input.shiftTap = true

	case shift && action == glfw.Release:
		if a.input.shiftTap {
			a.input.shortcuts = append(a.input.shortcuts, shortcutColor)
		}
		a.input.shiftTap = false

	case action == glfw.Press:
		a.input.shiftTap = false
		if s, ok := keyShortcut(key, name, mods); ok {
			a.input.shortcuts = append(a.input.shortcuts, s)
		}
	}
}

// keyShortcut is the shortcut of a key press, if any.
func keyShortcut(key glfw.Key, name string, mods glfw.ModifierKey) (shortcut, bool) {
	command := mods&(glfw.ModControl|glfw.ModSuper) != 0
	switch {
	case command && mods&glfw.ModAlt == 0:
		if mods&glfw.ModShift != 0 {
			if s, ok := commandShiftKeys[name]; ok {
				return s, true
			}
		}
		s, ok := commandKeys[name]
		return s, ok
	case mods&(glfw.ModShift|glfw.ModControl|glfw.ModAlt|glfw.ModSuper) != 0:
		return 0, false
	}
	if s, ok := namelessKeys[key]; ok {
		return s, true
	}
	s, ok := plainKeys[name]
	return s, ok
}

// onMouse is called on mouse buttons and wheel: Shift is being used as a
// modifier.
func (a *app) onMouse() { a.input.shiftTap = false }

func (a *app) handleInput() {
	io := imgui.CurrentIO()

	// Not while typing in a field or using a widget, nor while a combo is
	// open, where they would change its value behind it
	popup := imgui.IsPopupOpenStrV("", imgui.PopupFlagsAnyPopupId|imgui.PopupFlagsAnyPopupLevel)
	if !io.WantCaptureKeyboard() && !popup {
		a.applyShortcuts()
	}
	a.input.shortcuts = a.input.shortcuts[:0]

	a.handleMouse()
}

// applyShortcuts runs the shortcuts pressed since the last frame.
func (a *app) applyShortcuts() {
	// Settings changes apply one by one: commands can change settings too
	// (saving remembers the project)
	change := func(edit func(settings *Settings)) {
		settings := a.settings
		edit(&settings)
		if settings != a.settings {
			a.setSettings(settings)
		}
	}

	for _, s := range a.input.shortcuts {
		switch s {
		case shortcutView:
			change(func(settings *Settings) { settings.View = cycle(views, settings.View) })
		case shortcutColor:
			change(func(settings *Settings) { settings.Color = cycle(colorModes, settings.Color) })
		case shortcutShading:
			change(func(settings *Settings) { settings.Shading = cycle(shadings, settings.Shading) })
		case shortcutWireframe:
			change(func(settings *Settings) { settings.Wireframe = !settings.Wireframe })
		case shortcutEdit:
			change(func(settings *Settings) { settings.setEditing(!settings.Editing) })
		case shortcutLockShore:
			change(func(settings *Settings) { settings.LockShore = !settings.LockShore })
		case shortcutSmaller:
			change(func(settings *Settings) {
				settings.BrushRadius = math.Max(minBrushRadius, math.Round(settings.BrushRadius/brushRadiusStep))
			})
		case shortcutLarger:
			change(func(settings *Settings) {
				settings.BrushRadius = math.Min(maxBrushRadius, math.Round(settings.BrushRadius*brushRadiusStep+0.5))
			})
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
		case shortcutMeasure:
			change(func(settings *Settings) { settings.setMeasuring(!settings.Measuring) })
		case shortcutBasin:
			change(func(settings *Settings) { settings.BasinColors = !settings.BasinColors })
		case shortcutFinishRuler:
			a.finishRuler()
		case shortcutDeleteRuler:
			a.deleteRuler()
		}
	}
}

// handleMouse paints, or moves the camera of the current view.
func (a *app) handleMouse() {
	io := imgui.CurrentIO()

	// Painting takes the left button; measuring its clicks
	painting := a.settings.painting()
	if a.paintInput() {
		return
	}
	a.measureInput()

	drag := &a.input.drag
	if !drag.active && !io.WantCaptureMouse() {
		for _, button := range []imgui.MouseButton{imgui.MouseButtonLeft, imgui.MouseButtonRight, imgui.MouseButtonMiddle} {
			if painting && button == imgui.MouseButtonLeft {
				continue
			}
			if imgui.IsMouseClickedBool(button) {
				drag.active, drag.button = true, button
				break
			}
		}
	}
	if drag.active && !imgui.IsMouseDown(drag.button) {
		drag.active = false
	}

	width, height, _ := a.viewSize()
	if delta := io.MouseDelta(); drag.active && (delta.X != 0 || delta.Y != 0) {
		a.dragView(float64(delta.X), float64(delta.Y), width, height, io.KeyShift() || io.KeyCtrl() || io.KeySuper())
	}

	if io.WantCaptureMouse() && !drag.active {
		return
	}
	if wheel := float64(io.MouseWheel()); wheel != 0 {
		a.wheelView(wheel, width, height)
	}
	if a.settings.View == viewOrbit && !painting && !a.settings.Measuring && imgui.IsMouseDoubleClicked(imgui.MouseButtonLeft) {
		a.centerOrbit()
	}
	if a.settings.View == viewMap && !painting {
		if imgui.IsMouseDoubleClicked(imgui.MouseButtonLeft) && !a.settings.Measuring {
			a.mapView.camera.fit(width, height)
		}
		if !a.settings.Measuring {
			imgui.SetMouseCursor(imgui.MouseCursorHand)
		}
	}
}

// dragView moves the camera of the current view for a mouse drag.
//
//   - orbit: left rotates (pans with shift, ctrl or cmd), right pans, middle
//     zooms; while painting, which takes the left button, right rotates
//     (pans with shift, ctrl or cmd)
//   - map: every button pans
func (a *app) dragView(dx, dy, width, height float64, modifier bool) {
	// Drag zoom: 0.95 per 100 pixels, zooming out when dragging down
	zoom := math.Pow(0.95, -dy/100)

	switch a.settings.View {
	case viewOrbit:
		camera := &a.terrain.orbit
		button := a.input.drag.button
		switch {
		case button == imgui.MouseButtonMiddle:
			camera.dolly(zoom)
		case modifier || (button == imgui.MouseButtonRight && !a.settings.painting()):
			camera.pan(dx, dy, height)
		default:
			camera.rotate(dx, dy, height)
		}

	case viewMap:
		camera := &a.mapView.camera
		camera.offset[0] += dx
		camera.offset[1] += dy
	}
}

// wheelView zooms the current view for a wheel movement (positive when
// scrolling up, one per notch).
func (a *app) wheelView(wheel, width, height float64) {
	switch a.settings.View {
	case viewOrbit:
		a.terrain.orbit.dolly(math.Pow(0.95, wheel))
	case viewMap:
		mouse := imgui.CurrentIO().MousePos()
		a.mapView.camera.zoomAt(float64(mouse.X), float64(mouse.Y), math.Exp(0.2*wheel), width, height)
	}
}

// centerOrbit moves the center of the orbit to the point under the cursor,
// at sea level, as panning keeps it.
func (a *app) centerOrbit() {
	mouse := imgui.CurrentIO().MousePos()
	position, ok := a.mapPosition(float64(mouse.X), float64(mouse.Y))
	if !ok {
		return
	}
	v := a.terrain
	v.orbit.target = mgl64.Vec3{position.X - v.width/2, position.Y - v.height/2, 0}
}

func (a *app) resetView() {
	if a.settings.View == viewMap {
		width, height, _ := a.viewSize()
		a.mapView.camera.fit(width, height)
	} else {
		a.terrain.resetCamera()
	}
}
