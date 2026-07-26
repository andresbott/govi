package player

import (
	"strings"
	"testing"
)

// section finds a section by title, failing the test when it is missing.
func section(t *testing.T, secs []infoSection, title string) infoSection {
	t.Helper()
	for _, s := range secs {
		if s.title == title {
			return s
		}
	}
	t.Fatalf("no %q section in %v", title, secs)
	return infoSection{}
}

// rowValue returns the value of the first row with the given label.
func rowValue(t *testing.T, sec infoSection, label string) string {
	t.Helper()
	for _, r := range sec.rows {
		if r.label == label {
			return r.value
		}
	}
	t.Fatalf("no %q row in section %q: %v", label, sec.title, sec.rows)
	return ""
}

func TestInfoSectionsGrouping(t *testing.T) {
	secs := infoSections(mediaInfo{
		path:     "/home/u/videos/clip.mkv",
		filename: "clip.mkv",
		duration: 3725,
		fileSize: 1572864,
		width:    1920, height: 1080, fps: 23.976,
		videoBitrate:    5_000_000,
		audioBitrate:    192_000,
		audioChannels:   "2",
		audioSampleRate: 48000,
		tracks: []trackInfo{
			{id: 1, kind: "video", codec: "h264", selected: true},
			{id: 1, kind: "audio", lang: "eng", codec: "aac", selected: true},
			{id: 2, kind: "audio", lang: "ger", codec: "ac3"},
			{id: 1, kind: "sub", lang: "eng", codec: "subrip", title: "Full"},
		},
	})

	var titles []string
	for _, s := range secs {
		titles = append(titles, s.title)
	}
	want := []string{"File", "Video", "Audio", "Subtitles"}
	if strings.Join(titles, ",") != strings.Join(want, ",") {
		t.Errorf("sections = %v, want %v", titles, want)
	}

	file := section(t, secs, "File")
	if got := rowValue(t, file, "Name"); got != "clip.mkv" {
		t.Errorf("Name = %q, want file name only", got)
	}
	if got := rowValue(t, file, "Folder"); got != "/home/u/videos" {
		t.Errorf("Folder = %q, want folder only", got)
	}
	if got := rowValue(t, file, "Duration"); got != "1:02:05" {
		t.Errorf("Duration = %q, want 1:02:05", got)
	}
	if got := rowValue(t, file, "Size"); got != "1.5 MiB" {
		t.Errorf("Size = %q", got)
	}

	video := section(t, secs, "Video")
	if got := rowValue(t, video, "Resolution"); got != "1920×1080 @ 24 fps" {
		t.Errorf("Resolution = %q", got)
	}
	if got := rowValue(t, video, "Bitrate"); got != "5.0 Mbps" {
		t.Errorf("video Bitrate = %q", got)
	}
	if got := rowValue(t, video, "▶ Track 1"); got != "h264" {
		t.Errorf("video track = %q", got)
	}

	// Both audio tracks are listed, with the playing one marked.
	audio := section(t, secs, "Audio")
	if got := rowValue(t, audio, "▶ Track 1"); got != "aac, eng" {
		t.Errorf("audio track 1 = %q", got)
	}
	if got := rowValue(t, audio, "   Track 2"); got != "ac3, ger" {
		t.Errorf("audio track 2 = %q", got)
	}
	if got := rowValue(t, audio, "Channels"); got != "2 ch, 48.0 kHz" {
		t.Errorf("Channels = %q", got)
	}
	if got := rowValue(t, audio, "Bitrate"); got != "192 kbps" {
		t.Errorf("audio Bitrate = %q", got)
	}

	subs := section(t, secs, "Subtitles")
	if got := rowValue(t, subs, "   Track 1"); got != "subrip, eng, Full" {
		t.Errorf("sub track = %q", got)
	}
}

func TestInfoSectionsEmpty(t *testing.T) {
	// Nothing reported: every field renders "—" and every kind says "none",
	// so an empty section is never mistaken for missing data.
	secs := infoSections(mediaInfo{})
	for _, tt := range []struct{ sec, label, want string }{
		{"File", "Name", "—"},
		{"File", "Folder", "—"},
		{"File", "Duration", "—"},
		{"File", "Size", "—"},
		{"Video", "Tracks", "none"},
		{"Video", "Resolution", "—"},
		{"Video", "Bitrate", "—"},
		{"Audio", "Tracks", "none"},
		{"Audio", "Bitrate", "—"},
		{"Subtitles", "Tracks", "none"},
	} {
		if got := rowValue(t, section(t, secs, tt.sec), tt.label); got != tt.want {
			t.Errorf("%s/%s = %q, want %q", tt.sec, tt.label, got, tt.want)
		}
	}
	// No channel/sample-rate report means no Channels row at all.
	for _, r := range section(t, secs, "Audio").rows {
		if r.label == "Channels" {
			t.Errorf("Channels row present with nothing reported: %v", r)
		}
	}
}

func TestInfoSectionsRelativePathAndURL(t *testing.T) {
	// A file loaded by bare name has no folder to show, and mpv's filename is
	// the fallback when `path` is empty.
	secs := infoSections(mediaInfo{path: "clip.mp4", filename: "clip.mp4"})
	file := section(t, secs, "File")
	if got := rowValue(t, file, "Name"); got != "clip.mp4" {
		t.Errorf("Name = %q", got)
	}
	if got := rowValue(t, file, "Folder"); got != "—" {
		t.Errorf("Folder = %q, want dash", got)
	}

	secs = infoSections(mediaInfo{path: "", filename: "fallback.mkv"})
	if got := rowValue(t, section(t, secs, "File"), "Name"); got != "fallback.mkv" {
		t.Errorf("Name = %q, want the filename fallback", got)
	}

	secs = infoSections(mediaInfo{path: "https://example.com/live.m3u8"})
	file = section(t, secs, "File")
	if got := rowValue(t, file, "Name"); got != "https://example.com/live.m3u8" {
		t.Errorf("URL Name = %q, want the whole URL", got)
	}
	if got := rowValue(t, file, "Folder"); got != "—" {
		t.Errorf("URL Folder = %q, want dash", got)
	}
}

func TestAudioLayoutPartialData(t *testing.T) {
	tests := []struct {
		name string
		in   mediaInfo
		want string
	}{
		{"both", mediaInfo{audioChannels: "6", audioSampleRate: 44100}, "6 ch, 44.1 kHz"},
		{"channels only", mediaInfo{audioChannels: "2"}, "2 ch"},
		{"rate only", mediaInfo{audioSampleRate: 48000}, "48.0 kHz"},
		{"neither", mediaInfo{}, ""},
	}
	for _, tt := range tests {
		if got := tt.in.audioLayout(); got != tt.want {
			t.Errorf("%s: audioLayout = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestResolutionPartialData(t *testing.T) {
	tests := []struct {
		name string
		in   mediaInfo
		want string
	}{
		{"full", mediaInfo{width: 1280, height: 720, fps: 30}, "1280×720 @ 30 fps"},
		{"no fps", mediaInfo{width: 1280, height: 720}, "1280×720"},
		{"no size", mediaInfo{fps: 30}, "—"},
		{"zero height", mediaInfo{width: 1280}, "—"},
	}
	for _, tt := range tests {
		if got := tt.in.resolution(); got != tt.want {
			t.Errorf("%s: resolution = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestTrackRowsListsEveryTrack(t *testing.T) {
	rows := trackRows([]trackInfo{
		{id: 1, kind: "audio", codec: "aac", lang: "eng", selected: true},
		{id: 2, kind: "audio", codec: "ac3"},
		{id: 3, kind: "audio"},
	})
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want one per track: %v", len(rows), rows)
	}
	if rows[0].label != "▶ Track 1" || rows[0].value != "aac, eng" {
		t.Errorf("row0 = %+v", rows[0])
	}
	if rows[1].label != "   Track 2" || rows[1].value != "ac3" {
		t.Errorf("row1 = %+v", rows[1])
	}
	// A track mpv described with nothing but an id still gets a row.
	if rows[2].value != "—" {
		t.Errorf("row2 value = %q, want dash", rows[2].value)
	}
}

func TestTracksOfKind(t *testing.T) {
	tracks := []trackInfo{
		{id: 1, kind: "video"},
		{id: 1, kind: "audio"},
		{id: 2, kind: "audio"},
		{id: 1, kind: "sub"},
	}
	if got := tracksOfKind(tracks, "audio"); len(got) != 2 || got[0].id != 1 || got[1].id != 2 {
		t.Errorf("audio tracks = %+v", got)
	}
	if got := tracksOfKind(tracks, "sub"); len(got) != 1 {
		t.Errorf("sub tracks = %+v", got)
	}
	if got := tracksOfKind(tracks, "nope"); got != nil {
		t.Errorf("unknown kind = %+v, want nil", got)
	}
}
