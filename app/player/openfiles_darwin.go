//go:build darwin && cgo

package player

/*
#cgo LDFLAGS: -framework Cocoa

// Implemented in openfiles_darwin.m. Returns 1 when the handler was installed,
// 0 when there is no NSApplication delegate to attach it to.
int goviInstallOpenFilesHandler(void);

// Posts a no-op AppKit event to wake a parked event wait. See the comment on
// its definition for why glfw.PostEmptyEvent cannot be used here.
void goviWakeEventLoop(void);
*/
import "C"

import (
	"errors"
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

// installOpenFilesHandler makes govi respond to Finder's open-document event and
// finishes AppKit's launch sequence.
//
// Called once, after GLFW is initialised — it grafts onto the delegate GLFW
// installs, so ordering matters. Two jobs rather than one because they share
// that single ordering constraint: the graft has to be in place before the
// launch notifications are posted, since a cold start's open-document event
// rides along with them. See openfiles_darwin.m for why govi drives the launch
// itself instead of letting glfw.CreateWindow wait for it — without it a
// bundled govi never opens a window from a shell at all.
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
	// rather than up to idleFrame later.
	C.goviWakeEventLoop()
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
