package player

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

// playerWithPlaylist builds a Player with a playlist over real temp files so
// fileExists checks pass. mpv stays nil: loadFile logs and skips the command.
func playerWithPlaylist(t *testing.T, names ...string) (*Player, string) {
	t.Helper()
	dir := mkFiles(t, names...)
	p := &Player{log: slog.Default(), actions: actionByID()}
	p.pl = scanPlaylist(filepath.Join(dir, names[0]))
	return p, dir
}

func TestAdvanceAfterRemovalPointsAtNextVideo(t *testing.T) {
	p, dir := playerWithPlaylist(t, "a.mp4", "b.mp4", "c.mp4")
	gone := filepath.Join(dir, "a.mp4")
	if err := os.Remove(gone); err != nil {
		t.Fatal(err)
	}
	p.advanceAfterRemoval(gone)
	if got := p.pl.current(); got != filepath.Join(dir, "b.mp4") {
		t.Errorf("current() = %q, want b.mp4", got)
	}
	if len(p.pl.entries) != 2 {
		t.Errorf("removed file still in playlist: %v", p.pl.entries)
	}
}

func TestAdvanceAfterRemovalLastFallsBackToPrevious(t *testing.T) {
	p, dir := playerWithPlaylist(t, "b.mp4", "a.mp4")
	// scanPlaylist positioned at b.mp4 (names[0]); it is the last entry.
	gone := filepath.Join(dir, "b.mp4")
	if err := os.Remove(gone); err != nil {
		t.Fatal(err)
	}
	p.advanceAfterRemoval(gone)
	if got := p.pl.current(); got != filepath.Join(dir, "a.mp4") {
		t.Errorf("current() = %q, want a.mp4", got)
	}
}

func TestAdvanceAfterRemovalOnlyEntryEmptiesPlaylist(t *testing.T) {
	p, dir := playerWithPlaylist(t, "a.mp4")
	gone := filepath.Join(dir, "a.mp4")
	if err := os.Remove(gone); err != nil {
		t.Fatal(err)
	}
	p.advanceAfterRemoval(gone)
	if got := p.pl.current(); got != "" {
		t.Errorf("current() = %q, want empty playlist", got)
	}
}

func TestAdvanceAfterRemovalNilPlaylistIsNoop(t *testing.T) {
	p := &Player{log: slog.Default()}
	p.advanceAfterRemoval("/v/a.mp4") // must not panic
}
