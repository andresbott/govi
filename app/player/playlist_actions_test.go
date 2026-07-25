package player

import (
	"strings"
	"testing"
)

func TestRegistryHasNextPrevVideoActions(t *testing.T) {
	byID := actionByID()
	next, ok := byID[actNextVideo]
	if !ok {
		t.Fatal("registry missing next-video action")
	}
	if next.label != "Next Video" {
		t.Errorf("next-video label = %q, want \"Next Video\"", next.label)
	}
	prev, ok := byID[actPrevVideo]
	if !ok {
		t.Fatal("registry missing previous-video action")
	}
	if prev.label != "Previous Video" {
		t.Errorf("previous-video label = %q, want \"Previous Video\"", prev.label)
	}
}

func TestNextPrevVideoDefaultKeys(t *testing.T) {
	km, err := buildKeymap(nil)
	if err != nil {
		t.Fatal(err)
	}
	rows := helpRows(defaultActions(), km)
	byLabel := map[string]string{}
	for _, r := range rows {
		byLabel[r.label] = r.keys
	}
	if got := byLabel["Next Video"]; !strings.Contains(got, "pagedown") {
		t.Errorf("Next Video keys = %q, want to contain pagedown", got)
	}
	if got := byLabel["Previous Video"]; !strings.Contains(got, "pageup") {
		t.Errorf("Previous Video keys = %q, want to contain pageup", got)
	}
}

func TestBuildMenuPlayingShowsNextPrev(t *testing.T) {
	p := &Player{actions: actionByID()}
	items := p.buildMenu(false)
	for _, want := range []string{"Next Video", "Previous Video"} {
		if !containsLabel(items, want) {
			t.Errorf("playing menu missing %q; got %v", want, labelsOf(items))
		}
	}
}

func TestBuildMenuIdleHidesNextPrev(t *testing.T) {
	p := &Player{actions: actionByID()}
	items := p.buildMenu(true)
	for _, notWant := range []string{"Next Video", "Previous Video"} {
		if containsLabel(items, notWant) {
			t.Errorf("idle menu should not contain %q; got %v", notWant, labelsOf(items))
		}
	}
}
