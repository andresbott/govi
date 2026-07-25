package player

import (
	"reflect"
	"testing"

	"github.com/go-gl/glfw/v3.3/glfw"
)

func TestEffectiveKeys(t *testing.T) {
	cases := []struct {
		name      string
		id        actionID
		overrides map[actionID][]string
		want      []string
	}{
		{"defaults when no override", actPlayPause, nil, []string{"space", "k"}},
		{"override wins", actPlayPause, map[actionID][]string{actPlayPause: {"p"}}, []string{"p"}},
		{"none means unbound", actPlayPause, map[actionID][]string{actPlayPause: {"none"}}, nil},
		{"no defaults no override", actStop, nil, nil},
		// Slots are positional: an empty primary stays empty, the secondary
		// does not move up into it.
		{"empty primary keeps position", actPlayPause,
			map[actionID][]string{actPlayPause: {"none", "k"}}, []string{"", "k"}},
		{"trailing empty slot trimmed", actPlayPause,
			map[actionID][]string{actPlayPause: {"space", "none"}}, []string{"space"}},
		{"all slots empty is unbound", actPlayPause,
			map[actionID][]string{actPlayPause: {"none", "none"}}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := effectiveKeys(c.id, c.overrides); !reflect.DeepEqual(got, c.want) {
				t.Errorf("effectiveKeys(%s) = %v, want %v", c.id, got, c.want)
			}
		})
	}
}

// prefsPlayer builds a Player with just enough state for binding logic.
func prefsPlayer(t *testing.T) (*Player, *map[string][]string) {
	t.Helper()
	km, err := buildKeymap(nil)
	if err != nil {
		t.Fatal(err)
	}
	var saved map[string][]string
	p := &Player{
		actions:   actionByID(),
		keymap:    km,
		overrides: map[actionID][]string{},
	}
	p.saveShortcuts = func(sc map[string][]string) error {
		saved = sc
		return nil
	}
	return p, &saved
}

func TestApplyBindingSetsSlotAndSaves(t *testing.T) {
	p, saved := prefsPlayer(t)
	if err := p.applyBinding(actMute, 0, "x"); err != nil {
		t.Fatalf("applyBinding: %v", err)
	}
	if got := effectiveKeys(actMute, p.overrides); !reflect.DeepEqual(got, []string{"x"}) {
		t.Errorf("mute keys = %v, want [x]", got)
	}
	if id := p.keymap[mustChord(t, "x")]; id != actMute {
		t.Errorf("keymap[x] = %q, want mute", id)
	}
	// The old default binding is gone (override replaces both slots).
	if id, ok := p.keymap[mustChord(t, "m")]; ok {
		t.Errorf("keymap[m] still bound to %q", id)
	}
	if got := (*saved)["mute"]; !reflect.DeepEqual(got, []string{"x"}) {
		t.Errorf("saved mute = %v, want [x]", got)
	}
}

func TestApplyBindingSecondSlotKeepsFirst(t *testing.T) {
	p, _ := prefsPlayer(t)
	if err := p.applyBinding(actMute, 1, "x"); err != nil {
		t.Fatalf("applyBinding: %v", err)
	}
	if got := effectiveKeys(actMute, p.overrides); !reflect.DeepEqual(got, []string{"m", "x"}) {
		t.Errorf("mute keys = %v, want [m x]", got)
	}
}

// Clearing the primary must leave the secondary in slot 1; promoting it would
// silently reshuffle the user's layout.
func TestApplyBindingClearPrimaryKeepsSecondaryInPlace(t *testing.T) {
	p, saved := prefsPlayer(t)
	if err := p.applyBinding(actPlayPause, 0, ""); err != nil {
		t.Fatalf("clear primary: %v", err)
	}
	if got := effectiveKeys(actPlayPause, p.overrides); !reflect.DeepEqual(got, []string{"", "k"}) {
		t.Errorf("play-pause keys = %v, want [\"\" k]", got)
	}
	if got := (*saved)["play-pause"]; !reflect.DeepEqual(got, []string{"none", "k"}) {
		t.Errorf("saved play-pause = %v, want [none k]", got)
	}
	// The cleared key is gone from dispatch, the kept one still works.
	if _, ok := p.keymap[mustChord(t, "space")]; ok {
		t.Error("keymap[space] should be unbound")
	}
	if id := p.keymap[mustChord(t, "k")]; id != actPlayPause {
		t.Errorf("keymap[k] = %q, want play-pause", id)
	}
}

// A key bound into the still-empty primary slot must not disturb the secondary.
func TestApplyBindingFillsClearedPrimary(t *testing.T) {
	p, _ := prefsPlayer(t)
	if err := p.applyBinding(actPlayPause, 0, ""); err != nil {
		t.Fatal(err)
	}
	if err := p.applyBinding(actPlayPause, 0, "x"); err != nil {
		t.Fatalf("bind primary: %v", err)
	}
	if got := effectiveKeys(actPlayPause, p.overrides); !reflect.DeepEqual(got, []string{"x", "k"}) {
		t.Errorf("play-pause keys = %v, want [x k]", got)
	}
}

// Restoring both slots to their default keys collapses the override even when
// it went through an empty-primary state.
func TestApplyBindingRefillingDefaultCollapsesOverride(t *testing.T) {
	p, _ := prefsPlayer(t)
	if err := p.applyBinding(actPlayPause, 0, ""); err != nil {
		t.Fatal(err)
	}
	if err := p.applyBinding(actPlayPause, 0, "space"); err != nil {
		t.Fatal(err)
	}
	if _, ok := p.overrides[actPlayPause]; ok {
		t.Errorf("override should collapse to defaults, got %v", p.overrides[actPlayPause])
	}
}

func TestApplyBindingConflictRejected(t *testing.T) {
	p, saved := prefsPlayer(t)
	// "m" is mute's default; binding it to stop must fail and change nothing.
	if err := p.applyBinding(actStop, 0, "m"); err == nil {
		t.Fatal("expected conflict error")
	}
	if id := p.keymap[mustChord(t, "m")]; id != actMute {
		t.Errorf("keymap[m] = %q, want mute (unchanged)", id)
	}
	if *saved != nil {
		t.Errorf("save must not run on conflict, saved %v", *saved)
	}
}

func TestApplyBindingClearBothSlotsWritesNone(t *testing.T) {
	p, saved := prefsPlayer(t)
	if err := p.applyBinding(actMute, 0, ""); err != nil {
		t.Fatalf("clear slot 0: %v", err)
	}
	if got := effectiveKeys(actMute, p.overrides); got != nil {
		t.Errorf("mute keys = %v, want none", got)
	}
	if got := (*saved)["mute"]; !reflect.DeepEqual(got, []string{"none"}) {
		t.Errorf("saved mute = %v, want [none]", got)
	}
	if _, ok := p.keymap[mustChord(t, "m")]; ok {
		t.Error("keymap[m] should be unbound")
	}
}

func TestApplyBindingRestoringDefaultsDropsOverride(t *testing.T) {
	p, saved := prefsPlayer(t)
	if err := p.applyBinding(actMute, 0, "x"); err != nil {
		t.Fatal(err)
	}
	if err := p.applyBinding(actMute, 0, "m"); err != nil {
		t.Fatal(err)
	}
	if _, ok := p.overrides[actMute]; ok {
		t.Errorf("override should collapse when equal to defaults, got %v", p.overrides[actMute])
	}
	if _, ok := (*saved)["mute"]; ok {
		t.Errorf("saved map should not contain mute, got %v", *saved)
	}
	if id := p.keymap[mustChord(t, "m")]; id != actMute {
		t.Errorf("keymap[m] = %q, want mute (default restored)", id)
	}
}

func TestApplyBindingClearActionWithoutDefaults(t *testing.T) {
	p, _ := prefsPlayer(t)
	if err := p.applyBinding(actStop, 0, "s"); err != nil {
		t.Fatal(err)
	}
	if err := p.applyBinding(actStop, 0, ""); err != nil {
		t.Fatal(err)
	}
	// No defaults to suppress: the override is simply removed, not "none".
	if _, ok := p.overrides[actStop]; ok {
		t.Errorf("stop override should be removed, got %v", p.overrides[actStop])
	}
}

func mustChord(t *testing.T, s string) keyChord {
	t.Helper()
	c, err := parseChord(s)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestCaptureChordString(t *testing.T) {
	cases := []struct {
		name string
		key  glfw.Key
		mods glfw.ModifierKey
		want string
		ok   bool
	}{
		{"letter", glfw.KeyX, 0, "x", true},
		{"with ctrl", glfw.KeyX, glfw.ModControl, "ctrl+x", true},
		{"named key", glfw.KeyPageDown, 0, "pagedown", true},
		{"digit", glfw.Key5, 0, "5", true},
		{"function key", glfw.KeyF11, 0, "f11", true},
		{"shifted slash is question mark", glfw.KeySlash, glfw.ModShift, "?", true},
		{"caps lock stripped", glfw.KeyX, glfw.ModCapsLock, "x", true},
		{"unrepresentable key rejected", glfw.KeySemicolon, 0, "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := captureChordString(c.key, c.mods)
			if c.ok && (err != nil || got != c.want) {
				t.Errorf("= %q, %v; want %q", got, err, c.want)
			}
			if !c.ok && err == nil {
				t.Errorf("= %q, want error", got)
			}
		})
	}
}

func TestHandleCaptureKeyBindsAndDisarms(t *testing.T) {
	p, saved := prefsPlayer(t)
	p.capture = &prefsCapture{id: actMute, slot: 0}
	if !p.handleCaptureKey(glfw.KeyX, 0) {
		t.Fatal("capture should consume the key")
	}
	if p.capture != nil {
		t.Error("capture should disarm after a key")
	}
	if got := (*saved)["mute"]; len(got) != 1 || got[0] != "x" {
		t.Errorf("saved mute = %v, want [x]", got)
	}
}

func TestHandleCaptureKeyEscCancels(t *testing.T) {
	p, saved := prefsPlayer(t)
	p.capture = &prefsCapture{id: actMute, slot: 0}
	if !p.handleCaptureKey(glfw.KeyEscape, 0) {
		t.Fatal("esc during capture must be consumed")
	}
	if p.capture != nil {
		t.Error("capture should disarm on esc")
	}
	if *saved != nil {
		t.Errorf("esc must not save, saved %v", *saved)
	}
}

// Backspace and Delete are ordinary bindable keys: clearing a slot is the
// row's clear button, so nothing has to reserve them.
func TestHandleCaptureKeyBindsDeleteAndBackspace(t *testing.T) {
	cases := []struct {
		name string
		key  glfw.Key
		mods glfw.ModifierKey
		want string
	}{
		{"backspace", glfw.KeyBackspace, 0, "backspace"},
		{"delete", glfw.KeyDelete, 0, "delete"},
		{"ctrl+delete", glfw.KeyDelete, glfw.ModControl, "ctrl+delete"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p, _ := prefsPlayer(t)
			p.capture = &prefsCapture{id: actMute, slot: 0}
			if !p.handleCaptureKey(c.key, c.mods) {
				t.Fatal("capture must consume the key")
			}
			if got := effectiveKeys(actMute, p.overrides); !reflect.DeepEqual(got, []string{c.want}) {
				t.Errorf("mute keys = %v, want [%s] (msg %q)", got, c.want, p.prefsMsg)
			}
		})
	}
}

func TestClearKind(t *testing.T) {
	cases := []struct {
		name      string
		id        actionID
		slot      int
		keys      []string
		overrides map[actionID][]string
		want      clearAction
	}{
		{"bound slot clears", actMute, 0, []string{"m"}, nil, clearBinding},
		{"empty slot at defaults is inert", actMute, 1, []string{"m"}, nil, clearNoop},
		{"empty slot when overridden restores", actMute, 1, []string{"x"},
			map[actionID][]string{actMute: {"x"}}, clearRestore},
		{"unbound override restores", actMute, 0, nil,
			map[actionID][]string{actMute: {"none"}}, clearRestore},
		{"action without defaults is inert", actStop, 0, nil,
			map[actionID][]string{actStop: {"none"}}, clearNoop},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := clearKind(c.id, c.slot, c.keys, c.overrides); got != c.want {
				t.Errorf("clearKind(%s, %d) = %v, want %v", c.id, c.slot, got, c.want)
			}
		})
	}
}

func TestRestoreDefaultsDropsOverride(t *testing.T) {
	p, saved := prefsPlayer(t)
	if err := p.applyBinding(actMute, 0, "x"); err != nil {
		t.Fatal(err)
	}
	if err := p.restoreDefaults(actMute); err != nil {
		t.Fatalf("restoreDefaults: %v", err)
	}
	if got := effectiveKeys(actMute, p.overrides); !reflect.DeepEqual(got, []string{"m"}) {
		t.Errorf("mute keys = %v, want [m]", got)
	}
	if id := p.keymap[mustChord(t, "m")]; id != actMute {
		t.Errorf("keymap[m] = %q, want mute", id)
	}
	if _, ok := p.keymap[mustChord(t, "x")]; ok {
		t.Error("keymap[x] should be unbound after restore")
	}
	if _, ok := (*saved)["mute"]; ok {
		t.Errorf("saved map should no longer contain mute, got %v", *saved)
	}
}

// Restoring one action's defaults must not resurrect a key another action has
// taken meanwhile; the whole keymap is revalidated first.
func TestRestoreDefaultsConflictRejected(t *testing.T) {
	p, _ := prefsPlayer(t)
	if err := p.applyBinding(actMute, 0, "x"); err != nil {
		t.Fatal(err)
	}
	if err := p.applyBinding(actStop, 0, "m"); err != nil {
		t.Fatal(err)
	}
	if err := p.restoreDefaults(actMute); err == nil {
		t.Fatal("expected conflict error restoring mute's default over stop")
	}
	if got := effectiveKeys(actMute, p.overrides); !reflect.DeepEqual(got, []string{"x"}) {
		t.Errorf("mute keys = %v, want [x] (unchanged)", got)
	}
	if id := p.keymap[mustChord(t, "m")]; id != actStop {
		t.Errorf("keymap[m] = %q, want stop (unchanged)", id)
	}
}

func TestRestoreDefaultsNoOverrideIsNoop(t *testing.T) {
	p, saved := prefsPlayer(t)
	if err := p.restoreDefaults(actMute); err != nil {
		t.Fatalf("restoreDefaults: %v", err)
	}
	if *saved != nil {
		t.Errorf("nothing to restore must not save, got %v", *saved)
	}
}

func TestHandleCaptureKeyIgnoresLoneModifier(t *testing.T) {
	p, _ := prefsPlayer(t)
	p.capture = &prefsCapture{id: actMute, slot: 0}
	if !p.handleCaptureKey(glfw.KeyLeftControl, glfw.ModControl) {
		t.Fatal("modifier press must be consumed while capturing")
	}
	if p.capture == nil {
		t.Error("capture must stay armed after a lone modifier")
	}
}

func TestHandleCaptureKeyConflictShowsMessage(t *testing.T) {
	p, _ := prefsPlayer(t)
	p.capture = &prefsCapture{id: actStop, slot: 0}
	p.handleCaptureKey(glfw.KeyM, 0) // m = mute's default
	if p.prefsMsg == "" {
		t.Error("conflict should set the message line")
	}
	if id := p.keymap[mustChord(t, "m")]; id != actMute {
		t.Errorf("keymap[m] = %q, want mute (unchanged)", id)
	}
}

func TestHandleCaptureKeyNotCapturing(t *testing.T) {
	p, _ := prefsPlayer(t)
	if p.handleCaptureKey(glfw.KeyX, 0) {
		t.Error("must not consume keys when no capture is armed")
	}
}

func TestTogglePrefsRequestsAndRevokesWindow(t *testing.T) {
	p, _ := prefsPlayer(t)
	p.togglePrefs()
	if !p.prefsWanted {
		t.Fatal("togglePrefs should request the preferences window")
	}
	if len(p.prefsRows) == 0 {
		t.Error("rows should be built when prefs opens")
	}
	p.togglePrefs()
	if p.prefsWanted {
		t.Error("second togglePrefs should revoke the window")
	}
}

// The preferences window is separate from the video overlays, so opening it
// must leave whatever overlay is on the video alone.
func TestOpenPrefsLeavesVideoOverlayAlone(t *testing.T) {
	p, _ := prefsPlayer(t)
	p.overlay = overlayInfo
	p.openPrefs()
	if p.overlay != overlayInfo {
		t.Errorf("overlay = %v, want overlayInfo (unchanged)", p.overlay)
	}
}

func TestClosePrefsDropsCapture(t *testing.T) {
	p, _ := prefsPlayer(t)
	p.openPrefs()
	p.capture = &prefsCapture{id: actMute, slot: 0}
	p.closePrefs()
	if p.prefsWanted {
		t.Error("closePrefs should revoke the window")
	}
	if p.capture != nil {
		t.Error("closePrefs should drop a pending capture")
	}
}

func TestPrefsWindowAction(t *testing.T) {
	cases := []struct {
		name       string
		want, have bool
		action     prefsAction
	}{
		{"requested but missing", true, false, prefsCreate},
		{"revoked but alive", false, true, prefsDestroy},
		{"open and wanted", true, true, prefsNoop},
		{"closed and unwanted", false, false, prefsNoop},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := prefsWindowAction(c.want, c.have); got != c.action {
				t.Errorf("prefsWindowAction(%v, %v) = %v, want %v", c.want, c.have, got, c.action)
			}
		})
	}
}

func TestHandlePrefsKeyEscClosesWindow(t *testing.T) {
	p, _ := prefsPlayer(t)
	p.openPrefs()
	p.handlePrefsKey(glfw.KeyEscape, 0)
	if p.prefsWanted {
		t.Error("esc with no capture armed should close the preferences window")
	}
}

func TestHandlePrefsKeyEscCancelsCaptureOnly(t *testing.T) {
	p, _ := prefsPlayer(t)
	p.openPrefs()
	p.capture = &prefsCapture{id: actMute, slot: 0}
	p.handlePrefsKey(glfw.KeyEscape, 0)
	if p.capture != nil {
		t.Error("esc should cancel the armed capture")
	}
	if !p.prefsWanted {
		t.Error("esc that cancelled a capture must not also close the window")
	}
}

func TestHandlePrefsKeyBindsWhileCapturing(t *testing.T) {
	p, saved := prefsPlayer(t)
	p.openPrefs()
	p.capture = &prefsCapture{id: actMute, slot: 0}
	p.handlePrefsKey(glfw.KeyX, 0)
	if got := (*saved)["mute"]; len(got) != 1 || got[0] != "x" {
		t.Errorf("saved mute = %v, want [x]", got)
	}
	if !p.prefsWanted {
		t.Error("binding a key must not close the window")
	}
}

// Mouse wheel deltas must become Gio scroll distances so the row list can be
// scrolled: GLFW reports +1 per notch upward, Gio expects pixels downward.
func TestScrollDistance(t *testing.T) {
	cases := []struct {
		name       string
		xoff, yoff float64
		wantX      float32
		wantY      float32
	}{
		{"wheel up scrolls back", 0, 1, 0, -prefsScrollScale},
		{"wheel down scrolls forward", 0, -1, 0, prefsScrollScale},
		{"horizontal wheel", 1, 0, -prefsScrollScale, 0},
		{"high resolution delta scales", 0, -0.5, 0, prefsScrollScale / 2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := scrollDistance(c.xoff, c.yoff)
			if got.X != c.wantX || got.Y != c.wantY {
				t.Errorf("scrollDistance(%v, %v) = %v, want (%v, %v)", c.xoff, c.yoff, got, c.wantX, c.wantY)
			}
		})
	}
}

// Player shortcuts are not dispatched from the preferences window: a key that
// is bound to an action does nothing there unless a slot is capturing.
func TestHandlePrefsKeyIgnoresShortcutsWhenIdle(t *testing.T) {
	p, saved := prefsPlayer(t)
	p.openPrefs()
	p.handlePrefsKey(glfw.KeyM, 0) // mute's default binding
	if p.muted {
		t.Error("preferences window must not dispatch player shortcuts")
	}
	if *saved != nil {
		t.Errorf("no capture armed, nothing should be saved, got %v", *saved)
	}
}
