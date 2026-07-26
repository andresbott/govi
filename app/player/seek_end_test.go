package player

import (
	"path/filepath"
	"testing"
)

func TestPassesEnd(t *testing.T) {
	tests := []struct {
		name           string
		pos, dur, step float64
		want           bool
	}{
		{"forward past the end", 55, 60, 10, true},
		{"forward landing exactly on the end", 50, 60, 10, true},
		{"forward staying inside the file", 10, 60, 10, false},
		{"backwards", 55, 60, -10, false},
		{"backwards past the start", 2, 60, -10, false},
		{"live stream with no duration", 55, 0, 10, false},
		{"duration mpv does not know yet", 55, -1, 10, false},
		{"zero step", 60, 60, 0, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := passesEnd(tc.pos, tc.dur, tc.step); got != tc.want {
				t.Errorf("passesEnd(%v, %v, %v) = %v, want %v", tc.pos, tc.dur, tc.step, got, tc.want)
			}
		})
	}
}

func TestPercentDelta(t *testing.T) {
	tests := []struct {
		name string
		dur  float64
		pct  int
		want float64
	}{
		{"ten percent of a minute", 60, 10, 6},
		{"backwards", 60, -10, -6},
		{"unknown duration", 0, 10, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := percentDelta(tc.dur, tc.pct); got != tc.want {
				t.Errorf("percentDelta(%v, %d) = %v, want %v", tc.dur, tc.pct, got, tc.want)
			}
		})
	}
}

func TestHasNext(t *testing.T) {
	tests := []struct {
		name string
		pl   *playlist
		want bool
	}{
		{"entry follows", &playlist{entries: []string{"/v/a.mp4", "/v/b.mp4"}, idx: 0}, true},
		{"last entry", &playlist{entries: []string{"/v/a.mp4", "/v/b.mp4"}, idx: 1}, false},
		{"single entry", &playlist{entries: []string{"/v/a.mp4"}, idx: 0}, false},
		{"empty playlist", &playlist{}, false},
		{"no playlist at all", nil, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.pl.hasNext(); got != tc.want {
				t.Errorf("hasNext() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPlayAdjacentReportsWhetherItMoved(t *testing.T) {
	p, _ := playerWithPlaylist(t, "a.mp4", "b.mp4")
	if !p.playAdjacent(1) {
		t.Error("playAdjacent(1) = false, want true when an entry follows")
	}
	if p.playAdjacent(1) {
		t.Error("playAdjacent(1) = true at the last entry, want false")
	}
	noPlaylist := &Player{}
	if noPlaylist.playAdjacent(1) {
		t.Error("playAdjacent(1) = true without a playlist, want false")
	}
}

// A forward seek that runs off the end continues with the next entry instead of
// being sent to mpv, which would clamp it to just before the end (a percentage
// seek snaps to the last keyframe) and leave the tail of the file playing.
func TestSeekPastEndPlaysNextEntry(t *testing.T) {
	p, dir := playerWithPlaylist(t, "a.mp4", "b.mp4")

	if !p.advancePastEnd(55, 60, 10) {
		t.Fatal("advancePastEnd = false, want true (seek runs off the end, b.mp4 follows)")
	}
	if got := p.pl.current(); got != filepath.Join(dir, "b.mp4") {
		t.Errorf("current() = %q, want b.mp4", got)
	}
}

func TestSeekInsideFileDoesNotAdvance(t *testing.T) {
	p, dir := playerWithPlaylist(t, "a.mp4", "b.mp4")

	if p.advancePastEnd(10, 60, 5) {
		t.Error("advancePastEnd = true for a seek that stays inside the file")
	}
	if got := p.pl.current(); got != filepath.Join(dir, "a.mp4") {
		t.Errorf("current() = %q, want a.mp4 unchanged", got)
	}
}

// At the last entry the seek must reach mpv: there is nothing to continue with,
// so mpv's own end-of-file handling (idle screen) is the right outcome.
func TestSeekPastEndAtLastEntryFallsThroughToMpv(t *testing.T) {
	// scanPlaylist positions at names[0]; b.mp4 sorts last.
	p, _ := playerWithPlaylist(t, "b.mp4", "a.mp4")

	if p.advancePastEnd(55, 60, 10) {
		t.Error("advancePastEnd = true at the last entry, want the seek sent to mpv")
	}
}

func TestSeekPastEndWithoutPlaylistFallsThroughToMpv(t *testing.T) {
	p := &Player{} // URL source: no playlist
	if p.advancePastEnd(55, 60, 10) {
		t.Error("advancePastEnd = true without a playlist, want the seek sent to mpv")
	}
}

func TestSeekPastEndOnLiveStreamFallsThroughToMpv(t *testing.T) {
	p, _ := playerWithPlaylist(t, "a.mp4", "b.mp4")
	// A stream mpv reports no duration for: there is no end to run off.
	if p.advancePastEnd(55, 0, 10) {
		t.Error("advancePastEnd = true with an unknown duration, want the seek sent to mpv")
	}
}
