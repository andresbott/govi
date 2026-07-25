// Package trash moves files and directories to the desktop trash, using the
// native mechanism of each OS so the result is what the user's file manager
// shows and can restore:
//
//   - Windows: SHFileOperationW with FOF_ALLOWUNDO — the Recycle Bin, including
//     the correct per-volume $Recycle.Bin.
//   - macOS: -[NSFileManager trashItemAtURL:] — ~/.Trash or the volume's
//     .Trashes, with the metadata Finder's "Put Back" needs.
//   - Linux/BSD: the FreeDesktop.org Trash specification, including per-volume
//     .Trash-$uid directories on other filesystems (what "gio trash" does).
//
// Trashing never copies file data. If a file cannot be moved into a trash
// directory on its own filesystem, Move fails rather than silently pulling
// gigabytes across a network mount.
package trash

import "errors"

// ErrEmptyPath is returned by Move for an empty path. It is a distinct error
// because the alternative is catastrophic: filepath.Abs("") resolves to the
// process's working directory, so an unguarded empty path would trash the
// caller's whole current directory.
var ErrEmptyPath = errors.New("trash: empty path")

// Move sends path to the OS trash. The returned error is non-nil if the item
// still exists at path afterwards, so callers can rely on a nil error meaning
// the original is gone. A missing path yields an error satisfying
// os.IsNotExist; an empty one yields ErrEmptyPath.
func Move(path string) error {
	if path == "" {
		return ErrEmptyPath
	}
	return move(path)
}
