package player

import (
	"fmt"
	"image"
	"image/color"
	"runtime"
	"time"

	"gioui.org/f32"
	"gioui.org/font/gofont"
	"gioui.org/gpu"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget/material"
	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/go-gl/glfw/v3.3/glfw"
)

const (
	prefsWinWidth  = 620
	prefsWinHeight = 520
	// prefsWinMinWidth is the narrowest the window can get before a shortcut row
	// starts clipping. A row is a fixed-width sequence (see overlay_prefs.go):
	// label 170 + chip 110 + clear 28 + spacer 10 + chip 110 + clear 28 = 456 dp,
	// inside UniformInset(20) on both sides = 496. Rounded to 500; below this the
	// second clear button is cut off, which is exactly the "messed up view" a
	// floor should prevent.
	prefsWinMinWidth = 500
	// prefsWinMinHeight keeps the header block (title, two hint lines, message)
	// plus a few list rows visible. The header costs ~120 dp with its spacers and
	// the 40 dp of inset; 300 leaves room for four 38 dp rows, enough that the
	// list still reads as a scrollable list rather than a single cramped row.
	prefsWinMinHeight = 300
	// prefsScrollScale converts one mouse-wheel notch into a scroll distance,
	// matching the factor Gio's own X11 backend applies.
	prefsScrollScale = 10
)

// scrollDistance converts a GLFW wheel offset into a Gio scroll distance.
// GLFW reports positive y for a wheel push away from the user, while Gio
// expects positive y to advance the content, so the sign flips.
func scrollDistance(xoff, yoff float64) f32.Point {
	return f32.Point{
		X: float32(-xoff * prefsScrollScale),
		Y: float32(-yoff * prefsScrollScale),
	}
}

// prefsWindow is the standalone preferences window: its own GLFW window, GL
// context and Gio renderer, entirely separate from the video window (which
// shares its framebuffer with mpv).
type prefsWindow struct {
	win    *glfw.Window
	gpuCtx gpu.GPU
	ops    op.Ops
	router input.Router
	// theme is this window's own material theme: a text.Shaper caches glyph
	// layout in plain maps and must not be shared between top-level windows,
	// so the video window's theme cannot be reused here.
	theme *material.Theme

	// pointer bookkeeping for the Gio router, mirroring input.go.
	btns    pointer.Buttons
	lastPos f32.Point
	epoch   time.Time
}

// syncPrefsWindow reconciles the live preferences window with the state the UI
// requested (p.prefsWanted). Must run on the main loop: GLFW forbids creating
// or destroying windows from inside a callback. Creation failures are logged
// and revoke the request rather than killing playback.
func (p *Player) syncPrefsWindow() {
	switch prefsWindowAction(p.prefsWanted, p.prefsWin != nil) {
	case prefsCreate:
		pw, err := p.newPrefsWindow()
		if err != nil {
			p.log.Error("open preferences window", "err", err)
			p.prefsWanted = false
			return
		}
		p.prefsWin = pw
	case prefsDestroy:
		p.destroyPrefsWindow()
	}
}

// destroyPrefsWindow releases the window's Gio renderer and GL context, then
// restores the video window's context as current.
func (p *Player) destroyPrefsWindow() {
	if p.prefsWin == nil {
		return
	}
	p.prefsWin.win.MakeContextCurrent()
	p.prefsWin.gpuCtx.Release()
	p.prefsWin.win.Destroy()
	p.prefsWin = nil
	p.capture = nil
	p.window.MakeContextCurrent()
}

// newPrefsWindow creates the window with its own GL context and Gio renderer.
// The context hints match the video window so Gio's gamma handling behaves
// identically (see the backbuffer note in initWindow); the context is
// independent (no sharing) so nothing can disturb mpv's render context.
func (p *Player) newPrefsWindow() (*prefsWindow, error) {
	if desktopGL {
		glfw.WindowHint(glfw.SRGBCapable, glfw.True)
	}
	glfw.WindowHint(glfw.ScaleToMonitor, glfw.True)
	glfw.WindowHint(glfw.CocoaRetinaFramebuffer, glfw.True)
	if desktopGL {
		glfw.WindowHint(glfw.ContextVersionMajor, 3)
		glfw.WindowHint(glfw.ContextVersionMinor, 3)
		glfw.WindowHint(glfw.OpenGLProfile, glfw.OpenGLCoreProfile)
		glfw.WindowHint(glfw.OpenGLForwardCompatible, glfw.True)
	} else {
		glfw.WindowHint(glfw.ContextCreationAPI, glfw.EGLContextAPI)
		glfw.WindowHint(glfw.ClientAPI, glfw.OpenGLESAPI)
		glfw.WindowHint(glfw.ContextVersionMajor, 3)
		glfw.WindowHint(glfw.ContextVersionMinor, 0)
	}

	win, err := glfw.CreateWindow(prefsWinWidth, prefsWinHeight, "govi — Preferences", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("create preferences window: %w", err)
	}
	setMinSize(win, prefsWinMinWidth, prefsWinMinHeight)

	theme := material.NewTheme()
	theme.Shaper = text.NewShaper(text.WithCollection(gofont.Collection()))
	pw := &prefsWindow{win: win, epoch: time.Now(), theme: theme}
	// Everything below needs the new context current; the video context is
	// restored before returning so the caller's next mpv render is unaffected.
	defer p.window.MakeContextCurrent()
	win.MakeContextCurrent()
	// No vsync here: a second blocking swap per iteration would halve the
	// video window's frame rate.
	glfw.SwapInterval(0)
	if desktopGL {
		// The forward-compatible core profile needs a bound VAO per context.
		var defVBA uint32
		gl.GenVertexArrays(1, &defVBA)
		gl.BindVertexArray(defVBA)
	}

	// Shared:false — unlike the video window, nothing but Gio draws into this
	// context, so it need not restore GL state after every frame.
	gpuCtx, err := gpu.New(gpu.OpenGL{ES: !desktopGL})
	if err != nil {
		win.Destroy()
		return nil, fmt.Errorf("init preferences gio gpu: %w", err)
	}
	pw.gpuCtx = gpuCtx

	p.registerPrefsCallbacks(pw)
	return pw, nil
}

// registerPrefsCallbacks forwards the preferences window's input to its own Gio
// router (for the binding chips) and its key presses to handlePrefsKey. Player
// shortcuts deliberately do not dispatch here.
func (p *Player) registerPrefsCallbacks(pw *prefsWindow) {
	pw.win.SetCursorPosCallback(func(w *glfw.Window, xpos, ypos float64) {
		scale := float32(1)
		if runtime.GOOS == "darwin" {
			scale, _ = w.GetContentScale()
		}
		pw.lastPos = f32.Point{X: float32(xpos) * scale, Y: float32(ypos) * scale}
		pw.router.Queue(pointer.Event{
			Kind:     pointer.Move,
			Position: pw.lastPos,
			Source:   pointer.Mouse,
			Time:     time.Since(pw.epoch),
			Buttons:  pw.btns,
		})
	})

	pw.win.SetMouseButtonCallback(func(w *glfw.Window, button glfw.MouseButton, action glfw.Action, mods glfw.ModifierKey) {
		var btn pointer.Buttons
		switch button {
		case glfw.MouseButton1:
			btn = pointer.ButtonPrimary
		case glfw.MouseButton2:
			btn = pointer.ButtonSecondary
		case glfw.MouseButton3:
			btn = pointer.ButtonTertiary
		}
		var kind pointer.Kind
		switch action {
		case glfw.Release:
			kind = pointer.Release
			pw.btns &^= btn
		case glfw.Press:
			kind = pointer.Press
			pw.btns |= btn
		default:
			return
		}
		pw.router.Queue(pointer.Event{
			Kind:     kind,
			Source:   pointer.Mouse,
			Time:     time.Since(pw.epoch),
			Position: pw.lastPos,
			Buttons:  pw.btns,
		})
	})

	// Mouse wheel: Gio drives the row list from pointer.Scroll events, so the
	// wheel has to be forwarded explicitly (GLFW reports it separately from
	// buttons).
	pw.win.SetScrollCallback(func(w *glfw.Window, xoff, yoff float64) {
		pw.router.Queue(pointer.Event{
			Kind:     pointer.Scroll,
			Source:   pointer.Mouse,
			Time:     time.Since(pw.epoch),
			Position: pw.lastPos,
			Buttons:  pw.btns,
			Scroll:   scrollDistance(xoff, yoff),
		})
	})

	pw.win.SetKeyCallback(func(w *glfw.Window, key glfw.Key, scancode int, action glfw.Action, mods glfw.ModifierKey) {
		if action != glfw.Press {
			return
		}
		p.handlePrefsKey(key, mods)
	})
}

// prefsFrame renders one frame of the preferences window. The video window's
// context is restored before returning.
func (p *Player) prefsFrame() error {
	pw := p.prefsWin
	defer p.window.MakeContextCurrent()
	pw.win.MakeContextCurrent()

	scale, _ := pw.win.GetContentScale()
	width, height := pw.win.GetFramebufferSize()
	sz := image.Point{X: width, Y: height}

	pw.ops.Reset()
	gtx := layout.Context{
		Ops:    &pw.ops,
		Now:    time.Now(),
		Source: pw.router.Source(),
		Metric: unit.Metric{
			PxPerDp: scale,
			PxPerSp: scale,
		},
		Constraints: layout.Exact(sz),
	}
	// The background is painted through Gio, not with a raw gl.Clear: Gio
	// renders into its own intermediate framebuffer, so clearing the default
	// one would leave the previous frame's glyphs behind (visible as smears
	// while the list scrolls).
	paint.Fill(gtx.Ops, prefsBG)
	p.layoutPrefsWindow(gtx, pw.theme)

	if err := pw.gpuCtx.Frame(gtx.Ops, gpu.OpenGLRenderTarget{}, sz); err != nil {
		return fmt.Errorf("preferences gio frame: %w", err)
	}
	pw.router.Frame(&pw.ops)

	pw.win.SwapBuffers()
	return nil
}

// prefsBG is the preferences window background: fully opaque, unlike the
// video overlays' translucent panelBG, because there is no video behind it.
// Text is drawn onto this within the same Gio frame, so glyphs blend against
// a known background and stay crisp.
var prefsBG = color.NRGBA{R: 0x1e, G: 0x1e, B: 0x22, A: 0xff}
