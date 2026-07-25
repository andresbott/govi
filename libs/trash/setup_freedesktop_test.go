//go:build !windows && !darwin

package trash

import "testing"

// setupTrash redirects the home trash into a temp dir so the tests never touch
// the developer's real trash. On Windows and macOS the OS owns the destination,
// so the equivalent there is a no-op.
func setupTrash(t *testing.T) {
	t.Helper()
	homeTrash(t)
}
