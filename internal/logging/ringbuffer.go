package logging

import (
	"bytes"
	"strings"
	"sync"
)

// defaultRingLines is the number of log lines a RingBuffer keeps when it is
// created with a non-positive capacity. Enough to hold the tail leading up to a
// crash without pinning meaningful memory.
const defaultRingLines = 500

// RingBuffer is a fixed-capacity, line-oriented, concurrency-safe io.Writer that
// keeps only the most recent lines written to it. It is meant to sit behind the
// slog handler (via io.MultiWriter) so a crash report can include the tail of
// the log without writing anything to disk until it is needed.
type RingBuffer struct {
	mu    sync.Mutex
	buf   []string // fixed-size ring; index by count%max
	max   int
	count int    // total complete lines ever written
	carry []byte // bytes of a line not yet terminated by '\n'
}

// NewRingBuffer returns a RingBuffer holding at most max lines. A non-positive
// max falls back to defaultRingLines.
func NewRingBuffer(max int) *RingBuffer {
	if max <= 0 {
		max = defaultRingLines
	}
	return &RingBuffer{buf: make([]string, max), max: max}
}

// Write splits p into lines on '\n' and records the complete ones, holding any
// trailing partial line until its newline arrives. It never returns an error:
// slog's handler must not fail because a diagnostic buffer is full.
func (r *RingBuffer) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.carry = append(r.carry, p...)
	for {
		i := bytes.IndexByte(r.carry, '\n')
		if i < 0 {
			break
		}
		r.append(string(r.carry[:i]))
		r.carry = r.carry[i+1:]
	}
	// Release the backing array once the carry is drained, so a steady stream of
	// full lines does not keep it growing.
	if len(r.carry) == 0 {
		r.carry = nil
	}
	return len(p), nil
}

// append stores one complete line, overwriting the oldest once the ring is full.
func (r *RingBuffer) append(line string) {
	r.buf[r.count%r.max] = line
	r.count++
}

// Snapshot returns the retained lines, oldest first, each terminated by '\n'.
// It returns "" when nothing complete has been written yet.
func (r *RingBuffer) Snapshot() string {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.count == 0 {
		return ""
	}
	start, n := 0, r.count
	if r.count > r.max {
		start, n = r.count-r.max, r.max
	}
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteString(r.buf[(start+i)%r.max])
		b.WriteByte('\n')
	}
	return b.String()
}
