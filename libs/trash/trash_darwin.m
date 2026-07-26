// Built only on macOS: the _darwin filename suffix is the constraint.

#import <Foundation/Foundation.h>
#include <stdlib.h>
#include <string.h>

// goviTrashItem moves the item at path to the trash appropriate for its volume.
//
// -[NSFileManager trashItemAtURL:resultingItemURL:error:] (macOS 10.8+) is the
// documented API behind Finder's "Move to Trash": it resolves ~/.Trash vs the
// volume's .Trashes/<uid>, renames within the volume (never copies), resolves
// name collisions, and records the metadata Finder needs for "Put Back". On a
// volume with no trash support it fails rather than deleting the item.
//
// isDir must be 1 when path is a directory; it only affects how the URL is
// formed, but a wrong value makes NSURL normalise the trailing slash oddly.
// Returns 0 on success. On failure returns 1 and, when errOut is non-NULL,
// stores a malloc'd message there for the caller to free.
int goviTrashItem(const char *path, int isDir, char **errOut) {
	if (errOut != NULL) {
		*errOut = NULL;
	}
	if (path == NULL || *path == '\0') {
		if (errOut != NULL) {
			*errOut = strdup("no path given");
		}
		return 1;
	}

	@autoreleasepool {
		// fileURLWithFileSystemRepresentation: is the correct decoder for a POSIX
		// path byte string — it preserves the filesystem's Unicode normalisation,
		// which building an NSString via UTF8String would not.
		NSURL *url = [NSURL fileURLWithFileSystemRepresentation:path
		                                           isDirectory:(isDir != 0)
		                                         relativeToURL:nil];
		if (url == nil) {
			if (errOut != NULL) {
				*errOut = strdup("path is not valid on this filesystem");
			}
			return 1;
		}

		NSError *err = nil;
		BOOL ok = [[NSFileManager defaultManager] trashItemAtURL:url
		                                       resultingItemURL:NULL
		                                                  error:&err];
		if (ok) {
			return 0;
		}

		if (errOut != NULL) {
			NSString *desc = (err != nil && err.localizedDescription != nil)
			                     ? err.localizedDescription
			                     : @"could not move the item to the trash";
			const char *utf8 = desc.UTF8String;
			// strdup so the message outlives this autorelease pool; Go frees it.
			*errOut = strdup(utf8 != NULL ? utf8 : "could not move the item to the trash");
		}
		return 1;
	}
}
