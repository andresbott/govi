// Package bundle assembles a macOS .app bundle around an already-compiled
// binary.
//
// A .app is a directory with a fixed layout and an XML manifest — no Apple
// tooling is involved in creating one, which is why this can run (and be
// tested) on Linux even though the result only means anything on macOS:
//
//	govi.app/Contents/Info.plist
//	govi.app/Contents/MacOS/govi
//	govi.app/Contents/Resources/govi.icns
//
// govi needs the bundle for the things a bare binary on PATH cannot have: a
// Dock icon, a Launchpad entry, a real app name in the menu bar, and an entry
// in Finder's "Open with" menu. It is the macOS counterpart of what
// zarf/govi.desktop does for the .deb.
package bundle

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// Layout is the on-disk shape of a bundle, relative to the .app directory.
const (
	contentsDir   = "Contents"
	macOSDir      = "Contents/MacOS"
	resourcesDir  = "Contents/Resources"
	infoPlistName = "Contents/Info.plist"
)

// Options describes a bundle to build.
type Options struct {
	// AppDir is the .app directory to create, e.g. "dist/govi.app". It is
	// created if missing; existing files inside it are overwritten.
	AppDir string
	// Binary is the path of the compiled executable to place inside
	// Contents/MacOS. Ignored when empty, which is how the caller handles a
	// binary goreleaser already wrote into the right place.
	Binary string
	// Icns is the .icns content for Contents/Resources. Optional.
	Icns []byte
	// Info is the metadata for Contents/Info.plist. Info.Executable names the
	// file inside Contents/MacOS and must be set.
	Info Info
}

// Build writes the bundle described by opts and returns the .app path.
//
// The executable bit is set explicitly on the binary rather than copied from
// the source: goreleaser's build output is already executable, but a bundle
// whose CFBundleExecutable is not executable fails to launch with an
// unhelpful Finder error, so it is cheap insurance.
func Build(opts Options) (string, error) {
	if opts.AppDir == "" {
		return "", errors.New("bundle: AppDir is required")
	}
	if filepath.Ext(opts.AppDir) != ".app" {
		return "", fmt.Errorf("bundle: AppDir %q must end in .app", opts.AppDir)
	}
	if opts.Info.Executable == "" {
		return "", errors.New("bundle: Info.Executable is required")
	}

	plist, err := InfoPlist(opts.Info)
	if err != nil {
		return "", err
	}

	for _, dir := range []string{contentsDir, macOSDir, resourcesDir} {
		if err := os.MkdirAll(filepath.Join(opts.AppDir, dir), 0o755); err != nil {
			return "", fmt.Errorf("bundle: create %s: %w", dir, err)
		}
	}

	if opts.Binary != "" {
		dst := filepath.Join(opts.AppDir, macOSDir, opts.Info.Executable)
		if err := copyFile(opts.Binary, dst, 0o755); err != nil {
			return "", fmt.Errorf("bundle: place executable: %w", err)
		}
	}

	if len(opts.Icns) > 0 {
		name := opts.Info.Icon
		if name == "" {
			return "", errors.New("bundle: Icns given but Info.Icon names no file")
		}
		if filepath.Ext(name) != ".icns" {
			name += ".icns"
		}
		dst := filepath.Join(opts.AppDir, resourcesDir, name)
		if err := os.WriteFile(dst, opts.Icns, 0o644); err != nil {
			return "", fmt.Errorf("bundle: write icon: %w", err)
		}
	}

	if err := os.WriteFile(filepath.Join(opts.AppDir, infoPlistName), plist, 0o644); err != nil {
		return "", fmt.Errorf("bundle: write Info.plist: %w", err)
	}

	return opts.AppDir, nil
}

// Verify checks that appDir is a bundle macOS will actually launch: the
// manifest exists and the executable it names is present and executable.
//
// This is the guard that makes a silent packaging mistake loud. Every failure
// mode it covers (an Info.plist naming a binary that was never copied, a
// non-executable payload) produces a bundle that installs fine and then fails
// to open with a message that names nothing useful.
func Verify(appDir, executable string) error {
	plist := filepath.Join(appDir, infoPlistName)
	if _, err := os.Stat(plist); err != nil {
		return fmt.Errorf("bundle: %s: %w", infoPlistName, err)
	}
	bin := filepath.Join(appDir, macOSDir, executable)
	st, err := os.Stat(bin)
	if err != nil {
		return fmt.Errorf("bundle: executable: %w", err)
	}
	if st.IsDir() {
		return fmt.Errorf("bundle: %s is a directory, want the executable", bin)
	}
	if st.Mode().Perm()&0o111 == 0 {
		return fmt.Errorf("bundle: %s is not executable (mode %v)", bin, st.Mode().Perm())
	}
	if st.Size() == 0 {
		return fmt.Errorf("bundle: %s is empty", bin)
	}
	return nil
}

// copyFile copies src to dst with the given mode, replacing dst if present.
func copyFile(src, dst string, mode fs.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	// Truncate rather than append, so re-running over a previous build does not
	// concatenate two binaries into an unrunnable file.
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	// OpenFile honours the mode only when it creates the file, so an existing
	// dst keeps its old permissions without this.
	return os.Chmod(dst, mode)
}
