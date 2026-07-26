//go:build windows

package trash

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

// SHFileOperationW constants (shellapi.h). Declared here rather than pulled from
// a dependency: these four values are frozen ABI.
const (
	foDelete = 0x0003 // wFunc: delete the files in pFrom

	fofSilent          = 0x0004 // no progress dialog
	fofNoConfirmation  = 0x0010 // answer "yes to all" to any prompt
	fofAllowUndo       = 0x0040 // *** send to the Recycle Bin instead of deleting
	fofNoErrorUI       = 0x0400 // no error dialog; report via the return value
	fofNoConfirmMkDir  = 0x0200 // never prompt to create a directory
	fofWantNukeWarning = 0x4000 // warn when an item cannot be recycled
)

var (
	shell32              = windows.NewLazySystemDLL("shell32.dll")
	procSHFileOperationW = shell32.NewProc("SHFileOperationW")
)

// shFileOpStructW mirrors SHFILEOPSTRUCTW. The bool fields are declared as
// int32/uintptr to keep the struct's alignment identical to the C layout.
type shFileOpStructW struct {
	hwnd                  uintptr
	wFunc                 uint32
	pFrom                 *uint16
	pTo                   *uint16
	fFlags                uint16
	fAnyOperationsAborted int32
	hNameMappings         uintptr
	lpszProgressTitle     *uint16
}

// move sends path to the Recycle Bin via the shell, which picks the correct
// per-volume $Recycle.Bin and writes the metadata Explorer needs to restore.
func move(path string) error {
	// The shell requires a fully-qualified path; a relative one is resolved
	// against the process's current directory, which is not what the caller
	// means. Long paths are handled by the shell itself, so no \\?\ prefix.
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	// SHFileOperationW reports "success" for a missing file, so check first;
	// callers rely on a nil error meaning the item was actually trashed.
	if _, err := os.Lstat(abs); err != nil {
		return err
	}

	from, err := doubleNulTerminated(abs)
	if err != nil {
		return err
	}
	// An empty, NUL-terminated title: passing nil here crashes some shell
	// versions when a dialog would have been shown.
	title := []uint16{0}

	op := shFileOpStructW{
		wFunc: foDelete,
		pFrom: &from[0],
		fFlags: fofAllowUndo | fofNoConfirmation | fofNoErrorUI | fofSilent |
			fofNoConfirmMkDir | fofWantNukeWarning,
		lpszProgressTitle: &title[0],
	}

	// The return value, not GetLastError, carries the shell's status: it is 0 on
	// success and an undocumented shell error code otherwise (DE_* values that
	// deliberately overlap winerror.h, hence the bare code in the message).
	ret, _, _ := procSHFileOperationW.Call(uintptr(unsafe.Pointer(&op)))
	// The struct holds raw pointers the compiler cannot trace, so keep both
	// buffers alive until the call has returned.
	runtime.KeepAlive(from)
	runtime.KeepAlive(title)
	if ret != 0 {
		return fmt.Errorf("trash: SHFileOperationW failed for %s: shell error 0x%x", abs, ret)
	}
	if op.fAnyOperationsAborted != 0 {
		return fmt.Errorf("trash: moving %s to the Recycle Bin was aborted", abs)
	}
	// FOF_ALLOWUNDO is best-effort: the shell silently falls back to a permanent
	// delete when the volume has no Recycle Bin (network shares, small removable
	// media). Either way the file is gone from its original location, which is
	// what Move promises, so this is not an error.
	return nil
}

// doubleNulTerminated encodes path as the UTF-16 pFrom list SHFileOperationW
// expects: the path, its terminating NUL, and a second NUL closing the list.
func doubleNulTerminated(path string) ([]uint16, error) {
	// windows.UTF16FromString rejects embedded NULs, which would otherwise
	// truncate the list and target a different file than the caller named.
	u, err := windows.UTF16FromString(path)
	if err != nil {
		return nil, fmt.Errorf("trash: invalid path %q: %w", path, err)
	}
	if len(u) < 2 {
		return nil, errors.New("trash: empty path")
	}
	return append(u, 0), nil
}
