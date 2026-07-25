//go:build !windows && !darwin

package trash

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// homeTrash points the home trash at a temp dir via XDG_DATA_HOME and returns
// its Trash path. t.Setenv restores the environment after the test.
func homeTrash(t *testing.T) string {
	t.Helper()
	dataHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	return filepath.Join(dataHome, "Trash")
}

// writeFile creates a file with known content and returns its path.
func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

// trashInfo reads the single .trashinfo file in the trash and returns its body.
func trashInfo(t *testing.T, trashDir string) string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(trashDir, "info"))
	if err != nil {
		t.Fatalf("read info dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 info file, got %d", len(entries))
	}
	body, err := os.ReadFile(filepath.Join(trashDir, "info", entries[0].Name()))
	if err != nil {
		t.Fatalf("read info file: %v", err)
	}
	return string(body)
}

func TestMoveRemovesOriginal(t *testing.T) {
	homeTrash(t)
	src := writeFile(t, t.TempDir(), "movie.mkv", "data")

	if err := Move(src); err != nil {
		t.Fatalf("Move: %v", err)
	}

	if _, err := os.Lstat(src); !os.IsNotExist(err) {
		t.Errorf("original still present, Lstat err = %v, want IsNotExist", err)
	}
}

func TestMovePlacesFileInTrashFilesDir(t *testing.T) {
	trashDir := homeTrash(t)
	src := writeFile(t, t.TempDir(), "movie.mkv", "payload")

	if err := Move(src); err != nil {
		t.Fatalf("Move: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(trashDir, "files", "movie.mkv"))
	if err != nil {
		t.Fatalf("read trashed file: %v", err)
	}
	if string(got) != "payload" {
		t.Errorf("trashed content = %q, want %q", got, "payload")
	}
}

func TestMoveWritesTrashInfoWithOriginalPath(t *testing.T) {
	trashDir := homeTrash(t)
	src := writeFile(t, t.TempDir(), "my movie.mkv", "data")

	if err := Move(src); err != nil {
		t.Fatalf("Move: %v", err)
	}

	info := trashInfo(t, trashDir)
	if !strings.HasPrefix(info, "[Trash Info]\n") {
		t.Errorf("info body does not start with [Trash Info] header:\n%s", info)
	}
	wantPath := "Path=" + escapePath(src) + "\n"
	if !strings.Contains(info, wantPath) {
		t.Errorf("info body missing %q:\n%s", wantPath, info)
	}
	if !strings.Contains(info, "DeletionDate=") {
		t.Errorf("info body missing DeletionDate:\n%s", info)
	}
}

func TestMoveKeepsNameOnCollision(t *testing.T) {
	trashDir := homeTrash(t)
	dir := t.TempDir()

	first := writeFile(t, dir, "movie.mkv", "first")
	if err := Move(first); err != nil {
		t.Fatalf("Move first: %v", err)
	}
	second := writeFile(t, dir, "movie.mkv", "second")
	if err := Move(second); err != nil {
		t.Fatalf("Move second: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(trashDir, "files", "movie.2.mkv"))
	if err != nil {
		t.Fatalf("read second trashed file: %v", err)
	}
	if string(got) != "second" {
		t.Errorf("movie.2.mkv content = %q, want %q", got, "second")
	}
	// The first file must be untouched by the collision handling.
	orig, err := os.ReadFile(filepath.Join(trashDir, "files", "movie.mkv"))
	if err != nil {
		t.Fatalf("read first trashed file: %v", err)
	}
	if string(orig) != "first" {
		t.Errorf("movie.mkv content = %q, want %q", orig, "first")
	}
}

func TestMoveCreatesTrashDirsWithPrivatePermissions(t *testing.T) {
	trashDir := homeTrash(t)
	src := writeFile(t, t.TempDir(), "movie.mkv", "data")

	if err := Move(src); err != nil {
		t.Fatalf("Move: %v", err)
	}

	// The spec recommends 0700: trash contents must not be world-readable.
	for _, dir := range []string{trashDir, filepath.Join(trashDir, "files"), filepath.Join(trashDir, "info")} {
		st, err := os.Stat(dir)
		if err != nil {
			t.Fatalf("stat %s: %v", dir, err)
		}
		if perm := st.Mode().Perm(); perm != 0o700 {
			t.Errorf("%s perm = %#o, want 0700", dir, perm)
		}
	}
}

func TestMoveDirectoryRecordsDirectorySizes(t *testing.T) {
	trashDir := homeTrash(t)
	dir := filepath.Join(t.TempDir(), "album")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeFile(t, dir, "a.mkv", "12345")

	if err := Move(dir); err != nil {
		t.Fatalf("Move: %v", err)
	}

	body, err := os.ReadFile(filepath.Join(trashDir, "directorysizes"))
	if err != nil {
		t.Fatalf("read directorysizes: %v", err)
	}
	// Format: "<size> <mtime> <escaped name>"; the name is the trash entry name.
	if !strings.HasSuffix(strings.TrimSpace(string(body)), " album") {
		t.Errorf("directorysizes entry = %q, want it to end with the escaped name", body)
	}
}

func TestMoveMissingFileReturnsError(t *testing.T) {
	homeTrash(t)

	err := Move(filepath.Join(t.TempDir(), "gone.mkv"))
	if err == nil {
		t.Fatal("Move on a missing file returned nil, want error")
	}
}

func TestMoveLeavesNoInfoFileWhenMoveFails(t *testing.T) {
	trashDir := homeTrash(t)

	// A directory the caller cannot unlink from: trashing must fail without
	// leaving an orphaned .trashinfo behind.
	parent := t.TempDir()
	src := writeFile(t, parent, "movie.mkv", "data")
	if err := os.Chmod(parent, 0o500); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(parent, 0o700) })

	if err := Move(src); err == nil {
		t.Fatal("Move from a read-only directory returned nil, want error")
	}

	entries, err := os.ReadDir(filepath.Join(trashDir, "info"))
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("read info dir: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("orphaned info files left behind: %d", len(entries))
	}
}
