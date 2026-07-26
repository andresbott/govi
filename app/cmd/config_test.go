package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTemp(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadConfigMissingFileUsesDefaults(t *testing.T) {
	cfg, err := loadConfig(filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	if err != nil {
		t.Fatalf("missing file should not error: %v", err)
	}
	pc := cfg.toPlayerConfig()
	// A missing file yields no explicit shortcut overrides at all; the player
	// falls back to built-in defaults, so the map is empty.
	if len(pc.Shortcuts) != 0 {
		t.Errorf("expected no overrides for missing file, got %v", pc.Shortcuts)
	}
}

func TestLoadConfigPartialOverridePreservesOthers(t *testing.T) {
	path := writeTemp(t, "shortcuts:\n  play-pause: [\"p\"]\n")
	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	pc := cfg.toPlayerConfig()
	if got := pc.Shortcuts["play-pause"]; len(got) != 1 || got[0] != "p" {
		t.Errorf("play-pause override = %v, want [p]", got)
	}
	// mute was not mentioned: it must NOT appear in overrides (player applies
	// its default).
	if _, ok := pc.Shortcuts["mute"]; ok {
		t.Errorf("mute should not be in overrides, got %v", pc.Shortcuts["mute"])
	}
}

func TestLoadConfigPlaceholderImage(t *testing.T) {
	path := writeTemp(t, "placeholderImage: /tmp/idle.png\n")
	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if cfg.toPlayerConfig().PlaceholderImage != "/tmp/idle.png" {
		t.Errorf("placeholder = %q", cfg.PlaceholderImage)
	}
}

func TestLoadConfigUnknownActionFails(t *testing.T) {
	path := writeTemp(t, "shortcuts:\n  fly-to-moon: [\"x\"]\n")
	_, err := loadConfig(path)
	if err == nil {
		t.Fatal("expected error for unknown action name")
	}
}

func TestLoadConfigBadYamlFails(t *testing.T) {
	path := writeTemp(t, "shortcuts: [this is: not valid\n")
	if _, err := loadConfig(path); err == nil {
		t.Fatal("expected error for malformed yaml")
	}
}

func TestDefaultConfigPathEndsWithGoviConfig(t *testing.T) {
	p, err := defaultConfigPath()
	if err != nil {
		t.Fatalf("defaultConfigPath: %v", err)
	}
	if filepath.Base(p) != "config.yaml" || filepath.Base(filepath.Dir(p)) != "govi" {
		t.Errorf("unexpected default path %q", p)
	}
}

func TestLoadConfigSupportsAllRegistryActions(t *testing.T) {
	path := writeTemp(t, `shortcuts:
  seek-forward: ["l"]
  seek-back: ["j"]
  seek-forward-percent: ["shift+l"]
  seek-back-percent: ["shift+j"]
  next-video: ["n"]
  previous-video: ["b"]
  move-to-trash: ["t"]
  delete-file: ["shift+d"]
  preferences: ["p"]
  progress: ["o"]
`)
	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	pc := cfg.toPlayerConfig()
	want := map[string]string{
		"seek-forward": "l", "seek-back": "j",
		"seek-forward-percent": "shift+l", "seek-back-percent": "shift+j",
		"next-video":     "n",
		"previous-video": "b", "move-to-trash": "t", "delete-file": "shift+d",
		"preferences": "p", "progress": "o",
	}
	for id, key := range want {
		got := pc.Shortcuts[id]
		if len(got) != 1 || got[0] != key {
			t.Errorf("shortcut %q = %v, want [%s]", id, got, key)
		}
	}
}

// Every action the config accepts must also survive the trip into
// player.Config: a new ShortcutsCfg field with no matching add() call in
// toPlayerConfig would silently drop the user's override.
func TestEveryKnownActionReachesPlayerConfig(t *testing.T) {
	for id := range knownActions {
		path := writeTemp(t, "shortcuts:\n  "+id+": [\"z\"]\n")
		cfg, err := loadConfig(path)
		if err != nil {
			t.Errorf("loadConfig with only %q set: %v", id, err)
			continue
		}
		if got := cfg.toPlayerConfig().Shortcuts[id]; len(got) != 1 || got[0] != "z" {
			t.Errorf("shortcut %q = %v, want [z] — missing from toPlayerConfig?", id, got)
		}
	}
}

func TestSaveShortcutsRoundTrip(t *testing.T) {
	path := writeTemp(t, "placeholderImage: /tmp/idle.png\nshortcuts:\n  mute: [\"x\"]\n")
	sc := map[string][]string{"play-pause": {"p"}, "quit": {"none"}}
	if err := saveShortcuts(path, sc); err != nil {
		t.Fatalf("saveShortcuts: %v", err)
	}
	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatalf("loadConfig after save: %v", err)
	}
	pc := cfg.toPlayerConfig()
	if got := pc.Shortcuts["play-pause"]; len(got) != 1 || got[0] != "p" {
		t.Errorf("play-pause = %v, want [p]", got)
	}
	if got := pc.Shortcuts["quit"]; len(got) != 1 || got[0] != "none" {
		t.Errorf("quit = %v, want [none]", got)
	}
	// The whole shortcuts section is replaced: the old mute override is gone.
	if _, ok := pc.Shortcuts["mute"]; ok {
		t.Errorf("mute should have been replaced, got %v", pc.Shortcuts["mute"])
	}
	// Other top-level keys survive.
	if pc.PlaceholderImage != "/tmp/idle.png" {
		t.Errorf("placeholderImage lost on save: %q", pc.PlaceholderImage)
	}
}

func TestSaveShortcutsCreatesMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "config.yaml")
	if err := saveShortcuts(path, map[string][]string{"mute": {"x"}}); err != nil {
		t.Fatalf("saveShortcuts on missing file: %v", err)
	}
	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if got := cfg.toPlayerConfig().Shortcuts["mute"]; len(got) != 1 || got[0] != "x" {
		t.Errorf("mute = %v, want [x]", got)
	}
}

func TestSaveShortcutsEmptyMapRemovesSection(t *testing.T) {
	path := writeTemp(t, "shortcuts:\n  mute: [\"x\"]\n")
	if err := saveShortcuts(path, map[string][]string{}); err != nil {
		t.Fatalf("saveShortcuts: %v", err)
	}
	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if n := len(cfg.toPlayerConfig().Shortcuts); n != 0 {
		t.Errorf("expected no shortcuts, got %v", cfg.toPlayerConfig().Shortcuts)
	}
}

func TestLoadConfigAudioState(t *testing.T) {
	path := writeTemp(t, "volume: 45\nmuted: true\n")
	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	pc := cfg.toPlayerConfig()
	if pc.Volume == nil {
		t.Fatal("Volume is nil, want the configured 45")
	}
	if *pc.Volume != 45 {
		t.Errorf("Volume = %v, want 45", *pc.Volume)
	}
	if !pc.Muted {
		t.Error("Muted = false, want true")
	}
}

// An absent volume must be distinguishable from a configured 0: the player
// applies mpv's own default in the first case and silence in the second, so a
// plain int would silently mute a first run.
func TestLoadConfigAbsentVolumeIsUnset(t *testing.T) {
	path := writeTemp(t, "placeholderImage: /tmp/idle.png\n")
	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if got := cfg.toPlayerConfig().Volume; got != nil {
		t.Errorf("Volume = %v for a config with no volume key, want nil", *got)
	}
}

func TestLoadConfigExplicitZeroVolumeIsSet(t *testing.T) {
	path := writeTemp(t, "volume: 0\n")
	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	got := cfg.toPlayerConfig().Volume
	if got == nil {
		t.Fatal("Volume is nil for an explicit `volume: 0`, want a set 0")
	}
	if *got != 0 {
		t.Errorf("Volume = %v, want 0", *got)
	}
}

func TestSaveAudioStateRoundTrip(t *testing.T) {
	path := writeTemp(t, "placeholderImage: /tmp/idle.png\nshortcuts:\n  mute: [\"x\"]\n")

	if err := saveAudioState(path, 45, true); err != nil {
		t.Fatalf("saveAudioState: %v", err)
	}

	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatalf("loadConfig after save: %v", err)
	}
	pc := cfg.toPlayerConfig()
	if pc.Volume == nil || *pc.Volume != 45 {
		t.Errorf("Volume = %v, want 45", pc.Volume)
	}
	if !pc.Muted {
		t.Error("Muted = false, want true")
	}
	// Everything else in the file survives, the way saveShortcuts preserves it.
	if pc.PlaceholderImage != "/tmp/idle.png" {
		t.Errorf("placeholderImage lost on save: %q", pc.PlaceholderImage)
	}
	if got := pc.Shortcuts["mute"]; len(got) != 1 || got[0] != "x" {
		t.Errorf("mute shortcut = %v, want [x] — saving the volume clobbered the shortcuts", got)
	}
}

func TestSaveAudioStateCreatesMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "config.yaml")

	if err := saveAudioState(path, 70, false); err != nil {
		t.Fatalf("saveAudioState on missing file: %v", err)
	}

	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if got := cfg.toPlayerConfig().Volume; got == nil || *got != 70 {
		t.Errorf("Volume = %v, want 70", got)
	}
}

// The two savers write to the same file, so neither may drop what the other
// stored: a volume saved on exit must survive a later shortcut rebind, and vice
// versa.
func TestSaveShortcutsPreservesTheSavedVolume(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := saveAudioState(path, 45, true); err != nil {
		t.Fatalf("saveAudioState: %v", err)
	}

	if err := saveShortcuts(path, map[string][]string{"play-pause": {"p"}}); err != nil {
		t.Fatalf("saveShortcuts: %v", err)
	}

	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	pc := cfg.toPlayerConfig()
	if pc.Volume == nil || *pc.Volume != 45 {
		t.Errorf("Volume = %v after a shortcut save, want 45", pc.Volume)
	}
	if !pc.Muted {
		t.Error("Muted = false after a shortcut save, want true")
	}
}

func TestSaveAudioStatePreservesSavedShortcuts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := saveShortcuts(path, map[string][]string{"play-pause": {"p"}}); err != nil {
		t.Fatalf("saveShortcuts: %v", err)
	}

	if err := saveAudioState(path, 45, false); err != nil {
		t.Fatalf("saveAudioState: %v", err)
	}

	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if got := cfg.toPlayerConfig().Shortcuts["play-pause"]; len(got) != 1 || got[0] != "p" {
		t.Errorf("play-pause = %v after an audio save, want [p]", got)
	}
}

// A saved level must survive a round trip through the file even at the extremes,
// including 0 — the value the unset sentinel has to stay distinct from.
func TestSaveAudioStateRoundTripsEveryLevel(t *testing.T) {
	for _, vol := range []int{0, 1, 45, 100} {
		path := filepath.Join(t.TempDir(), "config.yaml")
		if err := saveAudioState(path, vol, false); err != nil {
			t.Fatalf("saveAudioState(%d): %v", vol, err)
		}
		cfg, err := loadConfig(path)
		if err != nil {
			t.Fatalf("loadConfig: %v", err)
		}
		got := cfg.toPlayerConfig().Volume
		if got == nil {
			t.Errorf("volume %d came back unset", vol)
			continue
		}
		if *got != vol {
			t.Errorf("volume = %v, want %v", *got, vol)
		}
	}
}

// The whole loop, over one file: what the player saves on exit is what the next
// launch feeds back to mpv. This is the test that fails if either half is wired to
// a different key than the other.
func TestSavedAudioStateSurvivesARestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")

	// First run: the player's SaveAudioState callback fires as root.go builds it.
	if err := saveAudioState(path, 45, true); err != nil {
		t.Fatalf("saveAudioState: %v", err)
	}

	// Second run: loadConfig -> toPlayerConfig is what player.Run receives.
	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	pc := cfg.toPlayerConfig()
	if pc.Volume == nil || *pc.Volume != 45 {
		t.Errorf("second run starts at %v, want the saved 45", pc.Volume)
	}
	if !pc.Muted {
		t.Error("second run starts unmuted, want the saved muted state")
	}
}

// `volume` and `muted` are top-level keys, so validateShortcutNames must not
// mistake them for action ids.
func TestLoadConfigAudioKeysAreNotShortcuts(t *testing.T) {
	path := writeTemp(t, "volume: 45\nmuted: true\nshortcuts:\n  mute: [\"x\"]\n")
	if _, err := loadConfig(path); err != nil {
		t.Fatalf("loadConfig with audio state alongside shortcuts: %v", err)
	}
}
