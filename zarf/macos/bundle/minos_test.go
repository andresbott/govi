package bundle

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// machO builds a minimal 64-bit little-endian Mach-O carrying exactly the load
// commands given, each already encoded by one of the helpers below.
//
// Synthesised rather than compiled from a fixture, because the interesting cases
// are versions no toolchain on the test machine would emit — the whole point is
// to assert on a minos the build did not intend.
func machO(cmds ...[]byte) []byte {
	var body []byte
	for _, c := range cmds {
		body = append(body, c...)
	}
	hdr := make([]byte, 32)
	le := binary.LittleEndian
	le.PutUint32(hdr[0:], 0xfeedfacf)         // Magic64
	le.PutUint32(hdr[4:], 0x0100000c)         // CpuArm64
	le.PutUint32(hdr[8:], 0)                  // cpu subtype
	le.PutUint32(hdr[12:], 2)                 // MH_EXECUTE
	le.PutUint32(hdr[16:], uint32(len(cmds))) // ncmds
	le.PutUint32(hdr[20:], uint32(len(body))) // sizeofcmds
	le.PutUint32(hdr[24:], 0)                 // flags
	le.PutUint32(hdr[28:], 0)                 // reserved
	return append(hdr, body...)
}

// packVersion is the xxxx.yy.zz form Mach-O stores versions in.
func packVersion(major, minor, patch uint32) uint32 {
	return major<<16 | minor<<8 | patch
}

// buildVersionCmd encodes LC_BUILD_VERSION for the given platform and minos.
func buildVersionCmd(platform, major, minor uint32) []byte {
	b := make([]byte, 24)
	le := binary.LittleEndian
	le.PutUint32(b[0:], loadCmdBuildVersion)
	le.PutUint32(b[4:], uint32(len(b)))
	le.PutUint32(b[8:], platform)
	le.PutUint32(b[12:], packVersion(major, minor, 0))
	le.PutUint32(b[16:], packVersion(26, 5, 0)) // sdk; never asserted on
	le.PutUint32(b[20:], 0)                     // ntools
	return b
}

// versionMinCmd encodes the pre-10.14 LC_VERSION_MIN_MACOSX.
func versionMinCmd(major, minor uint32) []byte {
	b := make([]byte, 16)
	le := binary.LittleEndian
	le.PutUint32(b[0:], loadCmdVersionMinMacOSX)
	le.PutUint32(b[4:], uint32(len(b)))
	le.PutUint32(b[8:], packVersion(major, minor, 0))
	le.PutUint32(b[12:], packVersion(major, minor, 0)) // sdk
	return b
}

// segmentCmd is a load command BinaryMinOS must skip over, so the tests prove it
// scans rather than only reading the first command.
func segmentCmd() []byte {
	b := make([]byte, 72)
	le := binary.LittleEndian
	le.PutUint32(b[0:], 0x19) // LC_SEGMENT_64
	le.PutUint32(b[4:], uint32(len(b)))
	copy(b[8:], "__PAGEZERO")
	return b
}

// writeMachO puts data at dir/govi and returns the path.
func writeMachO(t *testing.T, dir string, data []byte) string {
	t.Helper()
	p := filepath.Join(dir, "govi")
	if err := os.WriteFile(p, data, 0o755); err != nil {
		t.Fatalf("write Mach-O: %v", err)
	}
	return p
}

func TestBinaryMinOS(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want string
	}{
		{
			name: "LC_BUILD_VERSION",
			data: machO(buildVersionCmd(platformMacOS, 12, 0)),
			want: "12.0.0",
		},
		{
			name: "not the first load command",
			data: machO(segmentCmd(), buildVersionCmd(platformMacOS, 12, 3)),
			want: "12.3.0",
		},
		{
			name: "the runner-OS value v0.1.5 shipped",
			data: machO(buildVersionCmd(platformMacOS, 26, 0)),
			want: "26.0.0",
		},
		{
			name: "legacy LC_VERSION_MIN_MACOSX",
			data: machO(versionMinCmd(11, 0)),
			want: "11.0.0",
		},
		{
			name: "a non-macOS platform is skipped in favour of the macOS one",
			data: machO(buildVersionCmd(2 /* iOS */, 18, 0), buildVersionCmd(platformMacOS, 12, 0)),
			want: "12.0.0",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := BinaryMinOS(writeMachO(t, t.TempDir(), tc.data))
			if err != nil {
				t.Fatalf("BinaryMinOS: %v", err)
			}
			if got != tc.want {
				t.Errorf("BinaryMinOS = %q, want %q", got, tc.want)
			}
		})
	}
}

// A Mach-O with no version command at all must be an error, not an empty string
// that silently satisfies the caller's comparison.
func TestBinaryMinOSWithoutAVersionCommand(t *testing.T) {
	_, err := BinaryMinOS(writeMachO(t, t.TempDir(), machO(segmentCmd())))
	if err == nil {
		t.Fatal("BinaryMinOS succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "no minimum macOS version") {
		t.Errorf("error = %v, want it to name the missing version command", err)
	}
}

// bundleWith writes a .app whose payload is data, and returns the .app path.
func bundleWith(t *testing.T, data []byte) string {
	t.Helper()
	app := filepath.Join(t.TempDir(), "govi.app")
	if err := os.MkdirAll(filepath.Join(app, macOSDir), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeMachO(t, filepath.Join(app, macOSDir), data)
	return app
}

func TestVerifyMinOSAccepts(t *testing.T) {
	tests := []struct {
		name      string
		data      []byte
		minSystem string
	}{
		{
			name:      "the binary agrees with the plist",
			data:      machO(buildVersionCmd(platformMacOS, 12, 0)),
			minSystem: "12.0",
		},
		{
			// The plist carries "12.0" and Mach-O "12.0.0"; comparing the strings
			// would reject every correct build.
			name:      "a padded Mach-O version against a short plist one",
			data:      machO(buildVersionCmd(platformMacOS, 12, 0)),
			minSystem: "12.0.0",
		},
		{
			name:      "no minimum declared means nothing to check",
			data:      machO(buildVersionCmd(platformMacOS, 26, 0)),
			minSystem: "",
		},
		{
			// A Linux run of the packaging tool: the payload is an ELF, and there
			// is no macOS claim to contradict.
			name:      "the payload is not a Mach-O",
			data:      []byte("\x7fELF not a mach-o at all"),
			minSystem: "12.0",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := VerifyMinOS(bundleWith(t, tc.data), "govi", tc.minSystem); err != nil {
				t.Errorf("VerifyMinOS: %v", err)
			}
		})
	}
}

// This is the v0.1.5 regression: the plist says 12.0, the binary demands the
// build runner's macOS 26, and Finder refuses to open the bundle. The build must
// fail here instead.
func TestVerifyMinOSRejectsARunnerOSStamp(t *testing.T) {
	err := VerifyMinOS(bundleWith(t, machO(buildVersionCmd(platformMacOS, 26, 0))), "govi", "12.0")
	if err == nil {
		t.Fatal("VerifyMinOS succeeded, want an error")
	}
	// The message has to name both versions and the fix, since whoever reads it
	// is looking at a red release build with no Mac in front of them.
	for _, want := range []string{"26.0.0", "12.0", "MACOSX_DEPLOYMENT_TARGET"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %v, want it to mention %q", err, want)
		}
	}
}

// The other direction is a bug too: bumping the plist alone would advertise a
// floor the binary does not enforce.
func TestVerifyMinOSRejectsAPlistBumpedOnItsOwn(t *testing.T) {
	if err := VerifyMinOS(bundleWith(t, machO(buildVersionCmd(platformMacOS, 12, 0))), "govi", "13.0"); err == nil {
		t.Fatal("VerifyMinOS succeeded, want an error")
	}
}

// A missing payload must be reported, not treated as "no Mach-O, nothing to
// check" — that would make the guard vacuous exactly when the bundle is broken.
func TestVerifyMinOSRejectsAMissingBinary(t *testing.T) {
	app := filepath.Join(t.TempDir(), "govi.app")
	if err := os.MkdirAll(filepath.Join(app, macOSDir), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := VerifyMinOS(app, "govi", "12.0"); err == nil {
		t.Fatal("VerifyMinOS succeeded on a bundle with no executable, want an error")
	}
}
