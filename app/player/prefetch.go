package player

import (
	"context"
	"io"
	"os"
	"sync"
)

const (
	// prefetchHead is how much of a neighbouring file's start is pulled into
	// the OS page cache: enough for the container header, the index of a
	// faststart MP4 and the first keyframes.
	prefetchHead = 2 << 20
	// prefetchTail covers the trailing metadata formats that keep it at the
	// end of the file (Matroska cues, a non-faststart MP4 moov atom).
	prefetchTail = 256 << 10
	// prefetchChunk bounds one read, so cancellation is noticed promptly on
	// slow storage instead of only between the head and tail passes.
	prefetchChunk = 256 << 10
)

// warmFile reads the start and end of path into the OS page cache and discards
// the bytes, so the mpv demuxer finds them in memory when the file is opened.
// It returns how many bytes were read. Nothing is written: the file is opened
// read-only and left byte-for-byte untouched.
func warmFile(ctx context.Context, path string) (int64, error) {
	f, err := os.Open(path) //nolint:gosec // G304: read-only warm-up of a playlist entry
	if err != nil {
		return 0, err
	}
	defer f.Close() //nolint:errcheck // read-only handle, nothing to flush

	fi, err := f.Stat()
	if err != nil {
		return 0, err
	}
	size := fi.Size()

	head := min(size, int64(prefetchHead))
	n, err := readInto(ctx, io.NewSectionReader(f, 0, head))
	if err != nil {
		return n, err
	}
	// Skip the tail pass when the head already covered the whole file, so a
	// small file is not read twice.
	if size <= head {
		return n, nil
	}
	tailAt := max(head, size-int64(prefetchTail))
	tail, err := readInto(ctx, io.NewSectionReader(f, tailAt, size-tailAt))
	return n + tail, err
}

// readInto drains r into the void in prefetchChunk steps, stopping as soon as
// ctx is done. It reports the bytes read and ctx's error on cancellation.
func readInto(ctx context.Context, r io.Reader) (int64, error) {
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		n, err := io.CopyN(io.Discard, r, prefetchChunk)
		total += n
		switch {
		case err == io.EOF:
			return total, nil
		case err != nil:
			return total, err
		}
	}
}

// neighbors returns the entries around the current one — next first, since
// forward navigation is the common case — skipping positions off either end.
func (pl *playlist) neighbors() []string {
	if pl == nil {
		return nil
	}
	var out []string
	for _, i := range []int{pl.idx + 1, pl.idx - 1} {
		if i >= 0 && i < len(pl.entries) && i != pl.idx {
			out = append(out, pl.entries[i])
		}
	}
	return out
}

// prefetcher warms playlist neighbours in the background. Each start cancels
// the generation before it, so holding down next/previous does not pile up
// reads for files the user already skipped past. warm defaults to warmFile;
// tests inject their own.
type prefetcher struct {
	warm   func(context.Context, string) (int64, error)
	cancel context.CancelFunc
	mu     sync.Mutex
}

// start cancels any in-flight warm-up and warms paths concurrently. It returns
// immediately; the reads outlive it in their own goroutines.
func (pf *prefetcher) start(paths []string) {
	pf.mu.Lock()
	defer pf.mu.Unlock()

	if pf.cancel != nil {
		pf.cancel()
		pf.cancel = nil
	}
	if len(paths) == 0 {
		return
	}
	warm := pf.warm
	if warm == nil {
		warm = warmFile
	}

	ctx, cancel := context.WithCancel(context.Background())
	pf.cancel = cancel
	for _, path := range paths {
		go func(path string) {
			_, _ = warm(ctx, path) // best-effort: a cold cache is the only cost
		}(path)
	}
}

// stop cancels any in-flight warm-up (called as the player shuts down).
func (pf *prefetcher) stop() { pf.start(nil) }
