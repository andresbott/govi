package player

import (
	"fmt"
	"log/slog"
	"math"
	"path/filepath"
	"strings"
)

// humanBytes formats a byte count using binary (IEC) units.
func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	units := []string{"KiB", "MiB", "GiB", "TiB"}
	return fmt.Sprintf("%.1f %s", float64(n)/float64(div), units[exp])
}

// humanBitrate formats bits/sec as kbps or Mbps. Zero (unknown) renders "—".
func humanBitrate(bitsPerSec float64) string {
	if bitsPerSec <= 0 {
		return "—"
	}
	if bitsPerSec < 1_000_000 {
		return fmt.Sprintf("%.0f kbps", bitsPerSec/1000)
	}
	return fmt.Sprintf("%.1f Mbps", bitsPerSec/1_000_000)
}

// humanRate formats a sample rate in Hz as kHz. Zero (unknown) renders "—".
func humanRate(hz float64) string {
	if hz <= 0 {
		return "—"
	}
	return fmt.Sprintf("%.1f kHz", hz/1000)
}

// humanDuration formats a length in seconds as h:mm:ss, dropping the hours
// field below an hour. A non-positive or non-finite value (live stream, or mpv
// not knowing the length yet) renders "—".
func humanDuration(seconds float64) string {
	if seconds <= 0 || math.IsNaN(seconds) || math.IsInf(seconds, 0) {
		return "—"
	}
	total := int64(seconds + 0.5)
	h, m, s := total/3600, total/60%60, total%60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

// splitPath separates a media path into the containing folder and the file
// name, which the info overlay shows on separate rows. A path with no separator
// has no folder to show, so the whole thing is the name. A URL is left whole:
// filepath would mangle its scheme ("https://h/f" -> "https:/h"), and the host
// is not a folder the user can open anyway.
func splitPath(path string) (dir, name string) {
	if path == "" || isURL(path) {
		return "", path
	}
	d, n := filepath.Split(path)
	if d == "" {
		return "", path
	}
	// filepath.Split keeps the trailing separator; drop it except at the root.
	return filepath.Clean(d), n
}

// absMediaPath anchors a relative media path to the current directory, so it
// keeps resolving after something else changes the process's cwd.
//
// Three cases are returned unchanged. The empty string means "no file, start on
// the idle screen" — filepath.Abs("") would turn that into the cwd, which mpv
// would then try to play as a directory. A URL has no cwd to be relative to, and
// filepath would mangle its scheme. And a path Abs cannot resolve (the cwd was
// deleted under us) is handed to mpv as-is rather than dropped: mpv's own error
// message for it is better than silently opening the idle screen.
func absMediaPath(path string, log *slog.Logger) string {
	if path == "" || isURL(path) {
		return path
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		log.Warn("could not resolve path, passing it to mpv unchanged", "path", path, "err", err)
		return path
	}
	return abs
}

// isURL reports whether path looks like a URL rather than a local file, going
// by the "scheme://" prefix mpv itself accepts for network streams.
func isURL(path string) bool {
	i := strings.Index(path, "://")
	if i <= 0 {
		return false
	}
	for _, r := range path[:i] {
		if !isSchemeRune(r) {
			return false
		}
	}
	return true
}

// isSchemeRune reports whether r may appear in a URL scheme (RFC 3986).
func isSchemeRune(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		return true
	case r == '+', r == '-', r == '.':
		return true
	}
	return false
}
