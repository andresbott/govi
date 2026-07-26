package player

import (
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	mpv "github.com/gen2brain/go-mpv"
)

// encodeClip writes a real encoded video of the given duration, so seeks hit
// keyframe snapping the way they do on user files (a synthetic lavfi source does
// not). It skips the test when ffmpeg is unavailable.
func encodeClip(t *testing.T, dir, name string, seconds int) string {
	t.Helper()
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not installed")
	}
	path := filepath.Join(dir, name)
	cmd := exec.Command(ffmpeg, "-y",
		"-f", "lavfi", "-i", "testsrc=duration="+itoa(seconds)+":size=160x120:rate=15",
		"-c:v", "libx264", "-pix_fmt", "yuv420p", "-loglevel", "error", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("ffmpeg failed: %v: %s", err, out)
	}
	return path
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// headlessPlayer builds a Player driving a real libmpv with no video output, so
// the seek path can be exercised without a display.
func headlessPlayer(t *testing.T) *Player {
	t.Helper()
	m := mpv.New()
	for k, v := range map[string]string{"vo": "null", "ao": "null", "idle": "yes"} {
		if err := m.SetOptionString(k, v); err != nil {
			t.Fatal(err)
		}
	}
	if err := m.Initialize(); err != nil {
		t.Skipf("cannot initialize headless mpv: %v", err)
	}
	t.Cleanup(func() {
		m.Command([]string{"quit"}) //nolint:errcheck // best-effort shutdown
		for {
			ev := m.WaitEvent(1)
			if ev == nil || ev.EventID == mpv.EventShutdown {
				break
			}
		}
		m.TerminateDestroy()
	})
	return &Player{log: slog.Default(), mpv: m}
}

// waitPlaying blocks until mpv reports a playable position, so a seek in the
// test acts on a started file rather than being dropped.
func waitPlaying(t *testing.T, p *Player) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		ev := p.mpv.WaitEvent(0.1)
		if ev != nil && ev.EventID == mpv.EventPlaybackRestart {
			return
		}
	}
	t.Fatal("mpv never started playback")
}

// TestSeekPercentPastEndAdvancesOnRealFile is the regression test for
// shift+Right at the end of a file: mpv clamps a relative-percent seek to the
// last keyframe instead of overshooting the end, so it never reports
// end-of-file and auto-advance alone would leave the tail of the file playing.
// The seek itself has to switch to the next entry.
func TestSeekPercentPastEndAdvancesOnRealFile(t *testing.T) {
	dir := t.TempDir()
	first := encodeClip(t, dir, "a.mp4", 20)
	second := encodeClip(t, dir, "b.mp4", 5)

	p := headlessPlayer(t)
	p.pl = scanPlaylist(first)
	if p.pl.current() != first {
		t.Fatalf("playlist starts at %q, want %q", p.pl.current(), first)
	}
	if err := p.mpv.Command([]string{"loadfile", first}); err != nil {
		t.Fatal(err)
	}
	waitPlaying(t, p)

	// Land 1s from the end. "exact" matters: a plain seek snaps to the nearest
	// keyframe, which on a short clip can be many seconds earlier and would put
	// the position back inside the file.
	if err := p.mpv.Command([]string{"seek", "19", "absolute", "exact"}); err != nil {
		t.Fatal(err)
	}
	waitPlaying(t, p)
	if pos := p.propFloat("time-pos"); pos < 18 {
		t.Fatalf("setup seek landed at %.2fs, want ~19s (near the end)", pos)
	}

	// 10% of 20s = 2s, from ~19s: past the end.
	p.seekPercent(seekPercentStep)

	if got := p.pl.current(); got != second {
		t.Errorf("current() = %q, want %q — the seek past the end did not continue with the next video", got, second)
	}
}

// A percentage seek in the middle of the file must still be a normal seek.
func TestSeekPercentInsideFileStaysOnFile(t *testing.T) {
	dir := t.TempDir()
	first := encodeClip(t, dir, "a.mp4", 20)
	encodeClip(t, dir, "b.mp4", 5)

	p := headlessPlayer(t)
	p.pl = scanPlaylist(first)
	if err := p.mpv.Command([]string{"loadfile", first}); err != nil {
		t.Fatal(err)
	}
	waitPlaying(t, p)

	p.seekPercent(seekPercentStep)

	if got := p.pl.current(); got != first {
		t.Errorf("current() = %q, want %q — a mid-file seek must not change entry", got, first)
	}
	if _, err := os.Stat(first); err != nil {
		t.Errorf("source file disturbed: %v", err)
	}
}
