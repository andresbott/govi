package player

import (
	"fmt"
	"strings"

	"github.com/go-gl/glfw/v3.3/glfw"
)

// keyChord is a physical key plus modifier mask, the unit the GLFW key
// callback is matched against.
type keyChord struct {
	key  glfw.Key
	mods glfw.ModifierKey
}

// namedKeys maps lower-case config key names to GLFW keys. Printable
// punctuation that GLFW reports as a base key (e.g. "?" is Shift+"/") is
// handled by shiftedKeys below.
var namedKeys = map[string]glfw.Key{
	"space": glfw.KeySpace, "esc": glfw.KeyEscape, "enter": glfw.KeyEnter,
	"tab": glfw.KeyTab, "up": glfw.KeyUp, "down": glfw.KeyDown,
	"left": glfw.KeyLeft, "right": glfw.KeyRight,
	"pageup": glfw.KeyPageUp, "pagedown": glfw.KeyPageDown,
	"backspace": glfw.KeyBackspace, "delete": glfw.KeyDelete,
	"plus": glfw.KeyEqual, "-": glfw.KeyMinus,
}

// shiftedKeys maps punctuation typed with Shift to its base GLFW key and the
// implied Shift modifier.
var shiftedKeys = map[string]keyChord{
	"?": {glfw.KeySlash, glfw.ModShift},
}

// isNoneKey reports whether a config key string is the "empty slot" marker.
// It is a slot placeholder rather than a whole-action marker so that clearing
// the first of two bindings does not make the second one hop up into the
// first slot in the preferences window.
func isNoneKey(s string) bool {
	return strings.EqualFold(s, "none")
}

// parseChord translates a config key string ("ctrl+up", "k", "?") into a
// keyChord. Names are case-insensitive; modifiers ctrl/alt/shift/super may
// prefix in any order joined by "+".
func parseChord(s string) (keyChord, error) {
	if s == "" {
		return keyChord{}, fmt.Errorf("empty key string")
	}
	// A trailing "+" is the plus key itself ("+", "ctrl++"); splitting on "+"
	// would otherwise leave an empty last segment. Swap it for a placeholder
	// that survives the split. A lone trailing "+" after a modifier ("ctrl+")
	// stays as-is and errors below as a missing key.
	if s == "+" || strings.HasSuffix(s, "++") {
		s = s[:len(s)-1] + "plus"
	}
	parts := strings.Split(s, "+")
	keyPart := parts[len(parts)-1]
	mods, err := parseMods(parts[:len(parts)-1], s)
	if err != nil {
		return keyChord{}, err
	}
	if keyPart == "" {
		return keyChord{}, fmt.Errorf("missing key in %q", s)
	}

	// Shifted punctuation matches case-sensitively ("?" is not lower-cased).
	if sc, ok := shiftedKeys[keyPart]; ok {
		sc.mods |= mods
		return sc, nil
	}
	key, ok := parseKeyName(strings.ToLower(keyPart))
	if !ok {
		return keyChord{}, fmt.Errorf("unknown key %q in %q", keyPart, s)
	}
	return keyChord{key, mods}, nil
}

// parseMods folds the modifier segments of a chord ("ctrl", "alt", "shift",
// "super", case-insensitive) into a mask. full is the original chord string,
// used only for error messages.
func parseMods(parts []string, full string) (glfw.ModifierKey, error) {
	var mods glfw.ModifierKey
	for _, m := range parts {
		switch strings.ToLower(m) {
		case "ctrl":
			mods |= glfw.ModControl
		case "alt":
			mods |= glfw.ModAlt
		case "shift":
			mods |= glfw.ModShift
		case "super":
			mods |= glfw.ModSuper
		default:
			return 0, fmt.Errorf("unknown modifier %q in %q", m, full)
		}
	}
	return mods, nil
}

// parseKeyName resolves an already lower-cased key name to a GLFW key: a named
// key, a single letter a-z, a single digit 0-9, or a function key f1-f12.
func parseKeyName(kp string) (glfw.Key, bool) {
	if k, ok := namedKeys[kp]; ok {
		return k, true
	}
	if len(kp) == 1 {
		switch c := kp[0]; {
		case c >= 'a' && c <= 'z':
			return glfw.KeyA + glfw.Key(c-'a'), true
		case c >= '0' && c <= '9':
			return glfw.Key0 + glfw.Key(c-'0'), true
		}
		return 0, false
	}
	if strings.HasPrefix(kp, "f") {
		var n int
		if _, err := fmt.Sscanf(kp, "f%d", &n); err == nil && n >= 1 && n <= 12 {
			return glfw.KeyF1 + glfw.Key(n-1), true
		}
	}
	return 0, false
}

// buildKeymap merges per-action key overrides onto the built-in defaults and
// returns the effective chord→action table. Merging is wholesale per action:
// an action present in overrides replaces both default slots. The special
// value "none" is an empty slot, so ["none"] unbinds an action outright and
// ["none", "x"] leaves the first slot empty while keeping "x" in the second.
// Errors on unparseable keys or a chord bound to two actions, naming the
// offending entry.
func buildKeymap(overrides map[actionID][]string) (map[keyChord]actionID, error) {
	effective := actionDefaults()
	for id, keys := range overrides {
		effective[id] = keys
	}

	km := make(map[keyChord]actionID)
	for id, keys := range effective {
		for _, ks := range keys {
			if isNoneKey(ks) {
				continue
			}
			chord, err := parseChord(ks)
			if err != nil {
				return nil, fmt.Errorf("shortcut %q: %w", id, err)
			}
			if other, dup := km[chord]; dup {
				return nil, fmt.Errorf("key %q bound to both %q and %q", ks, other, id)
			}
			km[chord] = id
		}
	}
	return km, nil
}
