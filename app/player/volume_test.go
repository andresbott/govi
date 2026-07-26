package player

import (
	"log/slog"
	"testing"
	"time"

	mpv "github.com/gen2brain/go-mpv"
)

func TestVolumeFractionIsTheShareOfTheMaximum(t *testing.T) {
	if got, want := volumeFraction(45), float32(0.45); got != want {
		t.Errorf("volumeFraction(45) = %v, want %v", got, want)
	}
}

func TestVolumeFractionClamps(t *testing.T) {
	tests := []struct {
		name string
		vol  float64
		want float32
	}{
		{"above the maximum", volumeMax + 20, 1},
		{"negative", -5, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := volumeFraction(tc.vol); got != tc.want {
				t.Errorf("volumeFraction(%v) = %v, want %v", tc.vol, got, tc.want)
			}
		})
	}
}

func TestVolumeTargetScalesTheFractionByTheMaximum(t *testing.T) {
	if got, want := volumeTarget(0.45), 45.0; got != want {
		t.Errorf("volumeTarget(0.45) = %v, want %v", got, want)
	}
}

func TestVolumeTargetClamps(t *testing.T) {
	tests := []struct {
		name string
		frac float32
		want float64
	}{
		{"fraction above one", 1.5, volumeMax},
		{"negative fraction", -0.5, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := volumeTarget(tc.frac); got != tc.want {
				t.Errorf("volumeTarget(%v) = %v, want %v", tc.frac, got, tc.want)
			}
		})
	}
}

// The slider sets an absolute level, unlike the volume shortcuts, which nudge it
// by a delta ("add volume").
func TestSetVolumeCommandIsAbsolute(t *testing.T) {
	got := setVolumeCommand(45.6)
	want := []string{"set", "volume", "46"}
	if len(got) != len(want) {
		t.Fatalf("setVolumeCommand(45.6) = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("setVolumeCommand(45.6) = %q, want %q", got, want)
			break
		}
	}
}

// The slider maps its full travel onto 0..volumeMax, so mpv's own ceiling has to
// be the same number: a larger one would leave part of the range unreachable, a
// smaller one would let the knob ask for a level mpv silently clamps, leaving the
// knob sitting where the volume isn't.
func TestVolumeMaxOptionMatchesTheSliderRange(t *testing.T) {
	if got, want := volumeMaxOption(), "100"; got != want {
		t.Errorf("volumeMaxOption() = %q, want %q", got, want)
	}
	if volumeMax != 100 {
		t.Errorf("volumeMax = %v but volumeMaxOption() is %q — the two must agree", volumeMax, volumeMaxOption())
	}
}

func TestVolumeIconFollowsLevelAndMuteState(t *testing.T) {
	tests := []struct {
		name  string
		vol   float64
		muted bool
		want  string
	}{
		{"muted", 80, true, "off"},
		{"muted at zero", 0, true, "off"},
		{"silent but not muted", 0, false, "down"},
		{"low", 30, false, "down"},
		{"high", 80, false, "up"},
	}
	seen := map[string]bool{}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			want := map[string]any{"off": iconVolumeOff, "down": iconVolumeDown, "up": iconVolumeUp}[tc.want]
			if got := volumeIcon(tc.vol, tc.muted); got != want {
				t.Errorf("volumeIcon(%v, %v) is not the %q icon", tc.vol, tc.muted, tc.want)
			}
			seen[tc.want] = true
		})
	}
	if len(seen) != 3 {
		t.Errorf("the table exercised %d of the 3 volume icons", len(seen))
	}
}

func TestVolumeIconsDecode(t *testing.T) {
	// Compiled-in material design paths, like the play/pause pair: a nil icon
	// would panic at layout time (no display needed to catch it here).
	tests := []struct {
		name string
		ok   func() bool
	}{
		{"volume up", func() bool { return iconVolumeUp != nil }},
		{"volume down", func() bool { return iconVolumeDown != nil }},
		{"volume off", func() bool { return iconVolumeOff != nil }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if !tc.ok() {
				t.Errorf("%s icon failed to decode", tc.name)
			}
		})
	}
}

func TestNoteVolumePropPublishesTheLevel(t *testing.T) {
	p := &Player{}

	p.noteVolumeProp(40.0)

	if got := p.volumeLevel(); got != 40 {
		t.Errorf("volumeLevel() = %v, want 40", got)
	}
}

// mpv sends a property change with no value when a property becomes
// unavailable. Zero is what the icon and the knob already treat as silence, and
// it beats keeping a level from before.
func TestNoteVolumePropUnavailableResetsTheLevel(t *testing.T) {
	p := &Player{}
	p.noteVolumeProp(40.0)

	p.noteVolumeProp(nil)

	if got := p.volumeLevel(); got != 0 {
		t.Errorf("volumeLevel() = %v after volume became unavailable, want 0", got)
	}
}

// The volume knob follows mpv from the observed level, on every frame the bar is
// visible — a synchronous read there would block on mpv's render timeout (see
// invariant 6 in docs/agents/player.md). A Player with no mpv handle pins it: a
// synchronous read would panic.
func TestVolumeKnobFollowsObservedLevel(t *testing.T) {
	p := &Player{}
	p.noteVolumeProp(40.0)

	p.syncVolumeKnob()

	if got, want := p.volume.Value, float32(0.4); got != want {
		t.Errorf("volume.Value = %v, want %v", got, want)
	}
}

// Dragging the volume knob sets mpv's level, the way scrubbing seeks.
func TestSetVolumeAppliesTheFractionToMpv(t *testing.T) {
	p := headlessPlayer(t)

	p.setVolume(0.4)

	if got := p.propInt("volume"); got != 40 {
		t.Errorf("mpv volume = %v after a drag to 0.4, want 40", got)
	}
}

// A drag on the volume slider must not seek: the two sliders share the bar, and
// wiring the wrong one to scrub would jump playback on a volume change.
func TestSetVolumeDoesNotSeek(t *testing.T) {
	p := headlessPlayer(t)
	if err := p.mpv.Command([]string{"loadfile", encodeClip(t, t.TempDir(), "a.mp4", 20)}); err != nil {
		t.Fatal(err)
	}
	waitPlaying(t, p)
	before := p.propFloat("time-pos")

	p.setVolume(0.4)

	if after := p.propFloat("time-pos"); after < before {
		t.Errorf("time-pos went from %.2fs to %.2fs across a volume drag, want no backwards seek", before, after)
	}
}

// The volume shortcuts bring the control bar up, like the seek shortcuts: the
// slider and the mute glyph are the whole readout, with no text overlay — the
// same trade the seek bar made when it replaced the progress flash.
func TestChangeVolumeRevealsTheControlBarWithoutText(t *testing.T) {
	p := headlessPlayer(t)

	p.changeVolume(-5)

	if !controlsVisible(time.Now(), p.lastInput, false) {
		t.Error("a volume change did not reveal the control bar")
	}
	if p.osdVisible(time.Now()) {
		t.Errorf("a volume change flashed %q, want no text overlay", p.osdText)
	}
}

func TestToggleMuteRevealsTheControlBarWithoutText(t *testing.T) {
	p := headlessPlayer(t)

	p.toggleMute()

	if !controlsVisible(time.Now(), p.lastInput, false) {
		t.Error("toggling mute did not reveal the control bar")
	}
	if p.osdVisible(time.Now()) {
		t.Errorf("toggling mute flashed %q, want no text overlay", p.osdText)
	}
}

// noteVolumeChanged reads the clamped level back from mpv, so the slider takes it
// from there instead of sitting stale until the pump delivers the observation a
// frame or more later. That read is why removing the text did not remove the
// function: the keyboard paths still have to refresh the knob.
func TestNoteVolumeChangedUpdatesTheObservedLevel(t *testing.T) {
	p := headlessPlayer(t)
	if err := p.mpv.Command(setVolumeCommand(40)); err != nil {
		t.Fatal(err)
	}

	p.noteVolumeChanged()

	if got := p.volumeLevel(); got != 40 {
		t.Errorf("volumeLevel() = %v after the mpv read, want 40", got)
	}
}

// End-to-end over every keyboard path to the volume, dispatched through the
// registry the way a real key press is: each must leave the bar visible with the
// slider on the level mpv ended up at. The per-action tests above cover
// changeVolume/toggleMute directly; this one is what catches a *binding* that
// stops reporting — e.g. a new volume step wired without the reveal.
func TestKeyboardVolumeActionsRevealTheBarWithTheLevel(t *testing.T) {
	for _, id := range []actionID{actVolumeUp, actVolumeDown, actMute} {
		t.Run(string(id), func(t *testing.T) {
			p := headlessPlayer(t)
			p.actions = actionByID()
			if err := p.mpv.Command(setVolumeCommand(40)); err != nil {
				t.Fatal(err)
			}

			p.runAction(id)
			p.syncVolumeKnob()

			if !controlsVisible(time.Now(), p.lastInput, false) {
				t.Errorf("%s did not reveal the control bar", id)
			}
			want := volumeFraction(float64(p.propInt("volume")))
			if got := p.volume.Value; got != want {
				t.Errorf("%s left the slider at %v, want %v (mpv's level)", id, got, want)
			}
			if p.osdVisible(time.Now()) {
				t.Errorf("%s flashed %q, want no text overlay", id, p.osdText)
			}
		})
	}
}

// The button's glyph is the mute state, so the keyboard and the menu change it
// too — a mute toggled by "m" must not leave the bar showing a speaker.
func TestTheVolumeIconFollowsAKeyboardMute(t *testing.T) {
	p := headlessPlayer(t)
	p.actions = actionByID()

	p.runAction(actMute)

	if got := volumeIcon(p.volumeLevel(), p.muted); got != iconVolumeOff {
		t.Error("the volume button still shows a speaker after a keyboard mute")
	}
}

// The wiring the unit tests above take as given: observeVolume registers the
// observer on a real libmpv and the pump routes what comes back into the cache.
// mpv reports a newly observed property straight away, so a pre-seeded level has
// to be corrected by the pump alone. No file is loaded on purpose: the pump calls
// glfw.PostEmptyEvent on end-file, which needs a GLFW the display-free tests do
// not have. The handle is built here rather than via headlessPlayer because the
// pump owns the event queue, and headlessPlayer's cleanup drains it from a second
// thread — which mpv_wait_event forbids.
func TestPumpRoutesObservedVolume(t *testing.T) {
	m := mpv.New()
	for k, v := range map[string]string{"vo": "null", "ao": "null", "idle": "yes", "volume": "60"} {
		if err := m.SetOptionString(k, v); err != nil {
			t.Fatal(err)
		}
	}
	if err := m.Initialize(); err != nil {
		t.Skipf("cannot initialize headless mpv: %v", err)
	}
	p := &Player{log: slog.Default(), mpv: m}
	p.noteVolumeProp(11.0) // stale value the pump has to correct

	if err := p.observeVolume(); err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	go func() {
		forwardMpvLogs(p)
		close(done)
	}()
	stop := func() {
		m.Command([]string{"quit"}) //nolint:errcheck // best-effort shutdown
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("forwardMpvLogs did not return after quit")
		}
		m.TerminateDestroy()
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if p.volumeLevel() == 60 {
			stop()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	got := p.volumeLevel()
	stop()
	t.Errorf("volumeLevel() = %v, want 60 — the pump never delivered the observed volume", got)
}
