package player

import (
	"fmt"
	"math"
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

// humanClock formats a duration in seconds as a playback clock: "m:ss", or
// "h:mm:ss" once it passes an hour. Negative input clamps to zero.
func humanClock(seconds float64) string {
	if seconds < 0 || math.IsNaN(seconds) {
		seconds = 0
	}
	total := int(seconds)
	h, m, s := total/3600, total/60%60, total%60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

// humanRate formats a sample rate in Hz as kHz. Zero (unknown) renders "—".
func humanRate(hz float64) string {
	if hz <= 0 {
		return "—"
	}
	return fmt.Sprintf("%.1f kHz", hz/1000)
}
