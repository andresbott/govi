//go:build darwin && cgo

package trash

/*
#cgo LDFLAGS: -framework Foundation

#include <stdlib.h>

// Implemented in trash_darwin.m. Returns 0 on success; on failure returns 1 and
// stores a malloc'd, caller-freed description in *errOut.
int goviTrashItem(const char *path, int isDir, char **errOut);
*/
import "C"

import (
	"errors"
	"os"
	"path/filepath"
	"unsafe"
)

// move sends path to the macOS trash using -[NSFileManager trashItemAtURL:].
//
// The native API is used rather than a hand-rolled move because it is the only
// way to get the two things users expect: the correct destination for the item's
// volume (~/.Trash for the boot volume, /Volumes/<vol>/.Trashes/<uid> for a
// mounted one, and a per-user directory on network shares), and the private
// metadata that makes Finder's "Put Back" work. Neither is reproducible from Go.
func move(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	// trashItemAtURL: reports a generic error for a missing file; check first so
	// callers get the usual *PathError and can test with os.IsNotExist.
	st, err := os.Lstat(abs)
	if err != nil {
		return err
	}

	cPath := C.CString(abs)
	defer C.free(unsafe.Pointer(cPath))

	isDir := C.int(0)
	if st.IsDir() {
		isDir = 1
	}

	var cErr *C.char
	if rc := C.goviTrashItem(cPath, isDir, &cErr); rc != 0 {
		if cErr != nil {
			defer C.free(unsafe.Pointer(cErr))
			return errors.New("trash: " + C.GoString(cErr))
		}
		return errors.New("trash: could not move " + abs + " to the trash")
	}
	return nil
}
