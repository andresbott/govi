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

// goviInstallOpenFilesHandler grafts the handler onto the live delegate's class.
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
		return 1;
	}
}
