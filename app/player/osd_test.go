package player

import (
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestVolumeStatusShowsLevel(t *testing.T) {
	if got := volumeStatus(45, false); got != "Volume 45%" {
		t.Errorf("volumeStatus(45, false) = %q, want \"Volume 45%%\"", got)
	}
}

func TestVolumeStatusMutedHidesLevel(t *testing.T) {
	if got := volumeStatus(45, true); got != "Muted" {
		t.Errorf("volumeStatus(45, true) = %q, want \"Muted\"", got)
	}
}

func TestPositionStatusIsOneBasedWithFileName(t *testing.T) {
	got := positionStatus(0, 3, "/v/a.mp4")
	if !strings.HasPrefix(got, "1 / 3") {
		t.Errorf("positionStatus(0, 3, ...) = %q, want it to start with \"1 / 3\"", got)
	}
	if !strings.Contains(got, "a.mp4") {
		t.Errorf("positionStatus = %q, want the file name", got)
	}
	if strings.Contains(got, "/v/") {
		t.Errorf("positionStatus = %q, want the base name only, not the path", got)
	}
}

func TestProgressStatusShowsElapsedTotalAndPercent(t *testing.T) {
	if got, want := progressStatus(30, 120), "0:30 / 2:00   25%"; got != want {
		t.Errorf("progressStatus(30, 120) = %q, want %q", got, want)
	}
}

func TestProgressStatusRoundsPercent(t *testing.T) {
	// 1/3 of the way in reads as 33%, not 33.33 or a truncated 32.
	if got, want := progressStatus(40, 120), "0:40 / 2:00   33%"; got != want {
		t.Errorf("progressStatus(40, 120) = %q, want %q", got, want)
	}
}

func TestProgressStatusUnknownDurationOmitsPercent(t *testing.T) {
	// A live stream has no duration: show the elapsed clock, never a 0% or NaN.
	got := progressStatus(42, 0)
	if got != "0:42" {
		t.Errorf("progressStatus(42, 0) = %q, want \"0:42\"", got)
	}
	if strings.Contains(got, "%") {
		t.Errorf("progressStatus with unknown duration = %q, want no percentage", got)
	}
}

func TestFlashProgressWithoutMpvIsNoop(t *testing.T) {
	p := &Player{}
	p.flashProgress() // mpv is nil in tests; must not panic
	if p.osdVisible(time.Now()) {
		t.Errorf("flashProgress without mpv flashed %q", p.osdText)
	}
	if p.osdProgress {
		t.Error("flashProgress without mpv marked the flash live")
	}
}

func TestRefreshOSDStopsTrackingOnceTheFlashExpired(t *testing.T) {
	p := &Player{osdText: "0:30 / 2:00   25%", osdProgress: true}
	p.osdUntil = time.Now()

	// Expired: refresh must clear the live flag instead of reading mpv (nil here,
	// so a read would panic).
	p.refreshOSD(time.Now().Add(time.Second))

	if p.osdProgress {
		t.Error("refreshOSD kept tracking progress after the flash expired")
	}
}

func TestFlashClearsTheProgressTracking(t *testing.T) {
	p := &Player{osdProgress: true}
	p.flash("Muted") // a snapshot flash replaces a live one
	if p.osdProgress {
		t.Error("a plain flash left progress tracking on, so it would overwrite itself")
	}
}

func TestFlashIsVisibleThenExpires(t *testing.T) {
	p := &Player{}
	p.flash("hello")
	now := time.Now()
	if !p.osdVisible(now) {
		t.Error("flash not visible right after being set")
	}
	if p.osdVisible(now.Add(osdDuration + time.Second)) {
		t.Error("flash still visible after osdDuration elapsed")
	}
}

func TestOsdNotVisibleWithoutAFlash(t *testing.T) {
	p := &Player{}
	if p.osdVisible(time.Now()) {
		t.Error("osdVisible with no flash set")
	}
}

func TestPlayAdjacentFlashesPosition(t *testing.T) {
	p, dir := playerWithPlaylist(t, "a.mp4", "b.mp4", "c.mp4")

	p.playAdjacent(1)

	if !p.osdVisible(time.Now()) {
		t.Fatal("advancing the playlist did not flash a status")
	}
	if want := "2 / 3"; !strings.Contains(p.osdText, want) {
		t.Errorf("osdText = %q, want it to contain %q", p.osdText, want)
	}
	if !strings.Contains(p.osdText, filepath.Base(filepath.Join(dir, "b.mp4"))) {
		t.Errorf("osdText = %q, want the new file name", p.osdText)
	}
}

func TestPlayAdjacentAtEndDoesNotFlash(t *testing.T) {
	p, _ := playerWithPlaylist(t, "a.mp4", "b.mp4")

	p.playAdjacent(-1) // already on the first entry

	if p.osdVisible(time.Now()) {
		t.Errorf("a failed advance flashed %q", p.osdText)
	}
}

func TestFlashPositionWithoutPlaylistIsNoop(t *testing.T) {
	p := &Player{log: slog.Default()}
	p.flashPosition() // must not panic
	if p.osdVisible(time.Now()) {
		t.Errorf("flashPosition with no playlist flashed %q", p.osdText)
	}
}

func TestFlashVolumeWithoutMpvReportsMuteState(t *testing.T) {
	p := &Player{muted: true}
	p.flashVolume() // mpv is nil in tests; must not panic
	if p.osdText != "Muted" {
		t.Errorf("osdText = %q, want \"Muted\"", p.osdText)
	}
}
