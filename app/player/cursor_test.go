package player

import (
	"testing"
	"time"
)

func TestCursorStaysVisibleWhileWindowed(t *testing.T) {
	// Only fullscreen hides the pointer: in a window it is how the user reaches
	// the title bar and everything else on the desktop.
	now := time.Now()
	if cursorHidden(now, now.Add(-time.Hour), false, false) {
		t.Error("pointer hidden in a windowed player")
	}
}

func TestCursorHidesOnceTheMouseRestsInFullscreen(t *testing.T) {
	now := time.Now()
	if !cursorHidden(now, now.Add(-cursorHideAfter-time.Millisecond), true, false) {
		t.Error("pointer still shown after the mouse rested in fullscreen")
	}
}

func TestCursorVisibleWhileTheMouseIsFreshInFullscreen(t *testing.T) {
	now := time.Now()
	if cursorHidden(now, now.Add(-100*time.Millisecond), true, false) {
		t.Error("pointer hidden right after a mouse move")
	}
}

func TestCursorHidesInFullscreenBeforeAnyMouseMove(t *testing.T) {
	// Fullscreen entered by keyboard with an untouched mouse: the pointer has
	// been resting the whole time, so there is nothing to keep it up.
	if !cursorHidden(time.Now(), time.Time{}, true, false) {
		t.Error("pointer shown in fullscreen although the mouse never moved")
	}
}

func TestCursorStaysVisibleWhileTheMenuIsOpen(t *testing.T) {
	// The context menu is aimed with the mouse: hiding the pointer under an open
	// menu would leave the user clicking blind.
	now := time.Now()
	p := &Player{lastPointerMove: now.Add(-cursorHideAfter), overlay: overlayMenu}

	p.syncCursor(now, true)

	if p.cursorHidden {
		t.Error("pointer hidden while the context menu was open")
	}
}

func TestCursorStaysVisibleWhileThePointerIsInUse(t *testing.T) {
	// A knob held still mid-drag is not an idle mouse: it is one the user is
	// holding, and the pointer is what they are aiming with.
	now := time.Now()
	if cursorHidden(now, now.Add(-cursorHideAfter), true, true) {
		t.Error("pointer hidden while it was still in use")
	}
}

func TestPointerBusyWhileASliderIsDragged(t *testing.T) {
	// The bar's own drag guard is what decides this, so the two auto-hides agree
	// on what "the pointer is in use" means.
	p := &Player{overlay: overlayNone}
	if p.pointerBusy() != p.barDragging() {
		t.Error("pointerBusy ignores a dragged slider knob")
	}
}

func TestSyncCursorHidesAfterTheDelay(t *testing.T) {
	p := &Player{lastPointerMove: time.Now().Add(-cursorHideAfter)}

	p.syncCursor(time.Now(), true)

	if !p.cursorHidden {
		t.Error("syncCursor left the pointer visible in an idle fullscreen player")
	}
}

func TestNotePointerMoveBringsTheCursorBack(t *testing.T) {
	now := time.Now()
	p := &Player{lastPointerMove: now.Add(-cursorHideAfter)}
	p.syncCursor(now, true)

	p.notePointerMove(now, true)

	if p.cursorHidden {
		t.Error("pointer stayed hidden after the mouse moved")
	}
	if !p.lastPointerMove.Equal(now) {
		t.Errorf("lastPointerMove = %v, want %v", p.lastPointerMove, now)
	}
}

func TestHandlePointerMoveRevealsBothTheBarAndTheCursor(t *testing.T) {
	// One mouse move has to undo both auto-hides: the control bar and the pointer.
	now := time.Now()
	p := &Player{lastPointerMove: now.Add(-cursorHideAfter)}
	p.syncCursor(now, true)

	p.handlePointerMove(now)

	if p.cursorHidden {
		t.Error("pointer stayed hidden after a mouse move")
	}
	if !controlsVisible(now, p.lastInput, false) {
		t.Error("control bar stayed hidden after a mouse move")
	}
	if !p.lastPointerMove.Equal(now) {
		t.Errorf("lastPointerMove = %v, want %v", p.lastPointerMove, now)
	}
}

func TestSyncCursorRevealsTheCursorOnLeavingFullscreen(t *testing.T) {
	now := time.Now()
	p := &Player{lastPointerMove: now.Add(-cursorHideAfter)}
	p.syncCursor(now, true)

	p.syncCursor(now, false)

	if p.cursorHidden {
		t.Error("pointer stayed hidden after leaving fullscreen")
	}
}
