package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Security-critical: env capture is an allowlist, so anything that looks like a
// credential is dropped by default, never by matching a denylist of secret
// names. Keys not on the list must not reach the report.
func TestCuratedEnvKeepsAllowlistDropsSecrets(t *testing.T) {
	in := []string{
		"DISPLAY=:0",
		"WAYLAND_DISPLAY=wayland-0",
		"LD_LIBRARY_PATH=/opt/lib",
		"LC_ALL=en_US.UTF-8",
		"MESA_LOADER_DRIVER_OVERRIDE=iris",
		"AWS_SECRET_ACCESS_KEY=shh",
		"GITHUB_TOKEN=ghp_secret",
		"PASSWORD=hunter2",
		"MY_API_KEY=abc123",
		"RANDOM_VAR=whatever",
	}
	got := strings.Join(curatedEnv(in), "\n")

	for _, keep := range []string{"DISPLAY=:0", "WAYLAND_DISPLAY=wayland-0", "LD_LIBRARY_PATH=/opt/lib", "LC_ALL=en_US.UTF-8", "MESA_LOADER_DRIVER_OVERRIDE=iris"} {
		if !strings.Contains(got, keep) {
			t.Errorf("expected allowlisted var %q to be kept, got:\n%s", keep, got)
		}
	}
	for _, drop := range []string{"AWS_SECRET_ACCESS_KEY", "GITHUB_TOKEN", "PASSWORD", "MY_API_KEY", "RANDOM_VAR"} {
		if strings.Contains(got, drop) {
			t.Errorf("secret-like var %q must be dropped, got:\n%s", drop, got)
		}
	}
}

func TestCuratedEnvIsSorted(t *testing.T) {
	got := curatedEnv([]string{"PATH=/bin", "DISPLAY=:0", "LANG=C"})
	want := []string{"DISPLAY=:0", "LANG=C", "PATH=/bin"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("expected sorted output %v, got %v", want, got)
	}
}

func TestCuratedEnvSkipsMalformedEntries(t *testing.T) {
	got := curatedEnv([]string{"DISPLAY=:0", "NOEQUALS", ""})
	if strings.Join(got, ",") != "DISPLAY=:0" {
		t.Fatalf("malformed entries should be skipped, got %v", got)
	}
}

func TestCrashReportContentsIncludesAllSections(t *testing.T) {
	d := crashData{
		Reason:    "panic",
		Detail:    "runtime error: invalid memory address",
		Stack:     "goroutine 1 [running]:\nmain.boom()",
		Version:   "v1.2.3",
		BuildTime: "2026-08-23T00:00:00Z",
		Commit:    "abc1234",
		GoVersion: "go1.24",
		OS:        "linux",
		Arch:      "amd64",
		Env:       []string{"DISPLAY=:0"},
		Logs:      "12:00:00 level=ERROR msg=\"init gio gpu\"\n",
	}
	out := crashReportContents(d)

	for _, want := range []string{
		"govi crash report",
		"panic",
		"runtime error: invalid memory address",
		"v1.2.3", "abc1234", "go1.24", "linux/amd64",
		"DISPLAY=:0",
		"init gio gpu",
		"main.boom()",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected report to contain %q, got:\n%s", want, out)
		}
	}
}

func TestCrashReportContentsOmitsStackWhenEmpty(t *testing.T) {
	out := crashReportContents(crashData{Reason: "error", Detail: "boom"})
	if strings.Contains(strings.ToLower(out), "stack") {
		t.Fatalf("a report without a stack should not have a stack section, got:\n%s", out)
	}
}

func TestWriteCrashReportCreatesFileUserOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "crash_report.txt")
	if err := writeCrashReport(path, crashData{Reason: "error", Detail: "boom"}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("report not written: %v", err)
	}
	if !strings.Contains(string(b), "boom") {
		t.Fatalf("report missing detail, got:\n%s", b)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Fatalf("expected 0600 perms (report may hold paths/env), got %o", perm)
	}
}

func TestWriteCrashReportOverwrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "crash_report.txt")
	if err := writeCrashReport(path, crashData{Reason: "error", Detail: "first"}); err != nil {
		t.Fatal(err)
	}
	if err := writeCrashReport(path, crashData{Reason: "error", Detail: "second"}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if strings.Contains(string(b), "first") {
		t.Fatalf("second report should overwrite the first, got:\n%s", b)
	}
	if !strings.Contains(string(b), "second") {
		t.Fatalf("expected latest report, got:\n%s", b)
	}
}
