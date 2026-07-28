// Built only on macOS: the _darwin filename suffix is the constraint.

#import <Cocoa/Cocoa.h>
#import <objc/runtime.h>

// Implemented in Go (openfiles_darwin.go, //export goviOpenFileFromFinder).
void goviOpenFileFromFinder(char *path);

// When Finder opens a document with a bundled app, it does not pass the path in
// argv: it sends an `odoc` Apple Event, which AppKit turns into a call to
// -application:openFiles: on the NSApplication delegate. Nothing handles that
// method by default, and an unhandled open event is dropped silently — govi
// would come up on its idle screen with the file the user clicked discarded.
//
// GLFW installs its own delegate in _glfwPlatformInit (cocoa_init.m) and relies
// on it for applicationShouldTerminate, among others, so this cannot replace the
// delegate object. Instead the method is grafted onto whatever class the
// delegate already is, using the objc runtime. That keeps GLFW's handlers intact
// and means the graft survives any future GLFW version that adds more of them.

// goviOpenFilesImp is the IMP installed as -application:openFiles:.
//
// Only the first path is used: govi plays one file at a time and derives its
// playlist from that file's folder, so a multi-file selection is not a playlist
// — the folder scan already covers the siblings.
static void goviOpenFilesImp(id self, SEL _cmd, NSApplication *app, NSArray<NSString *> *paths) {
	(void)self;
	(void)_cmd;

	if (paths.count > 0) {
		NSString *first = paths[0];
		const char *utf8 = first.fileSystemRepresentation;
		if (utf8 != NULL) {
			// The Go side copies the string immediately; nothing here outlives
			// the call. Cast away const because cgo's exported signature takes
			// a mutable char*.
			goviOpenFileFromFinder((char *)utf8);
		}
	}

	// Tell AppKit the open succeeded. Without this the Dock shows the launch as
	// failed and, on a cold start, macOS may report that govi "cannot open" the
	// file even though it is playing.
	[app replyToOpenOrPrint:NSApplicationDelegateReplySuccess];
}

// goviFinishLaunching completes AppKit's launch sequence before any window is
// created.
//
// GLFW does not do this itself: _glfwPlatformCreateWindow calls [NSApp run] and
// relies on -applicationDidFinishLaunching: to call [NSApp stop:] and unwind it
// (cocoa_window.m, cocoa_init.m). That notification is only delivered once the
// process is a *launched, active* application, which holds for a bare binary but
// not for govi since v0.1.5, where the executable lives inside govi.app:
//
//   - run from a shell (the Homebrew symlink points into Contents/MacOS), the app
//     never activates, the notification never arrives, and glfw.CreateWindow
//     blocks forever — no window, and not even a Dock icon to click, because the
//     regular activation policy is also set inside that same callback;
//   - launched through Finder or `open`, the notification waits for the user to
//     activate the app, so the window only appeared after a click on the Dock
//     icon.
//
// -finishLaunching posts the two launch notifications synchronously, which sets
// GLFW's finishedLaunching flag (its delegate observes them) and applies the
// activation policy. CreateWindow then skips [NSApp run] altogether. Calling it
// twice is what must be avoided, hence the guard: AppKit's own -run would call
// it again, and a second launch sequence re-posts open-document events.
//
// Called with the delegate already in place, so the open-files handler below is
// grafted on first and a cold start's odoc event still lands in the queue.
static void goviFinishLaunching(void) {
	static BOOL done = NO;
	if (done) {
		return;
	}
	done = YES;
	[NSApp finishLaunching];
	// finishLaunching alone leaves an unbundled or shell-launched process without
	// focus: it is ordered in but not frontmost, so the window would open behind
	// whatever terminal started it. Activating matches what a double-click gives.
	[NSApp activateIgnoringOtherApps:YES];
}

// goviInstallOpenFilesHandler grafts the handler onto the live delegate's class
// and finishes AppKit's launch sequence.
//
// Returns 1 when the handler is in place, 0 when there is no delegate to attach
// it to (which means GLFW has not initialised yet — the Go side treats that as a
// programming error rather than retrying).
int goviInstallOpenFilesHandler(void) {
	@autoreleasepool {
		if (NSApp == nil) {
			return 0;
		}
		id delegate = [NSApp delegate];
		if (delegate == nil) {
			return 0;
		}

		Class cls = object_getClass(delegate);
		SEL sel = @selector(application:openFiles:);

		// "v@:@@" = void return, self, _cmd, then two object arguments.
		const char *types = "v@:@@";

		// class_addMethod fails when the class already implements the selector,
		// in which case the existing implementation is replaced instead. Either
		// way the method ends up installed, which is all the caller needs.
		if (!class_addMethod(cls, sel, (IMP)goviOpenFilesImp, types)) {
			class_replaceMethod(cls, sel, (IMP)goviOpenFilesImp, types);
		}

		// Only after the graft: -finishLaunching is what delivers a cold start's
		// open-document event, and the handler has to be installed to catch it.
		goviFinishLaunching();
		return 1;
	}
}

// goviWakeEventLoop posts a no-op event so a thread parked in
// nextEventMatchingMask returns now instead of waiting out its timeout.
//
// Deliberately not glfw.PostEmptyEvent: that one begins with
// `if (!finishedLaunching) [NSApp run];` (cocoa_window.m), so calling it from
// -application:openFiles: during a cold start would start a *nested* run loop
// from inside a delegate callback, with the outer one still on the stack. This
// does only the postEvent half.
void goviWakeEventLoop(void) {
	@autoreleasepool {
		NSEvent *event = [NSEvent otherEventWithType:NSEventTypeApplicationDefined
		                                   location:NSMakePoint(0, 0)
		                              modifierFlags:0
		                                  timestamp:0
		                               windowNumber:0
		                                    context:nil
		                                    subtype:0
		                                      data1:0
		                                      data2:0];
		[NSApp postEvent:event atStart:YES];
	}
}
