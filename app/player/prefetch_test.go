package player

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeSized creates a file of size bytes in a temp dir and returns its path.
func writeSized(t *testing.T, name string, size int) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, bytes.Repeat([]byte("v"), size), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestWarmFileReadsHeadAndTailOfLargeFile(t *testing.T) {
	path := writeSized(t, "big.mkv", prefetchHead+prefetchTail+4096)
	n, err := warmFile(context.Background(), path)
	if err != nil {
		t.Fatalf("warmFile: %v", err)
	}
	if want := int64(prefetchHead + prefetchTail); n != want {
		t.Errorf("warmFile read %d bytes, want %d (head + tail)", n, want)
	}
}

func TestWarmFileReadsSmallFileOnce(t *testing.T) {
	const size = 4096
	path := writeSized(t, "small.mkv", size)
	n, err := warmFile(context.Background(), path)
	if err != nil {
		t.Fatalf("warmFile: %v", err)
	}
	if n != size {
		t.Errorf("warmFile read %d bytes, want %d (whole file, no double read)", n, size)
	}
}

func TestWarmFileLeavesFileUntouched(t *testing.T) {
	const size = 4096
	path := writeSized(t, "keep.mkv", size)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := warmFile(context.Background(), path); err != nil {
		t.Fatalf("warmFile: %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("file gone after warmFile: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Error("warmFile modified the file contents")
	}
}

func TestWarmFileMissingPathReturnsError(t *testing.T) {
	if _, err := warmFile(context.Background(), filepath.Join(t.TempDir(), "nope.mkv")); err == nil {
		t.Error("warmFile on a missing path returned nil error")
	}
}

func TestWarmFileCancelledContextStopsEarly(t *testing.T) {
	path := writeSized(t, "big.mkv", prefetchHead+prefetchTail+4096)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	n, err := warmFile(ctx, path)
	if err == nil {
		t.Error("warmFile with a cancelled context returned nil error")
	}
	if n != 0 {
		t.Errorf("warmFile read %d bytes despite a cancelled context, want 0", n)
	}
}

func TestNeighborsReturnsNextThenPrevious(t *testing.T) {
	pl := &playlist{entries: []string{"/v/a.mp4", "/v/b.mp4", "/v/c.mp4"}, idx: 1}
	got := pl.neighbors()
	want := []string{"/v/c.mp4", "/v/a.mp4"}
	if len(got) != len(want) {
		t.Fatalf("neighbors() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("neighbors()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestNeighborsAtFirstEntryReturnsOnlyNext(t *testing.T) {
	pl := &playlist{entries: []string{"/v/a.mp4", "/v/b.mp4"}, idx: 0}
	if got := pl.neighbors(); len(got) != 1 || got[0] != "/v/b.mp4" {
		t.Errorf("neighbors() = %v, want [/v/b.mp4]", got)
	}
}

func TestNeighborsAtLastEntryReturnsOnlyPrevious(t *testing.T) {
	pl := &playlist{entries: []string{"/v/a.mp4", "/v/b.mp4"}, idx: 1}
	if got := pl.neighbors(); len(got) != 1 || got[0] != "/v/a.mp4" {
		t.Errorf("neighbors() = %v, want [/v/a.mp4]", got)
	}
}

func TestNeighborsSingleEntryReturnsNone(t *testing.T) {
	pl := &playlist{entries: []string{"/v/a.mp4"}, idx: 0}
	if got := pl.neighbors(); len(got) != 0 {
		t.Errorf("neighbors() = %v, want none", got)
	}
}

// recorder collects the paths a prefetcher asks to warm.
type recorder struct {
	paths chan string
}

func newRecorder() *recorder { return &recorder{paths: make(chan string, 8)} }

func (r *recorder) warm(_ context.Context, path string) (int64, error) {
	r.paths <- path
	return 0, nil
}

// next returns the next warmed path, failing the test if none arrives.
func (r *recorder) next(t *testing.T) string {
	t.Helper()
	select {
	case p := <-r.paths:
		return p
	case <-time.After(2 * time.Second):
		t.Fatal("no path warmed")
		return ""
	}
}

func TestPrefetcherStartWarmsEveryPath(t *testing.T) {
	rec := newRecorder()
	pf := &prefetcher{warm: rec.warm}
	pf.start([]string{"/v/b.mp4", "/v/a.mp4"})

	got := map[string]bool{rec.next(t): true, rec.next(t): true}
	if !got["/v/a.mp4"] || !got["/v/b.mp4"] {
		t.Errorf("warmed %v, want both /v/a.mp4 and /v/b.mp4", got)
	}
}

func TestPrefetcherStartCancelsThePreviousGeneration(t *testing.T) {
	started := make(chan context.Context, 1)
	blocked := &prefetcher{warm: func(ctx context.Context, _ string) (int64, error) {
		started <- ctx
		<-ctx.Done() // hold the generation open until it is cancelled
		return 0, ctx.Err()
	}}
	blocked.start([]string{"/v/slow.mp4"})

	var first context.Context
	select {
	case first = <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("first generation never started")
	}

	blocked.start([]string{"/v/other.mp4"})
	select {
	case <-first.Done():
	case <-time.After(2 * time.Second):
		t.Error("starting a new generation did not cancel the previous one")
	}
}

func TestPlayAdjacentPrefetchesNeighborsOfTheNewEntry(t *testing.T) {
	p, dir := playerWithPlaylist(t, "a.mp4", "b.mp4", "c.mp4")
	rec := newRecorder()
	p.pf.warm = rec.warm

	p.playAdjacent(1) // now on b.mp4; neighbours are c.mp4 then a.mp4

	got := map[string]bool{rec.next(t): true, rec.next(t): true}
	if !got[filepath.Join(dir, "c.mp4")] || !got[filepath.Join(dir, "a.mp4")] {
		t.Errorf("warmed %v, want c.mp4 and a.mp4", got)
	}
}

func TestOpenFilePrefetchesNeighborsOfTheOpenedFile(t *testing.T) {
	dir := mkFiles(t, "a.mp4", "b.mp4", "c.mp4")
	rec := newRecorder()
	p := &Player{log: slog.Default()}
	p.pf.warm = rec.warm

	p.openFile(filepath.Join(dir, "b.mp4"))

	got := map[string]bool{rec.next(t): true, rec.next(t): true}
	if !got[filepath.Join(dir, "c.mp4")] || !got[filepath.Join(dir, "a.mp4")] {
		t.Errorf("warmed %v, want c.mp4 and a.mp4", got)
	}
}

func TestSetPlaylistScansAndPrefetches(t *testing.T) {
	dir := mkFiles(t, "a.mp4", "b.mp4")
	rec := newRecorder()
	p := &Player{log: slog.Default()}
	p.pf.warm = rec.warm

	p.setPlaylist(filepath.Join(dir, "a.mp4"))

	if got := p.pl.current(); got != filepath.Join(dir, "a.mp4") {
		t.Errorf("current() = %q, want a.mp4", got)
	}
	if got := rec.next(t); got != filepath.Join(dir, "b.mp4") {
		t.Errorf("warmed %q, want b.mp4", got)
	}
}

func TestOpenFileURLDoesNotPrefetch(t *testing.T) {
	rec := newRecorder()
	p := &Player{log: slog.Default()}
	p.pf.warm = rec.warm

	p.openFile("http://example.com/video.mp4") // no playlist for URLs

	select {
	case path := <-rec.paths:
		t.Errorf("warmed %q for a URL source", path)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestAdvanceAfterRemovalPrefetchesNeighbors(t *testing.T) {
	p, dir := playerWithPlaylist(t, "a.mp4", "b.mp4", "c.mp4")
	rec := newRecorder()
	p.pf.warm = rec.warm
	gone := filepath.Join(dir, "a.mp4")
	if err := os.Remove(gone); err != nil {
		t.Fatal(err)
	}

	p.advanceAfterRemoval(gone) // now on b.mp4, whose only neighbour is c.mp4

	if got := rec.next(t); got != filepath.Join(dir, "c.mp4") {
		t.Errorf("warmed %q, want c.mp4", got)
	}
}

func TestPrefetcherStopCancelsInFlightWarmUp(t *testing.T) {
	started := make(chan context.Context, 1)
	pf := &prefetcher{warm: func(ctx context.Context, _ string) (int64, error) {
		started <- ctx
		<-ctx.Done()
		return 0, ctx.Err()
	}}
	pf.start([]string{"/v/slow.mp4"})

	var ctx context.Context
	select {
	case ctx = <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("warm-up never started")
	}

	pf.stop()
	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
		t.Error("stop did not cancel the in-flight warm-up")
	}
}

func TestPlayAdjacentWithoutPlaylistDoesNotPrefetch(t *testing.T) {
	rec := newRecorder()
	p := &Player{log: slog.Default()}
	p.pf.warm = rec.warm

	p.playAdjacent(1) // no playlist: must not panic or warm anything

	select {
	case path := <-rec.paths:
		t.Errorf("warmed %q with no playlist", path)
	case <-time.After(100 * time.Millisecond):
	}
}
