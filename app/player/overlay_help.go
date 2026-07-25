package player

import (
	"fmt"
	"sort"
	"strings"

	"gioui.org/layout"
	"github.com/go-gl/glfw/v3.3/glfw"
)

// helpRow is one line in the help overlay.
type helpRow struct {
	label string
	keys  string // "space [k]" or "menu"
}

// chordNames is the reverse of the parser's key tables, for display.
var chordNames = map[glfw.Key]string{
	glfw.KeySpace: "space", glfw.KeyEscape: "esc", glfw.KeyEnter: "enter",
	glfw.KeyTab: "tab", glfw.KeyUp: "up", glfw.KeyDown: "down",
	glfw.KeyLeft: "left", glfw.KeyRight: "right",
	glfw.KeyPageUp: "pageup", glfw.KeyPageDown: "pagedown",
	glfw.KeyBackspace: "backspace", glfw.KeyDelete: "delete",
	glfw.KeyEqual: "+", glfw.KeyMinus: "-",
}

// chordLabel renders a chord the way a user would type it in config.
func chordLabel(c keyChord) string {
	var mods []string
	if c.mods&glfw.ModControl != 0 {
		mods = append(mods, "ctrl")
	}
	if c.mods&glfw.ModAlt != 0 {
		mods = append(mods, "alt")
	}
	if c.mods&glfw.ModSuper != 0 {
		mods = append(mods, "super")
	}
	// Shift is implicit for shifted punctuation ("?"), explicit otherwise.
	base := keyName(c.key)
	if c.key == glfw.KeySlash && c.mods&glfw.ModShift != 0 {
		base = "?"
	} else if c.mods&glfw.ModShift != 0 {
		mods = append(mods, "shift")
	}
	if len(mods) == 0 {
		return base
	}
	return strings.Join(mods, "+") + "+" + base
}

func keyName(k glfw.Key) string {
	if n, ok := chordNames[k]; ok {
		return n
	}
	if k >= glfw.KeyA && k <= glfw.KeyZ {
		return string(rune('a' + (k - glfw.KeyA)))
	}
	if k >= glfw.Key0 && k <= glfw.Key9 {
		return string(rune('0' + (k - glfw.Key0)))
	}
	if k >= glfw.KeyF1 && k <= glfw.KeyF12 {
		return fmt.Sprintf("f%d", int(k-glfw.KeyF1)+1)
	}
	return "?"
}

// helpRows produces one row per action in registry order, with its effective
// bindings ("main [secondary]"). A keyless action shows "menu" when the
// context menu offers it, "unbound" otherwise.
func helpRows(actions []action, km map[keyChord]actionID) []helpRow {
	// Reverse the keymap: action -> its chords.
	byAction := map[actionID][]keyChord{}
	for chord, id := range km {
		byAction[id] = append(byAction[id], chord)
	}
	// Deterministic order within an action.
	for id := range byAction {
		cs := byAction[id]
		sort.Slice(cs, func(i, j int) bool {
			if cs[i].key != cs[j].key {
				return cs[i].key < cs[j].key
			}
			return cs[i].mods < cs[j].mods
		})
		byAction[id] = cs
	}

	rows := make([]helpRow, 0, len(actions))
	for _, a := range actions {
		chords := byAction[a.id]
		if len(chords) == 0 {
			marker := "unbound"
			if a.inMenu {
				marker = "menu"
			}
			rows = append(rows, helpRow{label: a.label, keys: marker})
			continue
		}
		labels := make([]string, len(chords))
		for i, c := range chords {
			labels[i] = chordLabel(c)
		}
		keys := labels[0]
		if len(labels) > 1 {
			keys += " [" + strings.Join(labels[1:], ", ") + "]"
		}
		rows = append(rows, helpRow{label: a.label, keys: keys})
	}
	return rows
}

// layoutHelp draws the centered help overlay.
func (p *Player) layoutHelp(gtx layout.Context) layout.Dimensions {
	hr := helpRows(defaultActions(), p.keymap)
	rows := make([][2]string, 0, len(hr)+1)
	for _, r := range hr {
		rows = append(rows, [2]string{r.label, r.keys})
	}
	rows = append(rows, [2]string{"", "Esc closes"})
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return p.drawPanel(gtx, func(gtx layout.Context) layout.Dimensions {
			return p.rowGrid(gtx, rows)
		})
	})
}
