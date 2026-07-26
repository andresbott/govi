package player

import (
	"strings"
	"testing"
)

func TestSeekCommandIsRelativeSeconds(t *testing.T) {
	got := strings.Join(seekCommand(5), " ")
	if want := "seek 5 relative"; got != want {
		t.Errorf("seekCommand(5) = %q, want %q", got, want)
	}
	got = strings.Join(seekCommand(-5), " ")
	if want := "seek -5 relative"; got != want {
		t.Errorf("seekCommand(-5) = %q, want %q", got, want)
	}
}

// The coarse seek is a share of the file's duration, which mpv computes itself
// via relative-percent — the player never has to read `duration` for it.
func TestSeekPercentCommandIsRelativePercent(t *testing.T) {
	got := strings.Join(seekPercentCommand(seekPercentStep), " ")
	if want := "seek 10 relative-percent"; got != want {
		t.Errorf("seekPercentCommand(10) = %q, want %q", got, want)
	}
	got = strings.Join(seekPercentCommand(-seekPercentStep), " ")
	if want := "seek -10 relative-percent"; got != want {
		t.Errorf("seekPercentCommand(-10) = %q, want %q", got, want)
	}
}
