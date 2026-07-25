package player

import (
	"math"
	"testing"
)

func TestParseTrackList(t *testing.T) {
	// Shape mirrors mpv's track-list node: []any of map[string]any, numbers
	// arrive as int64/float64 and flags as bool.
	raw := []any{
		map[string]any{"id": int64(1), "type": "audio", "lang": "eng", "codec": "aac", "selected": true},
		map[string]any{"id": int64(2), "type": "audio", "lang": "ger", "codec": "ac3", "selected": false},
		map[string]any{"id": int64(1), "type": "sub", "lang": "eng", "codec": "subrip", "selected": false, "title": "Full"},
	}
	tracks := parseTrackList(raw)
	if len(tracks) != 3 {
		t.Fatalf("got %d tracks, want 3", len(tracks))
	}
	if tracks[0].kind != "audio" || tracks[0].id != 1 || !tracks[0].selected {
		t.Errorf("track0 = %+v", tracks[0])
	}
	if tracks[2].kind != "sub" || tracks[2].title != "Full" {
		t.Errorf("track2 = %+v", tracks[2])
	}
}

func TestParseTrackListNilAndGarbage(t *testing.T) {
	if got := parseTrackList(nil); got != nil {
		t.Errorf("parseTrackList(nil) = %v, want nil", got)
	}
	if got := parseTrackList("not a list"); got != nil {
		t.Errorf("parseTrackList(string) = %v, want nil", got)
	}
}

func TestTrackLabel(t *testing.T) {
	tr := trackInfo{id: 1, kind: "audio", lang: "eng", codec: "aac"}
	if got := tr.label(); got != "1: eng (aac)" {
		t.Errorf("label = %q, want \"1: eng (aac)\"", got)
	}
	trTitle := trackInfo{id: 2, kind: "sub", title: "Commentary", lang: "eng", codec: "subrip"}
	if got := trTitle.label(); got != "2: Commentary (subrip)" {
		t.Errorf("label = %q, want title-based", got)
	}
	trBare := trackInfo{id: 3, kind: "audio"}
	if got := trBare.label(); got != "3" {
		t.Errorf("bare label = %q, want \"3\"", got)
	}
}

func TestZoomToVideoZoom(t *testing.T) {
	tests := []struct {
		factor float64
		want   float64
	}{
		{1.0, 0},
		{0.5, -1},
		{2.0, 1},
	}
	for _, tt := range tests {
		if got := zoomToVideoZoom(tt.factor); math.Abs(got-tt.want) > 1e-9 {
			t.Errorf("zoomToVideoZoom(%v) = %v, want %v", tt.factor, got, tt.want)
		}
	}
	// 1.5x -> log2(1.5)
	if got := zoomToVideoZoom(1.5); math.Abs(got-math.Log2(1.5)) > 1e-9 {
		t.Errorf("zoomToVideoZoom(1.5) = %v, want log2(1.5)", got)
	}
}
