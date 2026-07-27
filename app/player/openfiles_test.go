//go:build !darwin || !cgo

// Tagged to match openfiles_other.go: on a cgo macOS build these symbols reach
// Cocoa, where installOpenFilesHandler correctly fails without a running
// NSApplication, so asserting a nil error there would be wrong.

package player

import "testing"

// The stub path (everything but a cgo macOS build) must be inert rather than
// merely compile: the loop calls takePendingOpen every iteration, so anything
// but an immediate empty return would cost a frame, and installOpenFilesHandler
// returning an error would log a spurious warning on every Linux start.
func TestOpenFilesStubsAreInert(t *testing.T) {
	if err := installOpenFilesHandler(); err != nil {
		t.Errorf("installOpenFilesHandler() = %v, want nil off macOS", err)
	}
	for i := 0; i < 3; i++ {
		if got := takePendingOpen(); got != "" {
			t.Errorf("takePendingOpen() = %q, want empty", got)
		}
	}
}
