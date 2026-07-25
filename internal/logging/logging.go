// Package logging provides govi's structured logger: slog with an extra
// trace level for per-frame diagnostics, microsecond timestamps for latency
// forensics, and mappings to and from libmpv's log-message levels.
package logging

import (
	"fmt"
	"io"
	"log/slog"
	"strings"
)

// LevelTrace sits below slog.LevelDebug for very chatty output such as
// per-frame render timing.
const LevelTrace = slog.LevelDebug - 4

// ParseLevel converts a --log flag value into a slog level.
func ParseLevel(s string) (slog.Level, error) {
	switch strings.ToLower(s) {
	case "trace":
		return LevelTrace, nil
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("invalid log level %q (want trace, debug, info, warn or error)", s)
	}
}

// New returns a text logger writing to w at the given level, with
// microsecond timestamps and TRACE labeled as such instead of DEBUG-4.
func New(w io.Writer, level slog.Level) *slog.Logger {
	h := slog.NewTextHandler(w, &slog.HandlerOptions{
		Level: level,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			switch a.Key {
			case slog.TimeKey:
				a.Value = slog.StringValue(a.Value.Time().Format("15:04:05.000000"))
			case slog.LevelKey:
				if lvl, ok := a.Value.Any().(slog.Level); ok && lvl == LevelTrace {
					a.Value = slog.StringValue("TRACE")
				}
			}
			return a
		},
	})
	return slog.New(h)
}

// Trace logs msg at trace level.
func Trace(l *slog.Logger, msg string, args ...any) {
	l.Log(nil, LevelTrace, msg, args...) //nolint:staticcheck // slog accepts a nil context
}

// MpvLevel maps a libmpv log-message prefix level (as delivered in
// mpv_event_log_message) to the corresponding slog level. Unknown levels map
// to trace so nothing is dropped silently.
func MpvLevel(s string) slog.Level {
	switch s {
	case "fatal", "error":
		return slog.LevelError
	case "warn":
		return slog.LevelWarn
	case "info", "status":
		return slog.LevelInfo
	case "v", "debug":
		return slog.LevelDebug
	default:
		return LevelTrace
	}
}

// MpvRequestLevel returns the mpv_request_log_messages level that covers
// everything the logger would show at the given level, without requesting
// more than needed.
func MpvRequestLevel(level slog.Level) string {
	switch {
	case level <= LevelTrace:
		return "trace"
	case level <= slog.LevelDebug:
		return "debug"
	case level <= slog.LevelInfo:
		return "info"
	case level <= slog.LevelWarn:
		return "warn"
	default:
		return "error"
	}
}
