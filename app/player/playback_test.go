package player

import (
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	mpv "github.com/gen2brain/go-mpv"
)

func TestNotePlaybackPropPublishesPosition(t *testing.T) {
	p := &Player{}

	p.notePlaybackProp("time-pos", 42.5)

	if got := p.playbackPos(); got != 42.5 {
		t.Errorf("playbackPos() = %v, want 42.5", got)
	}
}

func TestNotePlaybackPropPublishesDuration(t *testing.T) {
	p := &Player{}

	p.notePlaybackProp("duration", 180.0)

	if got := p.playbackDur(); got != 180.0 {
		t.Errorf("playbackDur() = %v, want 180", got)
	}
}

// mpv sends a property change with no value when the property becomes
// unavailable — which is what happens to time-pos on stop. The cached position
// has to go back to zero, otherwise the next progress flash reports where the
// previous file happened to stop.
func TestNotePlaybackPropUnavailableResetsPosition(t *testing.T) {
	p := &Player{}
	p.notePlaybackProp("time-pos", 42.5)

	p.notePlaybackProp("time-pos", nil)

	if got := p.playbackPos(); got != 0 {
		t.Errorf("playbackPos() = %v after time-pos became unavailable, want 0", got)
	}
}

// seek reads the position and duration to decide whether the step runs off the
// end (advancePastEnd). Those reads are on the render thread as well, so they
// come from the cache: with the observed position near the end of the file and
// an entry following, the seek must continue with the next video without ever
// asking mpv. A nil mpv handle would panic on a synchronous read, and
// playAdjacent's own loadFile tolerates it.
func TestSeekPastEndUsesObservedValues(t *testing.T) {
	p, dir := playerWithPlaylist(t, "a.mp4", "b.mp4")
	p.notePlaybackProp("time-pos", 55.0)
	p.notePlaybackProp("duration", 60.0)

	p.seek(10)

	if got, want := p.pl.current(), filepath.Join(dir, "b.mp4"); got != want {
		t.Errorf("current() = %q, want %q — the seek past the end did not advance", got, want)
	}
}

// The wiring the unit tests above take as given: observePlayback registers the
// observers on a real libmpv and the pump routes what comes back into the
// cache. mpv reports each newly observed property straight away — while idle
// time-pos is unavailable, so a pre-seeded position must be reset by the pump
// alone. No file is loaded on purpose: the pump calls glfw.PostEmptyEvent on
// end-file, which needs a GLFW the display-free tests do not have.
func TestPumpRoutesObservedPlaybackProps(t *testing.T) {
	// The mpv handle is built here rather than via headlessPlayer: the pump owns
	// the event queue for the duration of the test, and headlessPlayer's cleanup
	// drains it from a second thread, which mpv_wait_event forbids.
	m := mpv.New()
	for k, v := range map[string]string{"vo": "null", "ao": "null", "idle": "yes"} {
		if err := m.SetOptionString(k, v); err != nil {
			t.Fatal(err)
		}
	}
	if err := m.Initialize(); err != nil {
		t.Skipf("cannot initialize headless mpv: %v", err)
	}
	p := &Player{log: slog.Default(), mpv: m}
	p.notePlaybackProp("time-pos", 42.5) // stale value the pump has to correct

	if err := p.observePlayback(); err != nil {
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
		if p.playbackPos() == 0 {
			stop()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	got := p.playbackPos()
	stop()
	t.Errorf("playbackPos() = %v, want 0 — the pump never delivered the observed time-pos", got)
}

func TestSeekPercentPastEndUsesObservedValues(t *testing.T) {
	p, dir := playerWithPlaylist(t, "a.mp4", "b.mp4")
	p.notePlaybackProp("time-pos", 55.0)
	p.notePlaybackProp("duration", 60.0)

	p.seekPercent(seekPercentStep) // 10% of 60s = 6s, from 55s: past the end

	if got, want := p.pl.current(), filepath.Join(dir, "b.mp4"); got != want {
		t.Errorf("current() = %q, want %q — the seek past the end did not advance", got, want)
	}
}

// The fix depends on libmpv reporting these two as doubles for a playing file.
func TestObservedPropsArriveAsDoublesForARealFile(t *testing.T) {
	p := headlessPlayer(t)
	if err := p.observePlayback(); err != nil {
		t.Fatal(err)
	}
	if err := p.mpv.Command([]string{"loadfile", encodeClip(t, t.TempDir(), "a.mp4", 5)}); err != nil {
		t.Fatal(err)
	}

	// Drain the queue here rather than running the pump: see above.
	deadline := time.Now().Add(10 * time.Second)
	var gotPos, gotDur bool
	for time.Now().Before(deadline) && (!gotPos || !gotDur) {
		ev := p.mpv.WaitEvent(0.1)
		if ev == nil || ev.EventID != mpv.EventPropertyChange {
			continue
		}
		prop := ev.Property()
		v, ok := prop.Data.(float64)
		if !ok || v <= 0 {
			continue
		}
		switch prop.Name {
		case "time-pos":
			gotPos = true
		case "duration":
			gotDur = true
		}
	}
	if !gotPos || !gotDur {
		t.Errorf("time-pos double: %v, duration double: %v — want both", gotPos, gotDur)
	}
}
