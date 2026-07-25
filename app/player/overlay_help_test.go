package player

import (
	"strings"
	"testing"

	"github.com/go-gl/glfw/v3.3/glfw"
)

func TestChordLabel(t *testing.T) {
	tests := []struct {
		c    keyChord
		want string
	}{
		{keyChord{glfw.KeySpace, 0}, "space"},
		{keyChord{glfw.KeyK, 0}, "k"},
		{keyChord{glfw.KeyUp, 0}, "up"},
		{keyChord{glfw.KeySlash, glfw.ModShift}, "?"},
		{keyChord{glfw.KeyF11, 0}, "f11"},
		{keyChord{glfw.KeyPageUp, 0}, "pageup"},
		{keyChord{glfw.KeyPageDown, 0}, "pagedown"},
		{keyChord{glfw.KeyUp, glfw.ModControl}, "ctrl+up"},
	}
	for _, tt := range tests {
		if got := chordLabel(tt.c); got != tt.want {
			t.Errorf("chordLabel(%+v) = %q, want %q", tt.c, got, tt.want)
		}
	}
}

func TestHelpRowsMenuOnlyMarked(t *testing.T) {
	km, err := buildKeymap(nil)
	if err != nil {
		t.Fatal(err)
	}
	rows := helpRows(defaultActions(), km)
	byLabel := map[string]string{}
	for _, r := range rows {
		byLabel[r.label] = r.keys
	}
	if got := byLabel["Play / Pause"]; !strings.Contains(got, "space") {
		t.Errorf("Play / Pause keys = %q, want to contain space", got)
	}
	// stop has no default binding -> "menu"
	if got := byLabel["Stop"]; got != "menu" {
		t.Errorf("Stop keys = %q, want \"menu\"", got)
	}
}

func TestHelpRowsReflectRebinding(t *testing.T) {
	km, err := buildKeymap(map[actionID][]string{actPlayPause: {"p"}})
	if err != nil {
		t.Fatal(err)
	}
	rows := helpRows(defaultActions(), km)
	for _, r := range rows {
		if r.label == "Play / Pause" {
			if !strings.Contains(r.keys, "p") || strings.Contains(r.keys, "space") {
				t.Errorf("rebinding not reflected: %q", r.keys)
			}
		}
	}
}

func TestHelpRowsPreferencesShowsMenuMarker(t *testing.T) {
	km, err := buildKeymap(nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range helpRows(defaultActions(), km) {
		if r.label == "Preferences" {
			if r.keys != "menu" {
				t.Errorf("Preferences keys = %q, want \"menu\"", r.keys)
			}
			return
		}
	}
	t.Error("no Preferences row in help")
}
