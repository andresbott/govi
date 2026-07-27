package bundle

import (
	"debug/macho"
	"encoding/binary"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

// Mach-O load commands that carry a minimum-OS version. debug/macho decodes
// neither into a typed struct, so both are read out of their raw bytes below.
const (
	// loadCmdBuildVersion is LC_BUILD_VERSION, what every current toolchain
	// emits: platform, minos, sdk, ntools.
	loadCmdBuildVersion = 0x32
	// loadCmdVersionMinMacOSX is LC_VERSION_MIN_MACOSX, the pre-10.14 form:
	// version, sdk. Still read so an older toolchain does not look like a binary
	// with no minimum at all.
	loadCmdVersionMinMacOSX = 0x24
	// platformMacOS is PLATFORM_MACOS in LC_BUILD_VERSION.
	platformMacOS = 1
)

// BinaryMinOS reports the minimum macOS version the Mach-O at path demands at
// load time, as a dotted string ("12.0.0").
//
// This is not the same claim as LSMinimumSystemVersion, and the difference is
// what shipped a broken v0.1.5: the plist is advisory metadata, while this field
// is what Launch Services enforces when Finder opens a bundle. A binary stamped
// higher than the running system is refused with "you need to upgrade to
// <version>" even though the same file execs fine from a shell, because dyld is
// laxer about it than Launch Services.
//
// The version is whatever the *linker* recorded. Under cgo that is clang, which
// defaults to the build machine's OS version unless MACOSX_DEPLOYMENT_TARGET is
// set — so this reads back what a release actually promises rather than what the
// config intended.
func BinaryMinOS(path string) (string, error) {
	f, err := macho.Open(path)
	if err != nil {
		return "", fmt.Errorf("bundle: read Mach-O %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	for _, l := range f.Loads {
		// Only load commands debug/macho has no type for arrive as raw bytes,
		// which is exactly the two this cares about.
		raw, ok := l.(macho.LoadBytes)
		if !ok {
			continue
		}
		b := raw.Raw()
		if len(b) < 8 {
			continue
		}
		switch f.ByteOrder.Uint32(b[0:4]) {
		case loadCmdBuildVersion:
			// cmd, cmdsize, platform, minos: 16 bytes to the end of minos.
			if len(b) < 16 {
				continue
			}
			if f.ByteOrder.Uint32(b[8:12]) != platformMacOS {
				continue // an iOS/catalyst entry says nothing about macOS
			}
			return decodeVersion(f.ByteOrder, b[12:16]), nil
		case loadCmdVersionMinMacOSX:
			// cmd, cmdsize, version: 12 bytes to the end of version.
			if len(b) < 12 {
				continue
			}
			return decodeVersion(f.ByteOrder, b[8:12]), nil
		}
	}
	return "", fmt.Errorf("bundle: %s declares no minimum macOS version "+
		"(no LC_BUILD_VERSION or LC_VERSION_MIN_MACOSX)", path)
}

// decodeVersion unpacks the packed form Mach-O uses for versions — 16 bits of
// major, then a byte each of minor and patch, so 0x000c0000 is 12.0.0.
func decodeVersion(order binary.ByteOrder, b []byte) string {
	v := order.Uint32(b)
	return fmt.Sprintf("%d.%d.%d", v>>16, (v>>8)&0xff, v&0xff)
}

// VerifyMinOS checks that the bundled executable's Mach-O minimum-OS version is
// the one Info.plist advertises in LSMinimumSystemVersion.
//
// Equality, not "at most": a mismatch in either direction is a packaging bug.
// Too high means no deployment target was set and the build runner's own OS
// version leaked into the binary, which makes the bundle unopenable from Finder
// on every older Mac — the v0.1.5 failure. Too low means the plist was bumped on
// its own and the two now disagree about what govi supports.
//
// A payload that is not a Mach-O at all passes: a Linux run of the packaging
// tool has nothing to check, and failing there would break the `--skip-dmg`
// rehearsal that exists to exercise this code off macOS.
func VerifyMinOS(appDir, executable, minSystem string) error {
	if minSystem == "" {
		return nil
	}
	bin := filepath.Join(appDir, macOSDir, executable)
	got, err := BinaryMinOS(bin)
	if err != nil {
		var fe *macho.FormatError
		if errors.As(err, &fe) {
			return nil
		}
		return err
	}
	if !sameVersion(got, minSystem) {
		return fmt.Errorf("bundle: %s requires macOS %s but Info.plist declares "+
			"LSMinimumSystemVersion %s; build with MACOSX_DEPLOYMENT_TARGET=%s",
			bin, got, minSystem, minSystem)
	}
	return nil
}

// sameVersion compares two dotted versions by value, so "12.0" and "12.0.0"
// match — the plist carries the short form and Mach-O the padded one.
func sameVersion(a, b string) bool {
	pa, pb := versionParts(a), versionParts(b)
	return pa == pb
}

// versionParts splits a dotted version into exactly three components, treating
// missing or unparsable ones as 0.
func versionParts(v string) [3]int {
	var out [3]int
	for i, s := range strings.SplitN(v, ".", 3) {
		n, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil {
			return out
		}
		out[i] = n
	}
	return out
}
