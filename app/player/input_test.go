package player

import (
	"testing"
	"time"

	"github.com/go-gl/glfw/v3.3/glfw"
)

// counterPlayer builds a Player whose only action is a repeatable counter bound
// to "x", so dispatchKey can be exercised without mpv or a window.
func counterPlayer(t *testing.T, repeat time.Duration) (*Player, *int) {
	t.Helper()
	const id actionID = "test-count"
	fired := 0
	p := &Player{
		keymap: map[keyChord]actionID{{glfw.KeyX, 0}: id},
		actions: map[actionID]action{
			id: {id: id, fn: func(*Player) { fired++ }, repeat: repeat},
		},
	}
	return p, &fired
}

func TestDispatchKeyFiresOnPress(t *testing.T) {
	p, fired := counterPlayer(t, 0)
	p.dispatchKey(keyChord{glfw.KeyX, 0}, false, time.Now())
	if *fired != 1 {
		t.Errorf("action fired %d times on a press, want 1", *fired)
	}
}

func TestDispatchKeyIgnoresRepeatForNonRepeatableAction(t *testing.T) {
	p, fired := counterPlayer(t, 0)
	now := time.Now()
	p.dispatchKey(keyChord{glfw.KeyX, 0}, false, now)
	p.dispatchKey(keyChord{glfw.KeyX, 0}, true, now.Add(time.Second))
	if *fired != 1 {
		t.Errorf("action fired %d times, want 1 (auto-repeat must not fire it)", *fired)
	}
}

func TestDispatchKeyRepeatsWhenHeldPastTheInterval(t *testing.T) {
	p, fired := counterPlayer(t, 100*time.Millisecond)
	now := time.Now()
	p.dispatchKey(keyChord{glfw.KeyX, 0}, false, now)
	p.dispatchKey(keyChord{glfw.KeyX, 0}, true, now.Add(150*time.Millisecond))
	p.dispatchKey(keyChord{glfw.KeyX, 0}, true, now.Add(300*time.Millisecond))
	if *fired != 3 {
		t.Errorf("action fired %d times, want 3 (press + two repeats)", *fired)
	}
}

func TestDispatchKeyThrottlesRepeatsInsideTheInterval(t *testing.T) {
	p, fired := counterPlayer(t, 100*time.Millisecond)
	now := time.Now()
	p.dispatchKey(keyChord{glfw.KeyX, 0}, false, now)
	// The OS repeats faster than the action's interval: these are dropped.
	p.dispatchKey(keyChord{glfw.KeyX, 0}, true, now.Add(30*time.Millisecond))
	p.dispatchKey(keyChord{glfw.KeyX, 0}, true, now.Add(60*time.Millisecond))
	if *fired != 1 {
		t.Errorf("action fired %d times, want 1 (repeats inside the interval are dropped)", *fired)
	}
}

func TestDispatchKeyUnboundChordIsNoop(t *testing.T) {
	p, fired := counterPlayer(t, 0)
	p.dispatchKey(keyChord{glfw.KeyZ, 0}, false, time.Now())
	if *fired != 0 {
		t.Errorf("action fired %d times for an unbound chord, want 0", *fired)
	}
}

// A plain click on the video does nothing to playback: pausing is the play
// button, the space bar, and the menu. The Player carries no mpv handle, so a
// stray togglePause would panic rather than quietly flip a flag.
func TestPrimaryClickDoesNotTogglePause(t *testing.T) {
	p := &Player{}

	p.handlePrimaryClick(time.Time{})

	if p.paused {
		t.Error("a single click paused the video")
	}
}

// The whole click decision is pure, so every branch is reachable without a
// window (toggleFullscreen and closeMenu both need GLFW). Crucially there is no
// pause outcome at all: a single click on the video is clickNothing.
func TestPrimaryClickOutcome(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name      string
		menuOpen  bool
		lastPress time.Time
		want      clickOutcome
	}{
		{"plain single click does nothing", false, now.Add(-doubleClickWindow - time.Second), clickNothing},
		{"first press of the session does nothing", false, time.Time{}, clickNothing},
		{"second press inside the window goes fullscreen", false, now.Add(-doubleClickWindow / 2), clickFullscreen},
		{"second press past the window does nothing", false, now.Add(-doubleClickWindow - time.Millisecond), clickNothing},
		{"menu open closes the menu", true, time.Time{}, clickCloseMenu},
		// The menu must win: dismissing a menu with a quick second click cannot
		// also throw the window into fullscreen.
		{"menu open beats a double click", true, now.Add(-doubleClickWindow / 2), clickCloseMenu},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := primaryClickOutcome(tc.menuOpen, now, tc.lastPress); got != tc.want {
				t.Errorf("primaryClickOutcome(menuOpen=%v) = %v, want %v", tc.menuOpen, got, tc.want)
			}
		})
	}
}

// A single click records its timestamp so a following one can pair with it.
func TestSingleClickRecordsItsTimestamp(t *testing.T) {
	p := &Player{}

	last := p.handlePrimaryClick(time.Time{})

	if last.IsZero() {
		t.Error("a single click did not record its timestamp, so a double click could never be detected")
	}
	if p.paused {
		t.Error("a single click paused the video")
	}
}

// A click with the menu open closes it and leaves the double-click bookkeeping
// untouched.
func TestPrimaryClickClosesTheMenuFirst(t *testing.T) {
	p := &Player{overlay: overlayMenu}
	last := time.Now().Add(-doubleClickWindow / 2) // would otherwise be a double click

	got := p.handlePrimaryClick(last)

	if p.overlay == overlayMenu {
		t.Error("a click with the menu open did not close it")
	}
	if !got.Equal(last) {
		t.Error("closing the menu disturbed the double-click timestamp")
	}
	if p.paused {
		t.Error("closing the menu paused the video")
	}
}

func TestNavigationAndVolumeActionsRepeat(t *testing.T) {
	byID := actionByID()
	for _, id := range []actionID{actNextVideo, actPrevVideo, actVolumeUp, actVolumeDown,
		actSeekForward, actSeekBack, actSeekForwardPct, actSeekBackPct} {
		if byID[id].repeat == 0 {
			t.Errorf("%q should keep firing while its key is held down", id)
		}
	}
}

func TestDestructiveActionsDoNotRepeat(t *testing.T) {
	byID := actionByID()
	for _, id := range []actionID{actQuit, actDelete, actTrash, actPlayPause, actFullscreen, actMute, actProgress} {
		if byID[id].repeat != 0 {
			t.Errorf("%q must not fire on auto-repeat", id)
		}
	}
}
