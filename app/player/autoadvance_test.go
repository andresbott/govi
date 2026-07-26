package player

import (
	"log/slog"
	"path/filepath"
	"testing"

	mpv "github.com/gen2brain/go-mpv"
)

func TestEndsWithEOF(t *testing.T) {
	tests := []struct {
		name   string
		reason mpv.Reason
		want   bool
	}{
		{"played to the end", mpv.EndFileEOF, true},
		{"replaced by loadfile", mpv.EndFileStop, false},
		{"player quitting", mpv.EndFileQuit, false},
		{"unplayable file", mpv.EndFileError, false},
		{"redirected to a playlist", mpv.EndFileRedirect, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := endsWithEOF(tc.reason); got != tc.want {
				t.Errorf("endsWithEOF(%v) = %v, want %v", tc.reason, got, tc.want)
			}
		})
	}
}

func TestNoteEndFileOnlyArmsOnEOF(t *testing.T) {
	p := &Player{log: slog.Default()}
	p.noteEndFile(mpv.EndFileStop)
	if p.eofPending.Load() {
		t.Error("a stop end-file armed the auto-advance")
	}
	p.noteEndFile(mpv.EndFileEOF)
	if !p.eofPending.Load() {
		t.Error("an eof end-file did not arm the auto-advance")
	}
}

func TestHandleEndOfFilePlaysTheNextEntry(t *testing.T) {
	p, dir := playerWithPlaylist(t, "a.mp4", "b.mp4", "c.mp4")
	p.eofPending.Store(true)

	p.handleEndOfFile()

	if got := p.pl.current(); got != filepath.Join(dir, "b.mp4") {
		t.Errorf("current() = %q, want b.mp4", got)
	}
	if p.eofPending.Load() {
		t.Error("eofPending still armed after handling")
	}
}

func TestHandleEndOfFileWithoutEOFStaysPut(t *testing.T) {
	p, dir := playerWithPlaylist(t, "a.mp4", "b.mp4")

	p.handleEndOfFile()

	if got := p.pl.current(); got != filepath.Join(dir, "a.mp4") {
		t.Errorf("current() = %q, want a.mp4 unchanged", got)
	}
}

func TestHandleEndOfFileAtLastEntryStaysPut(t *testing.T) {
	// scanPlaylist positions at names[0]; b.mp4 sorts last.
	p, dir := playerWithPlaylist(t, "b.mp4", "a.mp4")
	p.eofPending.Store(true)

	p.handleEndOfFile()

	if got := p.pl.current(); got != filepath.Join(dir, "b.mp4") {
		t.Errorf("current() = %q, want b.mp4 (nothing follows it)", got)
	}
	if p.eofPending.Load() {
		t.Error("eofPending still armed after handling")
	}
}

func TestHandleEndOfFileWithoutPlaylistIsNoop(t *testing.T) {
	p := &Player{log: slog.Default()}
	p.eofPending.Store(true)

	p.handleEndOfFile() // a URL source has no playlist: must not panic

	if p.eofPending.Load() {
		t.Error("eofPending still armed after handling")
	}
}

func TestLoadFileDisarmsPendingEOF(t *testing.T) {
	p, dir := playerWithPlaylist(t, "a.mp4", "b.mp4", "c.mp4")
	// The file ends just as the user presses next: mpv's eof arrives, then the
	// keypress loads the following entry itself. The stale flag must not make
	// the loop advance a second time and skip an entry.
	p.eofPending.Store(true)

	p.playAdjacent(1) // user-driven: now on b.mp4
	p.handleEndOfFile()

	if got := p.pl.current(); got != filepath.Join(dir, "b.mp4") {
		t.Errorf("current() = %q, want b.mp4 (a stale eof skipped an entry)", got)
	}
}

func TestNoteFileStartDisarmsPendingEOF(t *testing.T) {
	p := &Player{log: slog.Default()}
	// mpv reports the end of the outgoing file and the start of the incoming
	// one on the same queue: a file that has started playing means whatever
	// ended before it is no longer something to react to.
	p.eofPending.Store(true)

	p.noteFileStart()

	if p.eofPending.Load() {
		t.Error("a file starting left the auto-advance armed")
	}
}

// TestSeekPastEndReportsEOF pins the behaviour auto-advance relies on for
// keyboard navigation: a seek beyond the end of the file ends it with the same
// reason as playing it through, so advancing on eof also covers a user seeking
// off the end. Headless mpv (vo/ao null), no display needed.
func TestSeekPastEndReportsEOF(t *testing.T) {
	m := mpv.New()
	for k, v := range map[string]string{"vo": "null", "ao": "null", "idle": "yes"} {
		if err := m.SetOptionString(k, v); err != nil {
			t.Fatal(err)
		}
	}
	if err := m.Initialize(); err != nil {
		t.Skipf("cannot initialize headless mpv: %v", err)
	}
	defer m.TerminateDestroy()

	// A 10 s synthetic source, so the seek below lands well past its end
	// without the test having to wait for playback.
	if err := m.Command([]string{"loadfile", "av://lavfi:testsrc=duration=10:size=64x64:rate=10"}); err != nil {
		t.Fatal(err)
	}

	seeked := false
	for {
		ev := m.WaitEvent(5)
		if ev == nil {
			t.Fatal("no end-file event within the timeout")
		}
		switch ev.EventID {
		case mpv.EventFileLoaded:
			if !seeked {
				seeked = true
				if err := m.Command(seekCommand(3600)); err != nil {
					t.Fatalf("seek: %v", err)
				}
			}
		case mpv.EventEnd:
			reason := ev.EndFile().Reason
			if !endsWithEOF(reason) {
				t.Fatalf("seek past the end ended with reason %v, want one auto-advance reacts to", reason)
			}
			m.Command([]string{"quit"}) //nolint:errcheck // best-effort shutdown
			for {
				e := m.WaitEvent(5)
				if e == nil || e.EventID == mpv.EventShutdown {
					return
				}
			}
		case mpv.EventShutdown:
			t.Fatal("mpv shut down before reporting end-file")
		}
	}
}
