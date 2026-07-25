package player

import "fmt"

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
