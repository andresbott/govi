package player

import (
	"testing"

	"github.com/go-gl/glfw/v3.3/glfw"
)

func TestParseChord(t *testing.T) {
	tests := []struct {
		in      string
		want    keyChord
		wantErr bool
	}{
		{"space", keyChord{glfw.KeySpace, 0}, false},
		{"k", keyChord{glfw.KeyK, 0}, false},
		{"K", keyChord{glfw.KeyK, 0}, false}, // case-insensitive
		{"up", keyChord{glfw.KeyUp, 0}, false},
		{"+", keyChord{glfw.KeyEqual, 0}, false},
		{"-", keyChord{glfw.KeyMinus, 0}, false},
		{"?", keyChord{glfw.KeySlash, glfw.ModShift}, false},
		{"f11", keyChord{glfw.KeyF11, 0}, false},
		{"esc", keyChord{glfw.KeyEscape, 0}, false},
		{"pageup", keyChord{glfw.KeyPageUp, 0}, false},
		{"pagedown", keyChord{glfw.KeyPageDown, 0}, false},
		{"delete", keyChord{glfw.KeyDelete, 0}, false},
		{"backspace", keyChord{glfw.KeyBackspace, 0}, false},
		{"ctrl+delete", keyChord{glfw.KeyDelete, glfw.ModControl}, false},
		{"ctrl+up", keyChord{glfw.KeyUp, glfw.ModControl}, false},
		{"alt+shift+f", keyChord{glfw.KeyF, glfw.ModAlt | glfw.ModShift}, false},
		{"super+q", keyChord{glfw.KeyQ, glfw.ModSuper}, false},
		{"", keyChord{}, true},
		{"nope", keyChord{}, true},
		{"ctrl+", keyChord{}, true},
		{"ctrl+nope", keyChord{}, true},
	}
	for _, tt := range tests {
		got, err := parseChord(tt.in)
		if tt.wantErr {
			if err == nil {
				t.Errorf("parseChord(%q): expected error, got %+v", tt.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseChord(%q): unexpected error %v", tt.in, err)
			continue
		}
		if got != tt.want {
			t.Errorf("parseChord(%q) = %+v, want %+v", tt.in, got, tt.want)
		}
	}
}

func TestBuildKeymapDefaults(t *testing.T) {
	km, err := buildKeymap(nil)
	if err != nil {
		t.Fatalf("buildKeymap(nil): %v", err)
	}
	if got := km[keyChord{glfw.KeySpace, 0}]; got != actPlayPause {
		t.Errorf("space -> %q, want play-pause", got)
	}
	if got := km[keyChord{glfw.KeyEscape, 0}]; got != actQuit {
		t.Errorf("esc -> %q, want quit", got)
	}
}

// The coarse (percentage) seek shares the arrow keys with the fine seek and is
// told apart by Shift, so both must survive in the same keymap.
func TestBuildKeymapDefaultsSeekVariants(t *testing.T) {
	km, err := buildKeymap(nil)
	if err != nil {
		t.Fatalf("buildKeymap(nil): %v", err)
	}
	tests := []struct {
		chord keyChord
		want  actionID
	}{
		{keyChord{glfw.KeyRight, 0}, actSeekForward},
		{keyChord{glfw.KeyLeft, 0}, actSeekBack},
		{keyChord{glfw.KeyRight, glfw.ModShift}, actSeekForwardPct},
		{keyChord{glfw.KeyLeft, glfw.ModShift}, actSeekBackPct},
	}
	for _, tt := range tests {
		if got := km[tt.chord]; got != tt.want {
			t.Errorf("%s -> %q, want %q", chordLabel(tt.chord), got, tt.want)
		}
	}
}

func TestBuildKeymapWholesaleOverride(t *testing.T) {
	// Overriding play-pause with a single key leaves the secondary UNBOUND:
	// the default "k" must not survive.
	km, err := buildKeymap(map[actionID][]string{actPlayPause: {"p"}})
	if err != nil {
		t.Fatalf("buildKeymap: %v", err)
	}
	if got := km[keyChord{glfw.KeyP, 0}]; got != actPlayPause {
		t.Errorf("p -> %q, want play-pause", got)
	}
	if _, ok := km[keyChord{glfw.KeyK, 0}]; ok {
		t.Error("k should be unbound after wholesale override")
	}
	// An untouched action keeps its defaults.
	if got := km[keyChord{glfw.KeyM, 0}]; got != actMute {
		t.Errorf("m -> %q, want mute", got)
	}
}

func TestBuildKeymapNoneUnbinds(t *testing.T) {
	km, err := buildKeymap(map[actionID][]string{actPlayPause: {"none"}})
	if err != nil {
		t.Fatalf("buildKeymap: %v", err)
	}
	if _, ok := km[keyChord{glfw.KeySpace, 0}]; ok {
		t.Error("space should be unbound when play-pause set to none")
	}
	if _, ok := km[keyChord{glfw.KeyK, 0}]; ok {
		t.Error("k should be unbound when play-pause set to none")
	}
}

// "none" is a per-slot placeholder, so it can hold the primary slot empty
// while the secondary stays bound (the preferences window relies on this to
// keep slots from shifting when the primary is cleared).
func TestBuildKeymapNoneAsEmptySlot(t *testing.T) {
	km, err := buildKeymap(map[actionID][]string{actPlayPause: {"none", "k"}})
	if err != nil {
		t.Fatalf("buildKeymap: %v", err)
	}
	if _, ok := km[keyChord{glfw.KeySpace, 0}]; ok {
		t.Error("space should be unbound")
	}
	if got := km[keyChord{glfw.KeyK, 0}]; got != actPlayPause {
		t.Errorf("k -> %q, want play-pause", got)
	}
}

func TestBuildKeymapDuplicateChord(t *testing.T) {
	_, err := buildKeymap(map[actionID][]string{actMute: {"space"}})
	if err == nil {
		t.Fatal("expected duplicate-chord error (space already bound to play-pause)")
	}
}

func TestBuildKeymapUnparseableKey(t *testing.T) {
	_, err := buildKeymap(map[actionID][]string{actMute: {"nope"}})
	if err == nil {
		t.Fatal("expected error for unparseable key")
	}
}

func TestPickMonitor(t *testing.T) {
	monitors := []rectangle{
		{0, 0, 1920, 1080},    // left monitor
		{1920, 0, 2560, 1440}, // right monitor
	}
	tests := []struct {
		name string
		win  rectangle
		want int
	}{
		{"fully on left", rectangle{100, 100, 800, 600}, 0},
		{"fully on right", rectangle{2000, 100, 800, 600}, 1},
		{"mostly right", rectangle{1800, 100, 800, 600}, 1},
		{"mostly left", rectangle{1600, 100, 500, 600}, 0},
	}
	for _, tt := range tests {
		if got := pickMonitor(tt.win, monitors); got != tt.want {
			t.Errorf("%s: pickMonitor = %d, want %d", tt.name, got, tt.want)
		}
	}
}

func TestPickMonitorNoOverlapFallsBackToZero(t *testing.T) {
	monitors := []rectangle{{0, 0, 1920, 1080}}
	if got := pickMonitor(rectangle{5000, 5000, 100, 100}, monitors); got != 0 {
		t.Errorf("pickMonitor with no overlap = %d, want 0", got)
	}
}
