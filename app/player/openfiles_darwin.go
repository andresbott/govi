//go:build darwin && cgo

package player

/*
#cgo LDFLAGS: -framework Cocoa

// Implemented in openfiles_darwin.m. Returns 1 when the handler was installed,
// 0 when there is no NSApplication delegate to attach it to.
int goviInstallOpenFilesHandler(void);
*/
import "C"

import (
	"errors"

	"github.com/go-gl/glfw/v3.3/glfw"
)

// pendingOpen carries a path from the Cocoa main thread into the player loop.
//
// A channel rather than a direct call, because the Apple Event arrives on the
// main thread inside GLFW's event pump: calling into mpv from there would issue
// a loadfile from a callback, which is exactly what the loop's structure exists
// to avoid (see the note on handleEndOfFile in loop). Buffered and depth 1, and
// writes are non-blocking: if two events land between iterations the newer wins,
// which matches what a user double-clicking twice expects.
var pendingOpen = make(chan string, 1)

// installOpenFilesHandler makes govi respond to Finder's open-document event.
//
// Called once, after GLFW is initialised — it grafts onto the delegate GLFW
// installs, so ordering matters. Only relevant for the bundled .app: a bare
// binary invoked from a shell gets its path in argv and never sees an Apple
// Event.
func installOpenFilesHandler() error {
	if C.goviInstallOpenFilesHandler() == 0 {
		return errors.New("no NSApplication delegate: glfw.Init must run first")
	}
	return nil
}

//export goviOpenFileFromFinder
func goviOpenFileFromFinder(path *C.char) {
	// C.GoString copies, so the Objective-C side's buffer does not have to
	// outlive this call.
	p := C.GoString(path)
	if p == "" {
		return
	}
	// Drop the oldest pending path rather than block: this runs on the Cocoa
	// main thread, and blocking it would freeze the UI.
	select {
	case pendingOpen <- p:
	default:
		select {
		case <-pendingOpen:
		default:
		}
		select {
		case pendingOpen <- p:
		default:
		}
	}
	// The loop may be asleep in WaitEventsTimeout; wake it so the file opens now
	// rather than up to idleFrame later. PostEmptyEvent is thread-safe.
	glfw.PostEmptyEvent()
}

// takePendingOpen returns a path delivered by Finder since the last call, or ""
// when there is none. Non-blocking: the loop calls it every iteration.
func takePendingOpen() string {
	select {
	case p := <-pendingOpen:
		return p
	default:
		return ""
	}
}
