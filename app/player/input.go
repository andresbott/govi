package player

import (
	"runtime"
	"time"

	"gioui.org/f32"
	"gioui.org/io/pointer"
	"github.com/go-gl/glfw/v3.3/glfw"
)

// registerCallbacks forwards GLFW input to Gio's router so overlay widgets
// receive pointer events; events Gio doesn't consume fall through to player
// shortcuts (click video = toggle pause, space = toggle pause, q = quit).
func (p *Player) registerCallbacks() {
	var btns pointer.Buttons
	beginning := time.Now()
	var lastPos f32.Point
	var lastPrimaryPress time.Time

	p.window.SetCursorPosCallback(func(w *glfw.Window, xpos, ypos float64) {
		scale := float32(1)
		if runtime.GOOS == "darwin" {
			// macOS cursor positions are not scaled to the underlying
			// framebuffer size when CocoaRetinaFramebuffer is true.
			scale, _ = w.GetContentScale()
		}
		lastPos = f32.Point{X: float32(xpos) * scale, Y: float32(ypos) * scale}
		p.router.Queue(pointer.Event{
			Kind:     pointer.Move,
			Position: lastPos,
			Source:   pointer.Mouse,
			Time:     time.Since(beginning),
			Buttons:  btns,
		})
	})

	p.window.SetMouseButtonCallback(func(w *glfw.Window, button glfw.MouseButton, action glfw.Action, mods glfw.ModifierKey) {
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
			btns &^= btn
		case glfw.Press:
			kind = pointer.Press
			btns |= btn
		default:
			return
		}
		p.router.Queue(pointer.Event{
			Kind:     kind,
			Source:   pointer.Mouse,
			Time:     time.Since(beginning),
			Position: lastPos,
			Buttons:  btns,
		})
		// Right click opens the context menu at the cursor.
		if btn == pointer.ButtonSecondary && action == glfw.Press {
			p.openMenu(lastPos)
			return
		}

		// If Gio consumed the click (a widget is animating a response), it
		// schedules a wakeup; otherwise treat a primary click on the video
		// itself as play/pause.
		if _, ok := p.router.WakeupTime(); !ok && btn == pointer.ButtonPrimary && action == glfw.Press {
			lastPrimaryPress = p.handlePrimaryClick(lastPrimaryPress)
		}
	})

	p.window.SetKeyCallback(func(w *glfw.Window, key glfw.Key, scancode int, action glfw.Action, mods glfw.ModifierKey) {
		if action != glfw.Press {
			return
		}
		if p.handleConfirmKey(key) {
			return
		}
		if p.handleEscape(w, key, mods) {
			return
		}
		chord := keyChord{key: key, mods: relevantMods(mods)}
		if id, ok := p.keymap[chord]; ok {
			if a, ok := p.actions[id]; ok && a.fn != nil {
				a.fn(p)
			}
		}
	})

	p.window.SetDropCallback(func(w *glfw.Window, names []string) {
		if len(names) == 0 {
			return
		}
		path := names[0] // first dropped path wins
		p.log.Info("loading dropped file", "path", path)
		p.openFile(path)
	})
}

// handlePrimaryClick reacts to a primary click that Gio did not consume and
// returns the new last-press timestamp for double-click tracking.
func (p *Player) handlePrimaryClick(lastPress time.Time) time.Time {
	// A primary click while the menu is open closes it instead of toggling
	// pause; menu item clicks are handled by Gio via the router and won't
	// reach here.
	if p.overlay == overlayMenu {
		p.closeMenu()
		return lastPress
	}
	// Double click toggles fullscreen; the single-click pause from the first
	// press is undone so playback state is unchanged.
	now := time.Now()
	if now.Sub(lastPress) < 400*time.Millisecond {
		p.togglePause()
		p.toggleFullscreen()
		return time.Time{}
	}
	p.togglePause()
	return now
}

// handleConfirmKey consumes Enter (confirm) and Esc (cancel) while the delete
// confirmation is open, reporting whether the key was consumed. Every other key
// keeps dispatching so shortcuts stay live.
func (p *Player) handleConfirmKey(key glfw.Key) bool {
	if p.overlay != overlayConfirm {
		return false
	}
	switch key {
	case glfw.KeyEnter, glfw.KeyKPEnter:
		p.deleteConfirmed()
		return true
	case glfw.KeyEscape:
		p.closeConfirm()
		return true
	}
	return false
}

// handleEscape applies Esc's context-sensitive behaviour, checked before the
// keymap: close an open overlay first, then exit fullscreen. It reports whether
// Esc was consumed; if not, Esc falls through to its bound action (quit by
// default).
func (p *Player) handleEscape(w *glfw.Window, key glfw.Key, mods glfw.ModifierKey) bool {
	if key != glfw.KeyEscape || mods != 0 {
		return false
	}
	if p.overlay != overlayNone {
		if p.overlay == overlayMenu {
			p.closeMenu()
		} else {
			p.overlay = overlayNone
		}
		return true
	}
	if w.GetMonitor() != nil {
		p.toggleFullscreen()
		return true
	}
	return false
}

// relevantMods strips lock modifiers (Caps/Num Lock) that GLFW may report so
// chord matching only considers Ctrl/Alt/Shift/Super.
func relevantMods(m glfw.ModifierKey) glfw.ModifierKey {
	return m & (glfw.ModControl | glfw.ModAlt | glfw.ModShift | glfw.ModSuper)
}
