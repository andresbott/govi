//go:build !windows && !darwin

package trash

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// otherFilesystemDir returns a writable temp dir on a filesystem other than the
// one holding the home data dir, skipping the test when none is available. This
// is the local stand-in for a Samba/NFS/USB mount: the interesting property is
// only that os.Rename into the home trash would fail with EXDEV.
func otherFilesystemDir(t *testing.T) string {
	t.Helper()

	home, err := dataHome()
	if err != nil {
		t.Fatalf("dataHome: %v", err)
	}
	homeDev, err := deviceOfNearest(home)
	if err != nil {
		t.Fatalf("device of %s: %v", home, err)
	}

	for _, base := range []string{"/dev/shm", "/tmp", os.Getenv("XDG_RUNTIME_DIR")} {
		if base == "" {
			continue
		}
		dev, err := deviceOf(base)
		if err != nil || dev == homeDev {
			continue
		}
		dir, err := os.MkdirTemp(base, "govi-trash-")
		if err != nil {
			continue
		}
		t.Cleanup(func() { _ = os.RemoveAll(dir) })
		return dir
	}
	t.Skip("no second filesystem available to test per-volume trash")
	return ""
}

func TestMoveOnOtherFilesystemUsesVolumeTrash(t *testing.T) {
	homeTrashDir := homeTrash(t)
	dir := otherFilesystemDir(t)
	src := writeFile(t, dir, "movie.mkv", "payload")

	if err := Move(src); err != nil {
		t.Fatalf("Move: %v", err)
	}

	// The file must land in a trash on its own filesystem, not in the home one:
	// copying it home is exactly the behaviour this package exists to avoid.
	volumeTrashDir := filepath.Join(topDirOf(t, dir), ".Trash-"+strconv.Itoa(os.Getuid()))
	t.Cleanup(func() { _ = os.RemoveAll(volumeTrashDir) })

	got, err := os.ReadFile(filepath.Join(volumeTrashDir, "files", "movie.mkv"))
	if err != nil {
		t.Fatalf("read file from volume trash: %v", err)
	}
	if string(got) != "payload" {
		t.Errorf("trashed content = %q, want %q", got, "payload")
	}
	if _, err := os.Stat(filepath.Join(homeTrashDir, "files", "movie.mkv")); err == nil {
		t.Error("file was placed in the home trash; it must stay on its own filesystem")
	}
	if _, err := os.Lstat(src); !os.IsNotExist(err) {
		t.Errorf("original still present, Lstat err = %v, want IsNotExist", err)
	}
}

func TestMoveOnOtherFilesystemRecordsRelativePath(t *testing.T) {
	homeTrash(t)
	dir := otherFilesystemDir(t)
	src := writeFile(t, dir, "movie.mkv", "data")

	if err := Move(src); err != nil {
		t.Fatalf("Move: %v", err)
	}
	top := topDirOf(t, dir)
	volumeTrashDir := filepath.Join(top, ".Trash-"+strconv.Itoa(os.Getuid()))
	t.Cleanup(func() { _ = os.RemoveAll(volumeTrashDir) })

	info := trashInfo(t, volumeTrashDir)
	// The spec requires a path relative to the volume's top directory, so the
	// entry stays valid if the volume is mounted elsewhere later.
	rel, err := filepath.Rel(top, src)
	if err != nil {
		t.Fatalf("Rel: %v", err)
	}
	wantPath := "Path=" + escapePath(rel) + "\n"
	if !strings.Contains(info, wantPath) {
		t.Errorf("info body missing %q:\n%s", wantPath, info)
	}
	if strings.Contains(info, "Path="+escapePath(src)) {
		t.Errorf("info body recorded an absolute path:\n%s", info)
	}
}

// topDirOf resolves the mount point of dir via the package's own helper.
func topDirOf(t *testing.T, dir string) string {
	t.Helper()
	top, err := topDir(dir)
	if err != nil {
		t.Fatalf("topDir(%s): %v", dir, err)
	}
	return top
}
