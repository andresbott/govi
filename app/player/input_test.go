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

func TestNavigationAndVolumeActionsRepeat(t *testing.T) {
	byID := actionByID()
	for _, id := range []actionID{actNextVideo, actPrevVideo, actVolumeUp, actVolumeDown, actSeekForward, actSeekBack} {
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
