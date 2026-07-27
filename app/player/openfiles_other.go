//go:build !darwin || !cgo

package player

// The Finder open-document event is a macOS concept: everywhere else the file
// arrives in argv (a shell invocation, or Exec=govi %f from
// zarf/govi.desktop), which app/cmd already passes to Run. These stubs keep the
// loop free of build tags.

// installOpenFilesHandler is a no-op off macOS.
func installOpenFilesHandler() error { return nil }

// takePendingOpen never has anything to report off macOS.
func takePendingOpen() string { return "" }
