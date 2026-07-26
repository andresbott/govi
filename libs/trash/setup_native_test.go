//go:build windows || darwin

package trash

import "testing"

// setupTrash is a no-op on Windows and macOS: the destination is chosen by the
// shell / NSFileManager, so there is nothing to redirect. These tests therefore
// leave real items in the user's Recycle Bin or Trash when run locally, which is
// the price of exercising the genuine OS behaviour.
func setupTrash(t *testing.T) {
	t.Helper()
}
