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
// shortcuts (double-click video = fullscreen, space = toggle pause, q = quit).
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
		p.revealControls(time.Now()) // brings the auto-hiding control bar up
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
		p.revealControls(time.Now()) // a click counts as activity too
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
		// schedules a wakeup; otherwise the click landed on the video itself
		// (see handlePrimaryClick — a single click there does nothing).
		if _, ok := p.router.WakeupTime(); !ok && btn == pointer.ButtonPrimary && action == glfw.Press {
			lastPrimaryPress = p.handlePrimaryClick(lastPrimaryPress)
		}
	})

	p.window.SetKeyCallback(func(w *glfw.Window, key glfw.Key, scancode int, action glfw.Action, mods glfw.ModifierKey) {
		switch action {
		case glfw.Press, glfw.Repeat:
		default: // Release
			return
		}
		if action == glfw.Press {
			if p.handleConfirmKey(key) {
				return
			}
			if p.handleEscape(w, key, mods) {
				return
			}
		}
		p.dispatchKey(keyChord{key: key, mods: relevantMods(mods)}, action == glfw.Repeat, time.Now())
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

// dispatchKey runs the action bound to chord. repeated marks a GLFW
// auto-repeat rather than a fresh press: those only fire for actions that opted
// in (action.repeat > 0) and no more often than that interval, so the OS repeat
// rate does not decide how fast the player navigates. now is the event time,
// injected so the throttle is testable.
func (p *Player) dispatchKey(chord keyChord, repeated bool, now time.Time) {
	id, ok := p.keymap[chord]
	if !ok {
		return
	}
	a, ok := p.actions[id]
	if !ok || a.fn == nil {
		return
	}
	if repeated {
		if a.repeat == 0 || now.Sub(p.lastRepeat[id]) < a.repeat {
			return
		}
	}
	if p.lastRepeat == nil {
		p.lastRepeat = make(map[actionID]time.Time)
	}
	p.lastRepeat[id] = now
	a.fn(p)
}

// doubleClickWindow is how close two primary presses have to be to count as a
// double click.
const doubleClickWindow = 400 * time.Millisecond

// clickOutcome is what an unconsumed primary click does. There is deliberately
// no "toggle pause" outcome: pausing by clicking anywhere on the video was
// removed 2026-07-26 (too easy to trigger by accident), leaving pause to the
// control bar's play button, the play-pause shortcut, and the menu entry.
type clickOutcome int

const (
	// clickNothing records the press for double-click tracking and does nothing
	// else — the plain single click on the video.
	clickNothing clickOutcome = iota
	clickCloseMenu
	clickFullscreen
)

// primaryClickOutcome decides what a primary click does, given whether the menu
// is open and when the previous press landed. Pure so the decision is testable
// without a window (toggleFullscreen and closeMenu both need one).
//
// The menu wins over the double click: closing it must not also count toward a
// pair, or dismissing a menu could throw the window into fullscreen. A zero
// lastPress is the first press of the session, which cannot complete a pair.
func primaryClickOutcome(menuOpen bool, now, lastPress time.Time) clickOutcome {
	switch {
	case menuOpen:
		return clickCloseMenu
	case !lastPress.IsZero() && now.Sub(lastPress) < doubleClickWindow:
		return clickFullscreen
	default:
		return clickNothing
	}
}

// handlePrimaryClick reacts to a primary click that Gio did not consume and
// returns the new last-press timestamp for double-click tracking. Menu item
// clicks are handled by Gio via the router and never reach here.
func (p *Player) handlePrimaryClick(lastPress time.Time) time.Time {
	now := time.Now()
	switch primaryClickOutcome(p.overlay == overlayMenu, now, lastPress) {
	case clickCloseMenu:
		p.closeMenu()
		return lastPress
	case clickFullscreen:
		p.toggleFullscreen()
		return time.Time{} // consumed: a third click starts a new pair
	case clickNothing:
	}
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
