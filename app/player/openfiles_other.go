//go:build !darwin || !cgo

package player

// The Finder open-document event is a macOS concept: everywhere else the file
// arrives in argv (a shell invocation, or Exec=govi %f from
// zarf/govi.desktop), which app/cmd already passes to Run. These stubs keep the
// loop free of build tags.

// installOpenFilesHandler is a no-op off macOS. Its other job on macOS —
// finishing AppKit's launch sequence — has no analogue either: X11 and Wayland
// have no launch handshake for glfw.CreateWindow to block on.
func installOpenFilesHandler() error { return nil }

// takePendingOpen never has anything to report off macOS.
func takePendingOpen() string { return "" }
