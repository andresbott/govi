package zarf_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// writeStubOtool creates an executable stub that prints the given otool -L
// output, and returns its path. This is what lets the darwin-only linkage check
// be tested on Linux: the script takes its otool from $OTOOL.
func writeStubOtool(t *testing.T, output string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "otool")
	script := "#!/bin/sh\ncat <<'EOF'\n" + output + "\nEOF\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("writing stub otool: %v", err)
	}
	return path
}

// runCheck runs the script with a stubbed otool against a dummy binary and
// returns its exit code plus combined output.
func runCheck(t *testing.T, otoolOutput string) (int, string) {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "govi")
	if err := os.WriteFile(bin, []byte("not a real binary"), 0o755); err != nil {
		t.Fatalf("writing fake binary: %v", err)
	}
	cmd := exec.Command("./darwin-check-libmpv.sh", bin)
	cmd.Env = append(os.Environ(), "OTOOL="+writeStubOtool(t, otoolOutput))
	out, err := cmd.CombinedOutput()
	if err == nil {
		return 0, string(out)
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("running script: %v (output: %s)", err, out)
	}
	return exitErr.ExitCode(), string(out)
}

func TestLinkageCheckAcceptsOptPath(t *testing.T) {
	code, out := runCheck(t, "govi:\n\t/opt/homebrew/opt/mpv/lib/libmpv.2.dylib (compatibility version 1.0.0, current version 2.0.0)\n\t/usr/lib/libSystem.B.dylib (compatibility version 1.0.0)")
	if code != 0 {
		t.Errorf("opt-path linkage should pass, got exit %d; output:\n%s", code, out)
	}
}

func TestLinkageCheckRejectsCellarPath(t *testing.T) {
	code, out := runCheck(t, "govi:\n\t/opt/homebrew/Cellar/mpv/0.41.0_6/lib/libmpv.2.dylib (compatibility version 1.0.0)")
	if code != 1 {
		t.Errorf("Cellar-pinned linkage must fail with exit 1, got %d; output:\n%s", code, out)
	}
}

func TestLinkageCheckRejectsMissingLibmpv(t *testing.T) {
	code, out := runCheck(t, "govi:\n\t/usr/lib/libSystem.B.dylib (compatibility version 1.0.0)")
	if code != 1 {
		t.Errorf("a binary with no libmpv reference must fail with exit 1, got %d; output:\n%s", code, out)
	}
}

func TestLinkageCheckRejectsBadUsage(t *testing.T) {
	out, err := exec.Command("./darwin-check-libmpv.sh").CombinedOutput()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected a non-zero exit with no arguments, got err=%v output=%s", err, out)
	}
	if exitErr.ExitCode() != 2 {
		t.Errorf("no arguments must exit 2, got %d; output:\n%s", exitErr.ExitCode(), out)
	}
}

func TestLinkageCheckRejectsMissingBinary(t *testing.T) {
	cmd := exec.Command("./darwin-check-libmpv.sh", filepath.Join(t.TempDir(), "does-not-exist"))
	cmd.Env = append(os.Environ(), "OTOOL="+writeStubOtool(t, "irrelevant"))
	out, err := cmd.CombinedOutput()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected a non-zero exit for a missing binary, got err=%v output=%s", err, out)
	}
	if exitErr.ExitCode() != 2 {
		t.Errorf("a missing binary must exit 2, got %d; output:\n%s", exitErr.ExitCode(), out)
	}
}
