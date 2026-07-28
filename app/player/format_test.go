package player

import (
	"io"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestHumanBytes(t *testing.T) {
	tests := []struct {
		in   int64
		want string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.0 KiB"},
		{1536, "1.5 KiB"},
		{1048576, "1.0 MiB"},
		{1572864, "1.5 MiB"},
		{1073741824, "1.0 GiB"},
	}
	for _, tt := range tests {
		if got := humanBytes(tt.in); got != tt.want {
			t.Errorf("humanBytes(%d) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestHumanBitrate(t *testing.T) {
	tests := []struct {
		in   float64
		want string
	}{
		{0, "—"},
		{500_000, "500 kbps"},
		{1_500_000, "1.5 Mbps"},
		{8_000_000, "8.0 Mbps"},
	}
	for _, tt := range tests {
		if got := humanBitrate(tt.in); got != tt.want {
			t.Errorf("humanBitrate(%v) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestHumanRate(t *testing.T) {
	tests := []struct {
		in   float64
		want string
	}{
		{0, "—"},
		{44100, "44.1 kHz"},
		{48000, "48.0 kHz"},
	}
	for _, tt := range tests {
		if got := humanRate(tt.in); got != tt.want {
			t.Errorf("humanRate(%v) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestHumanDuration(t *testing.T) {
	tests := []struct {
		in   float64
		want string
	}{
		{0, "—"},
		{-1, "—"},
		{math.NaN(), "—"},
		{math.Inf(1), "—"},
		{5, "0:05"},
		{65, "1:05"},
		{599.4, "9:59"},
		{3600, "1:00:00"},
		{3661, "1:01:01"},
		{7325.6, "2:02:06"},
	}
	for _, tt := range tests {
		if got := humanDuration(tt.in); got != tt.want {
			t.Errorf("humanDuration(%v) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestSplitPath(t *testing.T) {
	tests := []struct {
		in       string
		wantDir  string
		wantName string
	}{
		{"", "", ""},
		{"/home/u/videos/clip.mp4", "/home/u/videos", "clip.mp4"},
		{"clip.mp4", "", "clip.mp4"},
		{"videos/clip.mp4", "videos", "clip.mp4"},
		{"/clip.mp4", "/", "clip.mp4"},
		// A URL is left whole — filepath would mangle the scheme.
		{"https://example.com/stream.m3u8", "", "https://example.com/stream.m3u8"},
		{"rtmp://host/live", "", "rtmp://host/live"},
	}
	for _, tt := range tests {
		dir, name := splitPath(tt.in)
		if dir != tt.wantDir || name != tt.wantName {
			t.Errorf("splitPath(%q) = (%q, %q), want (%q, %q)", tt.in, dir, name, tt.wantDir, tt.wantName)
		}
	}
}

// The empty string is the case worth pinning: it means "no file, start idle",
// and filepath.Abs("") returns the working directory, which mpv would then try
// to play as a directory. Passing "" through unchanged is what keeps Run on its
// idle-screen branch.
func TestAbsMediaPathLeavesEmptyAndURLsAlone(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	for _, in := range []string{
		"",
		"https://example.com/stream.m3u8",
		"rtmp://host/live",
	} {
		if got := absMediaPath(in, log); got != in {
			t.Errorf("absMediaPath(%q) = %q, want it unchanged", in, got)
		}
	}
}

// A relative path must come back anchored to the directory govi was started in,
// because GLFW may chdir during initialisation on a bundled macOS build and mpv
// resolves relative paths against the cwd it is called with.
func TestAbsMediaPathAnchorsRelativePaths(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}

	got := absMediaPath("clip.mp4", log)
	if want := filepath.Join(cwd, "clip.mp4"); got != want {
		t.Errorf("absMediaPath(%q) = %q, want %q", "clip.mp4", got, want)
	}
	if !filepath.IsAbs(got) {
		t.Errorf("absMediaPath(%q) = %q, want an absolute path", "clip.mp4", got)
	}

	// An already-absolute path is returned as it was, not rewritten.
	abs := filepath.Join(cwd, "sub", "clip.mkv")
	if got := absMediaPath(abs, log); got != abs {
		t.Errorf("absMediaPath(%q) = %q, want it unchanged", abs, got)
	}
}
