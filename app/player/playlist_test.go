package player

import (
	"os"
	"path/filepath"
	"testing"
)

// mkFiles creates empty files in dir and returns dir.
func mkFiles(t *testing.T, names ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestScanPlaylistListsSortedVideosInFolder(t *testing.T) {
	dir := mkFiles(t, "b.mkv", "a.mp4", "c.webm", "notes.txt", "cover.jpg")
	pl := scanPlaylist(filepath.Join(dir, "b.mkv"))
	if pl == nil {
		t.Fatal("scanPlaylist returned nil for a valid video file")
	}
	want := []string{
		filepath.Join(dir, "a.mp4"),
		filepath.Join(dir, "b.mkv"),
		filepath.Join(dir, "c.webm"),
	}
	if len(pl.entries) != len(want) {
		t.Fatalf("entries = %v, want %v", pl.entries, want)
	}
	for i := range want {
		if pl.entries[i] != want[i] {
			t.Errorf("entries[%d] = %q, want %q", i, pl.entries[i], want[i])
		}
	}
	if got := pl.current(); got != filepath.Join(dir, "b.mkv") {
		t.Errorf("current() = %q, want the opened file", got)
	}
}

func TestScanPlaylistIncludesCurrentFileWithUnknownExtension(t *testing.T) {
	dir := mkFiles(t, "a.mp4", "weird.xyz")
	pl := scanPlaylist(filepath.Join(dir, "weird.xyz"))
	if pl == nil {
		t.Fatal("scanPlaylist returned nil")
	}
	if got := pl.current(); got != filepath.Join(dir, "weird.xyz") {
		t.Errorf("current() = %q, want the opened file even with unknown extension", got)
	}
	if len(pl.entries) != 2 {
		t.Errorf("entries = %v, want opened file plus a.mp4", pl.entries)
	}
}

func TestScanPlaylistNonFileReturnsNil(t *testing.T) {
	if pl := scanPlaylist("http://example.com/video.mp4"); pl != nil {
		t.Errorf("scanPlaylist(url) = %v, want nil", pl.entries)
	}
	if pl := scanPlaylist(t.TempDir()); pl != nil {
		t.Errorf("scanPlaylist(directory) = %v, want nil", pl.entries)
	}
}

func TestFirstVideoReturnsAlphabeticallyFirstVideo(t *testing.T) {
	dir := mkFiles(t, "b.mkv", "a.mp4", "c.webm", "notes.txt", "cover.jpg")
	if got := firstVideo(dir); got != filepath.Join(dir, "a.mp4") {
		t.Errorf("firstVideo = %q, want %q", got, filepath.Join(dir, "a.mp4"))
	}
}

func TestFirstVideoIgnoresNonVideoFiles(t *testing.T) {
	dir := mkFiles(t, "notes.txt", "cover.jpg")
	if got := firstVideo(dir); got != "" {
		t.Errorf("firstVideo = %q, want \"\" when the folder has no videos", got)
	}
}

func TestFirstVideoEmptyFolderReturnsEmpty(t *testing.T) {
	if got := firstVideo(t.TempDir()); got != "" {
		t.Errorf("firstVideo(empty) = %q, want \"\"", got)
	}
}

func TestFirstVideoMissingFolderReturnsEmpty(t *testing.T) {
	if got := firstVideo(filepath.Join(t.TempDir(), "nope")); got != "" {
		t.Errorf("firstVideo(missing) = %q, want \"\"", got)
	}
}

func TestFirstVideoSkipsSubdirectories(t *testing.T) {
	dir := mkFiles(t, "a.mp4")
	// A subdirectory whose name sorts first and looks like a video must not win.
	if err := os.Mkdir(filepath.Join(dir, "0sub.mp4"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := firstVideo(dir); got != filepath.Join(dir, "a.mp4") {
		t.Errorf("firstVideo = %q, want the file a.mp4, not the subdir", got)
	}
}

func TestResolveStartPathEmptyReturnsEmpty(t *testing.T) {
	if got := resolveStartPath(""); got != "" {
		t.Errorf("resolveStartPath(\"\") = %q, want \"\" (bare govi opens the idle screen)", got)
	}
}

func TestResolveStartPathDirectoryReturnsItsFirstVideo(t *testing.T) {
	dir := mkFiles(t, "b.mkv", "a.mp4")
	if got := resolveStartPath(dir); got != filepath.Join(dir, "a.mp4") {
		t.Errorf("resolveStartPath(dir) = %q, want the dir's first video", got)
	}
}

func TestResolveStartPathFilePassesThrough(t *testing.T) {
	dir := mkFiles(t, "a.mp4")
	file := filepath.Join(dir, "a.mp4")
	if got := resolveStartPath(file); got != file {
		t.Errorf("resolveStartPath(file) = %q, want it unchanged", got)
	}
}

func TestResolveStartPathEmptyFolderReturnsEmpty(t *testing.T) {
	if got := resolveStartPath(t.TempDir()); got != "" {
		t.Errorf("resolveStartPath(empty dir) = %q, want \"\"", got)
	}
}

func TestResolveStartPathURLPassesThrough(t *testing.T) {
	const url = "http://example.com/v.mp4"
	if got := resolveStartPath(url); got != url {
		t.Errorf("resolveStartPath(url) = %q, want it unchanged", got)
	}
}

func always(string) bool { return true }

func TestAdvanceMovesThroughEntries(t *testing.T) {
	pl := &playlist{entries: []string{"/v/a.mp4", "/v/b.mp4", "/v/c.mp4"}, idx: 0}
	if got := pl.advance(1, always); got != "/v/b.mp4" {
		t.Errorf("advance(1) = %q, want /v/b.mp4", got)
	}
	if got := pl.advance(1, always); got != "/v/c.mp4" {
		t.Errorf("advance(1) = %q, want /v/c.mp4", got)
	}
	if got := pl.advance(-1, always); got != "/v/b.mp4" {
		t.Errorf("advance(-1) = %q, want /v/b.mp4", got)
	}
}

func TestAdvanceStopsAtEnds(t *testing.T) {
	pl := &playlist{entries: []string{"/v/a.mp4", "/v/b.mp4"}, idx: 1}
	if got := pl.advance(1, always); got != "" {
		t.Errorf("advance(1) at last entry = %q, want \"\"", got)
	}
	if pl.current() != "/v/b.mp4" {
		t.Errorf("current moved after failed advance: %q", pl.current())
	}
	pl.idx = 0
	if got := pl.advance(-1, always); got != "" {
		t.Errorf("advance(-1) at first entry = %q, want \"\"", got)
	}
}

func TestAdvanceSkipsAndDropsMissingFiles(t *testing.T) {
	pl := &playlist{entries: []string{"/v/a.mp4", "/v/gone.mp4", "/v/c.mp4"}, idx: 0}
	exists := func(p string) bool { return p != "/v/gone.mp4" }
	if got := pl.advance(1, exists); got != "/v/c.mp4" {
		t.Errorf("advance(1) = %q, want /v/c.mp4 (skipping missing)", got)
	}
	if len(pl.entries) != 2 {
		t.Errorf("missing entry not dropped: %v", pl.entries)
	}
	if pl.current() != "/v/c.mp4" {
		t.Errorf("current() = %q, want /v/c.mp4", pl.current())
	}
}

func TestAdvanceAllMissingReturnsEmpty(t *testing.T) {
	pl := &playlist{entries: []string{"/v/a.mp4", "/v/gone.mp4"}, idx: 0}
	exists := func(p string) bool { return p == "/v/a.mp4" }
	if got := pl.advance(1, exists); got != "" {
		t.Errorf("advance(1) = %q, want \"\" when everything ahead is missing", got)
	}
	if pl.current() != "/v/a.mp4" {
		t.Errorf("current() = %q, want unchanged /v/a.mp4", pl.current())
	}
}

func TestRemoveCurrentPointsAtFollowingEntry(t *testing.T) {
	pl := &playlist{entries: []string{"/v/a.mp4", "/v/b.mp4", "/v/c.mp4"}, idx: 1}
	pl.remove("/v/b.mp4")
	if got := pl.current(); got != "/v/c.mp4" {
		t.Errorf("current() after removing current = %q, want /v/c.mp4", got)
	}
}

func TestRemoveLastEntryFallsBackToPrevious(t *testing.T) {
	pl := &playlist{entries: []string{"/v/a.mp4", "/v/b.mp4"}, idx: 1}
	pl.remove("/v/b.mp4")
	if got := pl.current(); got != "/v/a.mp4" {
		t.Errorf("current() after removing last = %q, want /v/a.mp4", got)
	}
}

func TestRemoveBeforeCurrentKeepsCurrent(t *testing.T) {
	pl := &playlist{entries: []string{"/v/a.mp4", "/v/b.mp4", "/v/c.mp4"}, idx: 2}
	pl.remove("/v/a.mp4")
	if got := pl.current(); got != "/v/c.mp4" {
		t.Errorf("current() = %q, want /v/c.mp4 unchanged", got)
	}
}

func TestRemoveOnlyEntryEmptiesPlaylist(t *testing.T) {
	pl := &playlist{entries: []string{"/v/a.mp4"}, idx: 0}
	pl.remove("/v/a.mp4")
	if got := pl.current(); got != "" {
		t.Errorf("current() on empty playlist = %q, want \"\"", got)
	}
}

func TestRemoveUnknownPathIsNoop(t *testing.T) {
	pl := &playlist{entries: []string{"/v/a.mp4", "/v/b.mp4"}, idx: 1}
	pl.remove("/v/nope.mp4")
	if got := pl.current(); got != "/v/b.mp4" {
		t.Errorf("current() = %q, want /v/b.mp4 unchanged", got)
	}
}
