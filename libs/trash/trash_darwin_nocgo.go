//go:build darwin && !cgo

package trash

import "errors"

// move fails on macOS without cgo. The only correct way to trash an item is
// -[NSFileManager trashItemAtURL:] (see trash_darwin.go): a hand-rolled move
// into ~/.Trash cannot write the private metadata Finder's "Put Back" needs, so
// files would land in the trash unrestorable. Failing loudly beats that.
//
// govi always builds with cgo (go-gl/glfw requires it), so this file exists only
// to turn a confusing link error into a clear message.
func move(string) error {
	return errors.New("trash: macOS support requires a cgo build")
}
