package player

import (
	"image/color"
	"testing"
	"time"
)

func TestProgressFractionIsThePlayedShare(t *testing.T) {
	if got, want := progressFraction(30, 120), float32(0.25); got != want {
		t.Errorf("progressFraction(30, 120) = %v, want %v", got, want)
	}
}

func TestProgressFractionUnknownDurationIsEmpty(t *testing.T) {
	// A live stream has no duration: the bar stays empty rather than dividing by
	// zero — there is no meaningful fraction of an unknown length.
	if got := progressFraction(42, 0); got != 0 {
		t.Errorf("progressFraction(42, 0) = %v, want 0", got)
	}
}

func TestProgressFractionClamps(t *testing.T) {
	tests := []struct {
		name     string
		pos, dur float64
		want     float32
	}{
		{"position past the end", 130, 120, 1},
		{"negative position", -5, 120, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := progressFraction(tc.pos, tc.dur); got != tc.want {
				t.Errorf("progressFraction(%v, %v) = %v, want %v", tc.pos, tc.dur, got, tc.want)
			}
		})
	}
}

func TestSeekTargetScalesTheFractionByDuration(t *testing.T) {
	if got, want := seekTarget(0.25, 120), 30.0; got != want {
		t.Errorf("seekTarget(0.25, 120) = %v, want %v", got, want)
	}
}

func TestSeekTargetClampsIntoTheFile(t *testing.T) {
	tests := []struct {
		name string
		frac float32
		dur  float64
		want float64
	}{
		{"fraction above one", 1.5, 120, 120},
		{"negative fraction", -0.5, 120, 0},
		{"unknown duration", 0.5, 0, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := seekTarget(tc.frac, tc.dur); got != tc.want {
				t.Errorf("seekTarget(%v, %v) = %v, want %v", tc.frac, tc.dur, got, tc.want)
			}
		})
	}
}

func TestSeekAbsoluteCommandUsesKeyframesForScrubbing(t *testing.T) {
	got := seekAbsoluteCommand(30.5)
	want := []string{"seek", "30.500", "absolute+keyframes"}
	if len(got) != len(want) {
		t.Fatalf("seekAbsoluteCommand(30.5) = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("seekAbsoluteCommand(30.5) = %q, want %q", got, want)
			break
		}
	}
}

func TestControlsVisibleWhileTheMouseIsFresh(t *testing.T) {
	now := time.Now()
	if !controlsVisible(now, now.Add(-100*time.Millisecond), false) {
		t.Error("controls hidden right after a mouse move")
	}
}

func TestControlsHideAfterTheMouseRests(t *testing.T) {
	now := time.Now()
	if controlsVisible(now, now.Add(-controlsHideAfter-time.Second), false) {
		t.Error("controls still shown long after the last mouse move")
	}
}

func TestControlsStayVisibleWhileDraggingTheKnob(t *testing.T) {
	// A slow scrub must not make the bar vanish under the pointer.
	now := time.Now()
	if !controlsVisible(now, now.Add(-controlsHideAfter-time.Second), true) {
		t.Error("controls hid while the knob was being dragged")
	}
}

func TestControlsHiddenBeforeAnyPointerInput(t *testing.T) {
	// The zero time is "never moved": nothing to reveal the bar yet.
	if controlsVisible(time.Now(), time.Time{}, false) {
		t.Error("controls shown before any pointer input")
	}
}

func TestRevealControlsShowsTheBar(t *testing.T) {
	p := &Player{}
	now := time.Now()

	p.revealControls(now)

	if !controlsVisible(now, p.lastInput, false) {
		t.Error("revealControls did not reveal the controls")
	}
}

// The "show progress" action (o) reveals the bar rather than flashing a clock.
// It works with no file loaded too — nothing to read, and the bar's own idle
// handling decides what to draw.
func TestShowProgressRevealsTheControlBar(t *testing.T) {
	p := &Player{}

	p.showProgress()

	if !controlsVisible(time.Now(), p.lastInput, false) {
		t.Error("the progress action did not reveal the control bar")
	}
	if p.osdVisible(time.Now()) {
		t.Errorf("the progress action flashed %q, want no text overlay", p.osdText)
	}
}

func TestControlsAlphaIsOpaqueOnceFadedIn(t *testing.T) {
	now := time.Now()
	revealed := now.Add(-controlsFade - time.Millisecond)
	if got := controlsAlpha(now, now, revealed, false); got != 1 {
		t.Errorf("controlsAlpha after the fade-in = %v, want 1", got)
	}
}

func TestControlsAlphaRampsUpWhileFadingIn(t *testing.T) {
	now := time.Now()
	revealed := now.Add(-controlsFade / 2)
	got := controlsAlpha(now, now, revealed, false)
	if got <= 0 || got >= 1 {
		t.Errorf("controlsAlpha halfway through the fade-in = %v, want strictly between 0 and 1", got)
	}
}

func TestControlsAlphaRampsDownWhileFadingOut(t *testing.T) {
	now := time.Now()
	// Past the hide delay but only halfway through the fade.
	last := now.Add(-controlsHideAfter - controlsFade/2)
	got := controlsAlpha(now, last, last, false)
	if got <= 0 || got >= 1 {
		t.Errorf("controlsAlpha halfway through the fade-out = %v, want strictly between 0 and 1", got)
	}
}

func TestControlsAlphaIsZeroOnceFadedOut(t *testing.T) {
	now := time.Now()
	last := now.Add(-controlsHideAfter - controlsFade - time.Millisecond)
	if got := controlsAlpha(now, last, last, false); got != 0 {
		t.Errorf("controlsAlpha after the fade-out = %v, want 0", got)
	}
}

func TestControlsAlphaIsZeroBeforeAnyInput(t *testing.T) {
	if got := controlsAlpha(time.Now(), time.Time{}, time.Time{}, false); got != 0 {
		t.Errorf("controlsAlpha before any input = %v, want 0", got)
	}
}

func TestControlsAlphaStaysOpaqueWhileDragging(t *testing.T) {
	now := time.Now()
	stale := now.Add(-controlsHideAfter - controlsFade - time.Second)
	if got := controlsAlpha(now, stale, stale, true); got != 1 {
		t.Errorf("controlsAlpha while dragging = %v, want 1 (no fade-out under the pointer)", got)
	}
}

func TestRevealControlsMidFadeOutResumesFromTheCurrentAlpha(t *testing.T) {
	// Re-revealing a half-faded bar must continue from where it is, not snap to
	// transparent and flicker back in.
	now := time.Now()
	last := now.Add(-controlsHideAfter - controlsFade/2)
	p := &Player{lastInput: last, revealedAt: last}
	before := controlsAlpha(now, p.lastInput, p.revealedAt, false)

	p.revealControls(now)

	after := controlsAlpha(now, p.lastInput, p.revealedAt, false)
	if diff := after - before; diff > 0.05 || diff < -0.05 {
		t.Errorf("alpha jumped from %v to %v across revealControls, want it to resume from roughly the same value", before, after)
	}
}

func TestRevealControlsWhileOpaqueKeepsItOpaque(t *testing.T) {
	// Continuous mouse movement must not restart the fade-in on every move.
	now := time.Now()
	p := &Player{lastInput: now.Add(-time.Second), revealedAt: now.Add(-time.Second)}

	p.revealControls(now)

	if got := controlsAlpha(now, p.lastInput, p.revealedAt, false); got != 1 {
		t.Errorf("controlsAlpha after a move on an already-visible bar = %v, want 1", got)
	}
}

func TestFadeColorScalesTheAlphaChannel(t *testing.T) {
	c := color.NRGBA{R: 0x3d, G: 0x8b, B: 0xff, A: 0xff}

	got := fadeColor(c, 0.5)

	if got.A != 0x7f && got.A != 0x80 {
		t.Errorf("fadeColor(…, 0.5).A = %#x, want about 0x80", got.A)
	}
	if got.R != c.R || got.G != c.G || got.B != c.B {
		t.Errorf("fadeColor changed the color channels: %v -> %v", c, got)
	}
}

func TestFadeColorClamps(t *testing.T) {
	c := color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
	if got := fadeColor(c, 2); got.A != 0xff {
		t.Errorf("fadeColor(…, 2).A = %#x, want 0xff", got.A)
	}
	if got := fadeColor(c, -1); got.A != 0 {
		t.Errorf("fadeColor(…, -1).A = %#x, want 0", got.A)
	}
}

func TestControlIconsDecode(t *testing.T) {
	// The icons are compiled-in material design paths; a decode failure is a
	// build bug, and a nil icon would panic at layout time (no display needed to
	// catch it here).
	tests := []struct {
		name string
		icon func() bool
	}{
		{"play", func() bool { return iconPlay != nil }},
		{"pause", func() bool { return iconPause != nil }},
		{"volume", func() bool { return iconVolume != nil }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if !tc.icon() {
				t.Errorf("%s icon failed to decode", tc.name)
			}
		})
	}
}

func TestPlayPauseIconFollowsPlaybackState(t *testing.T) {
	// Paused shows "play" (what the click will do), playing shows "pause".
	if got := playPauseIcon(true); got != iconPlay {
		t.Error("paused state does not show the play icon")
	}
	if got := playPauseIcon(false); got != iconPause {
		t.Error("playing state does not show the pause icon")
	}
}
