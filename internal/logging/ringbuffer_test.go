package logging

import (
	"io"
	"log/slog"
	"strings"
	"testing"
)

func TestRingBufferSnapshotEmptyByDefault(t *testing.T) {
	rb := NewRingBuffer(10)
	if got := rb.Snapshot(); got != "" {
		t.Fatalf("expected empty snapshot, got %q", got)
	}
}

func TestRingBufferKeepsLinesInOrder(t *testing.T) {
	rb := NewRingBuffer(10)
	for _, s := range []string{"one\n", "two\n", "three\n"} {
		if _, err := rb.Write([]byte(s)); err != nil {
			t.Fatal(err)
		}
	}
	if got, want := rb.Snapshot(), "one\ntwo\nthree\n"; got != want {
		t.Fatalf("snapshot mismatch:\ngot:  %q\nwant: %q", got, want)
	}
}

func TestRingBufferDropsOldestBeyondCap(t *testing.T) {
	rb := NewRingBuffer(2)
	for _, s := range []string{"one\n", "two\n", "three\n"} {
		if _, err := rb.Write([]byte(s)); err != nil {
			t.Fatal(err)
		}
	}
	if got, want := rb.Snapshot(), "two\nthree\n"; got != want {
		t.Fatalf("expected only the last %d lines, got %q", 2, got)
	}
}

func TestRingBufferSplitsMultiLineWrite(t *testing.T) {
	rb := NewRingBuffer(10)
	if _, err := rb.Write([]byte("a\nb\n")); err != nil {
		t.Fatal(err)
	}
	if got, want := rb.Snapshot(), "a\nb\n"; got != want {
		t.Fatalf("expected two split lines, got %q", got)
	}
}

// A partial line (no trailing newline yet) is held until the newline arrives,
// so a single log record split across writes is still stored as one line.
func TestRingBufferHoldsPartialLineUntilNewline(t *testing.T) {
	rb := NewRingBuffer(10)
	rb.Write([]byte("hel"))
	if got := rb.Snapshot(); got != "" {
		t.Fatalf("partial line should not appear yet, got %q", got)
	}
	rb.Write([]byte("lo\n"))
	if got, want := rb.Snapshot(), "hello\n"; got != want {
		t.Fatalf("expected reassembled line, got %q", got)
	}
}

// The ring buffer is meant to sit behind the real slog handler; a line logged
// through logging.New must land in the buffer verbatim in the handler's format.
func TestRingBufferCapturesLoggerOutput(t *testing.T) {
	rb := NewRingBuffer(10)
	log := New(io.MultiWriter(io.Discard, rb), slog.LevelInfo)
	log.Info("hello world", "answer", 42)
	snap := rb.Snapshot()
	if !strings.Contains(snap, "hello world") || !strings.Contains(snap, "answer=42") {
		t.Fatalf("expected logged record in snapshot, got %q", snap)
	}
}
