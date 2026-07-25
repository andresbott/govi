package player

import "testing"

// labelsOf flattens top-level menu labels (ignoring separators) for assertions.
func labelsOf(items []*menuItem) []string {
	var out []string
	for _, it := range items {
		if it.separator {
			continue
		}
		out = append(out, it.label)
	}
	return out
}

func containsLabel(items []*menuItem, want string) bool {
	for _, l := range labelsOf(items) {
		if l == want {
			return true
		}
	}
	return false
}

func TestBuildMenuIdleShowsLimitedEntries(t *testing.T) {
	p := &Player{actions: actionByID()}
	items := p.buildMenu(true)
	for _, want := range []string{"Shortcut Help", "Fullscreen", "Quit"} {
		if !containsLabel(items, want) {
			t.Errorf("idle menu missing %q; got %v", want, labelsOf(items))
		}
	}
	for _, notWant := range []string{"Play / Pause", "Audio", "Video", "Subtitles", "Stop"} {
		if containsLabel(items, notWant) {
			t.Errorf("idle menu should not contain %q; got %v", notWant, labelsOf(items))
		}
	}
}

func TestBuildMenuPlayingShowsPlaybackEntries(t *testing.T) {
	p := &Player{actions: actionByID()}
	items := p.buildMenu(false)
	for _, want := range []string{"Play / Pause", "Stop", "Audio", "Video", "Subtitles", "Video Info", "Shortcut Help", "Fullscreen", "Quit"} {
		if !containsLabel(items, want) {
			t.Errorf("playing menu missing %q; got %v", want, labelsOf(items))
		}
	}
}

func TestBuildMenuContainsPreferences(t *testing.T) {
	p := &Player{actions: actionByID()}
	if !containsLabel(p.buildMenu(false), "Preferences") {
		t.Errorf("playing menu missing Preferences; got %v", labelsOf(p.buildMenu(false)))
	}
	if !containsLabel(p.buildMenu(true), "Preferences") {
		t.Errorf("idle menu missing Preferences; got %v", labelsOf(p.buildMenu(true)))
	}
}
