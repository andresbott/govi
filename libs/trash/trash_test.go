package trash

import (
	"os"
	"path/filepath"
	"testing"
)

// These tests exercise the guarantees Move makes on every platform. The
// FreeDesktop-specific placement rules are covered in trash_freedesktop_test.go;
// here only behaviour that must hold on Windows and macOS too is asserted, so
// the native implementations are not completely untested when CI runs them.

func TestMoveMissingPathReturnsNotExist(t *testing.T) {
	setupTrash(t)

	err := Move(filepath.Join(t.TempDir(), "does-not-exist.mkv"))
	if err == nil {
		t.Fatal("Move on a missing path returned nil, want error")
	}
	// Callers distinguish "already gone" from a real failure, so the error must
	// stay inspectable rather than being wrapped into an opaque string.
	if !os.IsNotExist(err) {
		t.Errorf("os.IsNotExist(err) = false for %v, want true", err)
	}
}

func TestMoveEmptyPathReturnsError(t *testing.T) {
	setupTrash(t)

	// An empty path must be rejected outright. filepath.Abs("") resolves to the
	// *working directory*, so treating "" as a path would trash the caller's
	// whole cwd — the worst possible failure mode for this package.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "important.mkv"), []byte("x"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	t.Chdir(dir)

	if err := Move(""); err == nil {
		t.Fatal("Move(\"\") returned nil, want error")
	}
	if _, err := os.Lstat(dir); err != nil {
		t.Fatalf("Move(\"\") trashed the working directory: %v", err)
	}
}

func TestMoveRemovesDirectory(t *testing.T) {
	setupTrash(t)

	dir := filepath.Join(t.TempDir(), "album")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.mkv"), []byte("x"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := Move(dir); err != nil {
		t.Fatalf("Move: %v", err)
	}
	if _, err := os.Lstat(dir); !os.IsNotExist(err) {
		t.Errorf("directory still present, Lstat err = %v, want IsNotExist", err)
	}
}

func TestMoveRelativePathResolvesAgainstWorkingDir(t *testing.T) {
	setupTrash(t)

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "movie.mkv"), []byte("x"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	t.Chdir(dir)

	if err := Move("movie.mkv"); err != nil {
		t.Fatalf("Move: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(dir, "movie.mkv")); !os.IsNotExist(err) {
		t.Errorf("original still present, Lstat err = %v, want IsNotExist", err)
	}
}

func TestMoveTrashesBrokenSymlink(t *testing.T) {
	setupTrash(t)

	dir := t.TempDir()
	link := filepath.Join(dir, "dangling.mkv")
	if err := os.Symlink(filepath.Join(dir, "no-such-target.mkv"), link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	// A dangling symlink is a real directory entry the user can see and wants
	// gone, so Move must inspect the link itself and not its missing target.
	if err := Move(link); err != nil {
		t.Fatalf("Move on a broken symlink: %v", err)
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Errorf("broken symlink still present, Lstat err = %v, want IsNotExist", err)
	}
}

func TestMoveDoesNotFollowSymlink(t *testing.T) {
	setupTrash(t)

	dir := t.TempDir()
	target := filepath.Join(dir, "target.mkv")
	if err := os.WriteFile(target, []byte("keep me"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	link := filepath.Join(dir, "link.mkv")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	if err := Move(link); err != nil {
		t.Fatalf("Move: %v", err)
	}

	// Trashing a symlink must trash the link, never the file it points at.
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Errorf("symlink still present, Lstat err = %v, want IsNotExist", err)
	}
	if _, err := os.Lstat(target); err != nil {
		t.Errorf("Move followed the symlink and removed its target: %v", err)
	}
}
