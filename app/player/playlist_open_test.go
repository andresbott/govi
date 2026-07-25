package player

import (
	"log/slog"
	"path/filepath"
	"testing"
)

func TestOpenFileBuildsPlaylistFromFolder(t *testing.T) {
	dir := mkFiles(t, "a.mp4", "b.mp4")
	p := &Player{log: slog.Default()}
	p.openFile(filepath.Join(dir, "b.mp4"))
	if p.pl == nil {
		t.Fatal("openFile did not build a playlist")
	}
	if got := p.pl.current(); got != filepath.Join(dir, "b.mp4") {
		t.Errorf("current() = %q, want the opened file", got)
	}
	if len(p.pl.entries) != 2 {
		t.Errorf("entries = %v, want both videos in folder", p.pl.entries)
	}
}

func TestOpenFileNonLocalClearsPlaylist(t *testing.T) {
	dir := mkFiles(t, "a.mp4")
	p := &Player{log: slog.Default()}
	p.openFile(filepath.Join(dir, "a.mp4"))
	p.openFile("http://example.com/stream.mp4")
	if p.pl != nil {
		t.Errorf("playlist should be cleared for non-local source, got %v", p.pl.entries)
	}
}
