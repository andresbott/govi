// Package dmg wraps a .app bundle in a macOS disk image.
//
// This is the one part of govi's macOS packaging that needs a macOS host:
// hdiutil creates the HFS+/APFS image. The staging step — the directory that
// becomes the image's root, including the /Applications symlink users drag onto
// — is plain filesystem work and runs anywhere, so it is separated out and
// tested on Linux.
package dmg

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// Options describes the image to create.
type Options struct {
	// AppDir is the .app bundle to put in the image.
	AppDir string
	// Out is the .dmg path to write.
	Out string
	// VolumeName is the name the mounted volume shows in Finder.
	VolumeName string
	// StageDir is where the image root is assembled. The caller owns it; it
	// must be empty or absent.
	StageDir string
}

// Stage assembles the image root: the bundle plus the /Applications symlink.
//
// The bundle is hard-linked when possible and copied otherwise, so a 100 MB
// binary is not duplicated on disk for no reason. The /Applications symlink is
// what makes the familiar "drag the app into Applications" window work — hdiutil
// preserves it, and without it users have to know to copy the app themselves.
func Stage(opts Options) (string, error) {
	if opts.AppDir == "" {
		return "", errors.New("dmg: AppDir is required")
	}
	if opts.StageDir == "" {
		return "", errors.New("dmg: StageDir is required")
	}
	st, err := os.Stat(opts.AppDir)
	if err != nil {
		return "", fmt.Errorf("dmg: app bundle: %w", err)
	}
	if !st.IsDir() {
		return "", fmt.Errorf("dmg: %s is not a directory", opts.AppDir)
	}

	if entries, err := os.ReadDir(opts.StageDir); err == nil && len(entries) > 0 {
		return "", fmt.Errorf("dmg: stage directory %s is not empty", opts.StageDir)
	}
	if err := os.MkdirAll(opts.StageDir, 0o755); err != nil {
		return "", fmt.Errorf("dmg: create stage dir: %w", err)
	}

	dst := filepath.Join(opts.StageDir, filepath.Base(opts.AppDir))
	if err := copyTree(opts.AppDir, dst); err != nil {
		return "", fmt.Errorf("dmg: stage bundle: %w", err)
	}

	if err := os.Symlink("/Applications", filepath.Join(opts.StageDir, "Applications")); err != nil {
		return "", fmt.Errorf("dmg: create /Applications link: %w", err)
	}
	return opts.StageDir, nil
}

// ErrNotDarwin is returned by Create off macOS.
var ErrNotDarwin = errors.New("dmg: creating a disk image requires macOS (hdiutil)")

// Create stages the bundle and turns the result into a compressed disk image.
//
// UDZO is zlib-compressed and read-only: the format every macOS release since
// 10.1 mounts without a prompt, and the one `brew` expects if the image is ever
// used as a cask download.
func Create(opts Options) (string, error) {
	if opts.Out == "" {
		return "", errors.New("dmg: Out is required")
	}
	if runtime.GOOS != "darwin" {
		return "", ErrNotDarwin
	}
	if _, err := Stage(opts); err != nil {
		return "", err
	}

	vol := opts.VolumeName
	if vol == "" {
		vol = filepath.Base(opts.AppDir)
	}

	// hdiutil refuses to write over an existing file rather than replacing it,
	// which would make a re-run fail instead of rebuilding.
	if err := os.Remove(opts.Out); err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("dmg: remove previous image: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(opts.Out), 0o755); err != nil {
		return "", fmt.Errorf("dmg: create output dir: %w", err)
	}

	// Resolved up front so a missing hdiutil reads as such, instead of surfacing
	// as an opaque exec failure from the command below.
	hdiutil, err := exec.LookPath("hdiutil")
	if err != nil {
		return "", fmt.Errorf("dmg: hdiutil not found: %w", err)
	}

	cmd := exec.Command(hdiutil, "create",
		"-volname", vol,
		"-srcfolder", opts.StageDir,
		"-fs", "HFS+",
		"-format", "UDZO",
		"-quiet",
		opts.Out,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("dmg: hdiutil create: %w: %s", err, out)
	}
	return opts.Out, nil
}

// copyTree copies src to dst recursively, preserving permission bits and
// following no symlinks (a bundle contains none).
func copyTree(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		if info.Mode()&os.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		}
		// Hard-link first: same content, no second copy of the binary. It fails
		// across filesystems, where a real copy is the only option.
		if err := os.Link(path, target); err == nil {
			return nil
		}
		return copyFile(path, target, info.Mode().Perm())
	})
}

func copyFile(src, dst string, perm os.FileMode) error {
	in, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, in, perm)
}
