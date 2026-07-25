package logging

import (
	"bytes"
	"log/slog"
	"regexp"
	"strings"
	"testing"
)

func TestParseLevel(t *testing.T) {
	cases := []struct {
		in   string
		want slog.Level
	}{
		{"trace", LevelTrace},
		{"debug", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"error", slog.LevelError},
		{"WARN", slog.LevelWarn}, // case-insensitive
	}
	for _, c := range cases {
		got, err := ParseLevel(c.in)
		if err != nil {
			t.Fatalf("ParseLevel(%q) returned error: %v", c.in, err)
		}
		if got != c.want {
			t.Errorf("ParseLevel(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestParseLevelInvalid(t *testing.T) {
	if _, err := ParseLevel("bogus"); err == nil {
		t.Fatal("ParseLevel(\"bogus\") expected an error, got nil")
	}
}

func TestNewFiltersBelowLevel(t *testing.T) {
	var buf bytes.Buffer
	l := New(&buf, slog.LevelInfo)

	l.Debug("hidden message")
	l.Info("visible message")

	out := buf.String()
	if strings.Contains(out, "hidden message") {
		t.Errorf("debug message not filtered at info level:\n%s", out)
	}
	if !strings.Contains(out, "visible message") {
		t.Errorf("info message missing:\n%s", out)
	}
}

func TestTraceLevel(t *testing.T) {
	var buf bytes.Buffer
	l := New(&buf, LevelTrace)

	Trace(l, "very chatty", "frame", 42)

	out := buf.String()
	if !strings.Contains(out, "very chatty") {
		t.Fatalf("trace message missing:\n%s", out)
	}
	if !strings.Contains(out, "level=TRACE") {
		t.Errorf("expected level=TRACE label, got:\n%s", out)
	}
	if !strings.Contains(out, "frame=42") {
		t.Errorf("expected structured attr frame=42, got:\n%s", out)
	}
}

func TestTraceFilteredAtDebug(t *testing.T) {
	var buf bytes.Buffer
	l := New(&buf, slog.LevelDebug)

	Trace(l, "too chatty")

	if strings.Contains(buf.String(), "too chatty") {
		t.Errorf("trace message not filtered at debug level:\n%s", buf.String())
	}
}

func TestMicrosecondTimestamps(t *testing.T) {
	var buf bytes.Buffer
	l := New(&buf, slog.LevelInfo)

	l.Info("stamped")

	// e.g. time=15:04:05.123456
	re := regexp.MustCompile(`time=\d{2}:\d{2}:\d{2}\.\d{6}`)
	if !re.MatchString(buf.String()) {
		t.Errorf("expected microsecond timestamp, got:\n%s", buf.String())
	}
}

func TestMpvLevel(t *testing.T) {
	cases := []struct {
		in   string
		want slog.Level
	}{
		{"fatal", slog.LevelError},
		{"error", slog.LevelError},
		{"warn", slog.LevelWarn},
		{"info", slog.LevelInfo},
		{"status", slog.LevelInfo},
		{"v", slog.LevelDebug},
		{"debug", slog.LevelDebug},
		{"trace", LevelTrace},
		{"unknown-future-level", LevelTrace},
	}
	for _, c := range cases {
		if got := MpvLevel(c.in); got != c.want {
			t.Errorf("MpvLevel(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

// MpvLogLevel is the inverse direction: which log-message level govi should
// request from libmpv so that nothing the logger would show is missed, without
// flooding the event loop at low verbosity.
func TestMpvRequestLevel(t *testing.T) {
	cases := []struct {
		in   slog.Level
		want string
	}{
		{slog.LevelError, "error"},
		{slog.LevelWarn, "warn"},
		{slog.LevelInfo, "info"},
		{slog.LevelDebug, "debug"},
		{LevelTrace, "trace"},
	}
	for _, c := range cases {
		if got := MpvRequestLevel(c.in); got != c.want {
			t.Errorf("MpvRequestLevel(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}
