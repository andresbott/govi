package player

import (
	"time"

	"github.com/go-gl/glfw/v3.3/glfw"
)

// cursorHideAfter is how long the mouse has to rest before the pointer is hidden
// in fullscreen. Longer than controlsHideAfter: the bar going away is a small
// change the user can undo by moving a millimetre, while a vanished pointer is
// briefly disorienting, so it waits until the mouse is clearly out of use.
// The loop reconciles this every idleFrame, so it needs no timer — the same
// reasoning as the control bar and the status flash in osd.go.
const cursorHideAfter = 3 * time.Second

// cursorHidden reports whether the mouse pointer should be hidden at now, given
// when it last moved. Only fullscreen hides it: in a window the pointer is how
// the user reaches the title bar and the rest of the desktop, and a video filling
// the screen is the only case where the pointer sits on top of the picture with
// nothing else to do. busy pins it visible for the same reason the control bar
// has its drag guard — a mouse held still on a knob or aimed at an open menu is
// in use, not idle. A zero lastMove means the mouse has not moved at all: a
// fullscreen entered by keyboard starts with the pointer already at rest, so it
// hides immediately rather than waiting out a delay that never started.
func cursorHidden(now, lastMove time.Time, fullscreen, busy bool) bool {
	if !fullscreen || busy {
		return false
	}
	if lastMove.IsZero() {
		return true
	}
	return now.Sub(lastMove) >= cursorHideAfter
}

// pointerBusy reports whether the pointer is in use even though it is not
// moving: held on a slider knob, or aimed at the context menu it just opened.
// Both are cases where the user is looking for the pointer, so it stays up.
// Overlays other than the menu are keyboard-driven, so they do not count.
func (p *Player) pointerBusy() bool {
	return p.barDragging() || p.overlay == overlayMenu
}

// syncCursor brings the window's pointer in line with cursorHidden. Called from
// the loop every iteration, so leaving fullscreen (or the mouse going still)
// takes effect within a frame; the GLFW call is only made on a change, so the
// steady state costs nothing.
func (p *Player) syncCursor(now time.Time, fullscreen bool) {
	hide := cursorHidden(now, p.lastPointerMove, fullscreen, p.pointerBusy())
	if hide == p.cursorHidden {
		return
	}
	p.cursorHidden = hide
	p.applyCursorMode(hide)
}

// notePointerMove records a mouse move and brings the pointer straight back,
// rather than waiting for the loop: the reveal has to feel like it happens with
// the movement, not up to a frame later.
func (p *Player) notePointerMove(now time.Time, fullscreen bool) {
	p.lastPointerMove = now
	p.syncCursor(now, fullscreen)
}

// handlePointerMove is what a mouse move does to the two things that hide
// themselves when it rests: the control bar comes up, and the pointer comes back.
// The single entry point the GLFW cursor callback calls, so the two can never
// drift apart.
func (p *Player) handlePointerMove(now time.Time) {
	p.revealControls(now)
	p.notePointerMove(now, p.fullscreen())
}

// applyCursorMode sets the GLFW cursor mode. CursorHidden (not CursorDisabled)
// keeps the pointer free: it is invisible over the window but still moves
// normally and can leave for another screen or window. Unit tests build a Player
// without a window.
func (p *Player) applyCursorMode(hidden bool) {
	if p.window == nil {
		return
	}
	mode := glfw.CursorNormal
	if hidden {
		mode = glfw.CursorHidden
	}
	p.window.SetInputMode(glfw.CursorMode, mode)
}

// fullscreen reports whether the video window currently owns a monitor.
func (p *Player) fullscreen() bool {
	return p.window != nil && p.window.GetMonitor() != nil
}
