//go:build !windows && !darwin

package trash

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// The FreeDesktop trash spec mandates 0700 on trash directories: trashed files
// keep their original contents and must not become world-readable.
const trashDirPerm fs.FileMode = 0o700

// maxCollisionTries bounds the search for a free name in the trash. Each try
// costs one O_EXCL create; the limit only matters for pathological cases where
// thousands of same-named files were already trashed.
const maxCollisionTries = 1024

// move implements Move per the FreeDesktop.org Trash specification.
func move(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	st, err := os.Lstat(abs)
	if err != nil {
		return err
	}

	can, err := trashCanFor(abs, st)
	if err != nil {
		return err
	}
	return can.throw(abs, st)
}

// trashCan is a resolved trash directory. relativeTo is non-empty for a
// per-volume trash, in which case the recorded Path= is relative to that
// directory (the spec's rule so the volume stays relocatable).
type trashCan struct {
	dir        string
	relativeTo string
}

// trashCanFor picks the trash directory for abs: the home trash when abs lives
// on the same filesystem as the user's data dir, otherwise a trash on abs's own
// filesystem. This is what makes trashing a file from a Samba/NFS/USB mount a
// rename instead of a multi-gigabyte copy to the home partition.
func trashCanFor(abs string, st os.FileInfo) (trashCan, error) {
	home, err := dataHome()
	if err != nil {
		return trashCan{}, err
	}

	sameFS, err := onSameFilesystem(abs, st, home)
	if err != nil {
		return trashCan{}, err
	}
	if sameFS {
		return trashCan{dir: filepath.Join(home, "Trash")}, nil
	}

	top, err := topDir(abs)
	if err != nil {
		return trashCan{}, err
	}
	dir, err := volumeTrash(top)
	if err != nil {
		return trashCan{}, err
	}
	return trashCan{dir: dir, relativeTo: top}, nil
}

// dataHome returns $XDG_DATA_HOME, defaulting to ~/.local/share. A relative
// XDG_DATA_HOME is ignored, as the XDG basedir spec requires.
func dataHome() (string, error) {
	if d := os.Getenv("XDG_DATA_HOME"); filepath.IsAbs(d) {
		return d, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share"), nil
}

// deviceOf returns the filesystem device id of path, following symlinks.
func deviceOf(path string) (uint64, error) {
	//nolint:gosec // G703: inspecting a caller-supplied path's filesystem is the
	// point of this function; it only reads metadata and opens nothing.
	st, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	sys, ok := st.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, errors.New("trash: cannot read filesystem device id")
	}
	return uint64(sys.Dev), nil
}

// onSameFilesystem reports whether abs and the home data dir live on one
// filesystem. For anything that is not a directory the *parent's* device is
// used: on overlayfs a file's st_dev differs from its directory's, and it is the
// directory that determines whether a rename can succeed.
func onSameFilesystem(abs string, st os.FileInfo, home string) (bool, error) {
	probe := abs
	if !st.IsDir() {
		probe = filepath.Dir(abs)
	}
	devFile, err := deviceOf(probe)
	if err != nil {
		return false, err
	}
	// The data dir may not exist yet; fall back to the nearest existing parent.
	devHome, err := deviceOfNearest(home)
	if err != nil {
		return false, err
	}
	return devFile == devHome, nil
}

// deviceOfNearest returns the device of path, or of its closest existing
// ancestor when path has not been created yet.
func deviceOfNearest(path string) (uint64, error) {
	for {
		dev, err := deviceOf(path)
		if err == nil {
			return dev, nil
		}
		if !os.IsNotExist(err) {
			return 0, err
		}
		parent := filepath.Dir(path)
		if parent == path {
			return 0, err
		}
		path = parent
	}
}

// topDir returns the mount point that abs resides on, found by walking up until
// the filesystem device changes.
func topDir(abs string) (string, error) {
	dir := abs
	if st, err := os.Lstat(abs); err == nil && !st.IsDir() {
		dir = filepath.Dir(abs)
	}
	dev, err := deviceOf(dir)
	if err != nil {
		return "", err
	}
	for {
		parent := filepath.Dir(dir)
		if parent == dir {
			return dir, nil
		}
		parentDev, err := deviceOf(parent)
		if err != nil || parentDev != dev {
			return dir, nil //nolint:nilerr // a stat failure above means dir is the boundary
		}
		dir = parent
	}
}

// volumeTrash resolves the trash directory on a non-home filesystem. The spec
// defines two forms, checked in order: an admin-created sticky $top/.Trash with
// a per-uid subdirectory, else a self-created $top/.Trash-$uid.
func volumeTrash(top string) (string, error) {
	uid := strconv.Itoa(os.Getuid())

	shared := filepath.Join(top, ".Trash")
	if st, err := os.Lstat(shared); err == nil && st.IsDir() && st.Mode()&os.ModeSticky != 0 &&
		st.Mode()&os.ModeSymlink == 0 {
		dir := filepath.Join(shared, uid)
		if err := os.Mkdir(dir, trashDirPerm); err == nil || os.IsExist(err) {
			if ownedByUser(dir) {
				return dir, nil
			}
		}
	}

	dir := filepath.Join(top, ".Trash-"+uid)
	if err := os.Mkdir(dir, trashDirPerm); err != nil && !os.IsExist(err) {
		return "", fmt.Errorf("trash: cannot create %s: %w", dir, err)
	}
	// Ownership can be wrong on filesystems without real uids (FAT, some SMB
	// mounts); refuse rather than write into another user's trash.
	if !ownedByUser(dir) {
		return "", fmt.Errorf("trash: %s is not owned by the current user", dir)
	}
	return dir, nil
}

// ownedByUser reports whether path exists and belongs to the current uid.
func ownedByUser(path string) bool {
	st, err := os.Lstat(path)
	if err != nil {
		return false
	}
	sys, ok := st.Sys().(*syscall.Stat_t)
	return ok && int(sys.Uid) == os.Getuid()
}

// throw performs the spec's move: reserve a name by creating its .trashinfo
// exclusively, rename the item into files/, and record the size of directories.
func (c trashCan) throw(abs string, st os.FileInfo) error {
	filesDir := filepath.Join(c.dir, "files")
	infoDir := filepath.Join(c.dir, "info")
	for _, dir := range []string{c.dir, filesDir, infoDir} {
		if err := os.MkdirAll(dir, trashDirPerm); err != nil {
			return err
		}
	}

	recorded := abs
	if c.relativeTo != "" {
		rel, err := filepath.Rel(c.relativeTo, abs)
		if err != nil {
			return err
		}
		recorded = rel
	}

	name, info, err := c.reserveName(filesDir, infoDir, filepath.Base(abs), recorded)
	if err != nil {
		return err
	}

	target := filepath.Join(filesDir, name)
	if err := os.Rename(abs, target); err != nil {
		// Never fall back to copying: the trash is on this filesystem by
		// construction, so a failure here is a real error (permissions, or a
		// bind mount of the same device) and copying would duplicate data.
		_ = os.Remove(info)
		return fmt.Errorf("trash: cannot move %s: %w", abs, err)
	}

	if st.IsDir() {
		// Best effort: a missing directorysizes entry only costs file managers
		// a recomputation, so a failure must not fail the trash operation.
		_ = c.recordDirectorySize(target, name)
	}
	return nil
}

// reserveName finds a free entry name and writes its .trashinfo, returning the
// chosen name and the info file's path. The O_EXCL create is what makes the name
// ours against a concurrent trasher.
func (c trashCan) reserveName(filesDir, infoDir, base, recorded string) (string, string, error) {
	if base == "" || base == "." || base == string(os.PathSeparator) {
		return "", "", fmt.Errorf("trash: cannot trash %q", base)
	}
	for id := 1; id <= maxCollisionTries; id++ {
		name := uniqueName(base, id)
		// A leftover file with no info entry must not be overwritten.
		if _, err := os.Lstat(filepath.Join(filesDir, name)); err == nil {
			continue
		}
		info := filepath.Join(infoDir, name+".trashinfo")
		//nolint:gosec // G304: trashing a caller-supplied path is this package's purpose;
		// the name is derived from filepath.Base and confined to infoDir.
		f, err := os.OpenFile(info, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			if os.IsExist(err) {
				continue
			}
			return "", "", err
		}
		body := fmt.Sprintf("[Trash Info]\nPath=%s\nDeletionDate=%s\n",
			escapePath(recorded), time.Now().Format("2006-01-02T15:04:05"))
		_, writeErr := f.WriteString(body)
		closeErr := f.Close()
		if err := errors.Join(writeErr, closeErr); err != nil {
			// The reservation is void, so don't leave an orphaned info file.
			_ = os.Remove(info)
			return "", "", err
		}
		return name, info, nil
	}
	return "", "", fmt.Errorf("trash: no free name for %q after %d tries", base, maxCollisionTries)
}

// recordDirectorySize appends the trashed directory's size to the trash can's
// directorysizes cache, so file managers can show it without a full walk.
func (c trashCan) recordDirectorySize(target, name string) error {
	st, err := os.Stat(target)
	if err != nil {
		return err
	}
	size, err := dirSize(target)
	if err != nil {
		return err
	}

	line := fmt.Sprintf("%d %d %s\n", size, st.ModTime().Unix(), escapePath(name))
	path := filepath.Join(c.dir, "directorysizes")
	//nolint:gosec // G304: the path is built from this trash can's own directory.
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	// A single O_APPEND write below the pipe-buffer size is atomic enough that
	// concurrent trashers cannot interleave partial lines.
	_, writeErr := f.WriteString(line)
	return errors.Join(writeErr, f.Close())
}

// dirSize sums the apparent sizes of every regular file under root.
func dirSize(root string) (int64, error) {
	var total int64
	err := filepath.WalkDir(root, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			total += info.Size()
		}
		return nil
	})
	return total, err
}

// escapePath percent-encodes a path for the Path= key of a .trashinfo file, as
// the FreeDesktop trash spec requires (RFC 2396 escaping with "/" left intact).
// Kept unescaped: alphanumerics, the unreserved marks -_.~, and the separator.
func escapePath(path string) string {
	const unreserved = "-_.~/"
	var b strings.Builder
	b.Grow(len(path))
	// Iterate bytes, not runes: multi-byte UTF-8 is escaped per byte.
	for i := 0; i < len(path); i++ {
		c := path[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
			b.WriteByte(c)
		case strings.IndexByte(unreserved, c) >= 0:
			b.WriteByte(c)
		default:
			b.WriteByte('%')
			const hex = "0123456789ABCDEF"
			b.WriteByte(hex[c>>4])
			b.WriteByte(hex[c&0x0f])
		}
	}
	return b.String()
}

// uniqueName derives the id-th candidate name for a trashed file, inserting the
// counter before the extension so restored files keep their type ("movie.mkv" →
// "movie.2.mkv"). id 1 yields the name unchanged. The split is at the *first*
// dot, matching glib's get_unique_filename so multi-part extensions survive
// ("a.tar.gz" → "a.2.tar.gz").
func uniqueName(base string, id int) string {
	if id <= 1 {
		return base
	}
	suffix := "." + strconv.Itoa(id)
	if cut := strings.IndexByte(base, '.'); cut >= 0 {
		return base[:cut] + suffix + base[cut:]
	}
	return base + suffix
}
