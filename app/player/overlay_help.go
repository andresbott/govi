package player

import (
	"fmt"
	"image/color"
	"sort"
	"strings"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget/material"
	"github.com/go-gl/glfw/v3.3/glfw"
)

// helpRow is one line in the help overlay: an action and its two binding
// slots, kept apart so the overlay can column them the way the preferences
// window does. primary carries the "menu"/"unbound" marker for a keyless
// action, since that is where the eye looks for a binding.
type helpRow struct {
	label     string
	primary   string // "space", or "menu" / "unbound" when nothing is bound
	secondary string // "k", or "" — extra chords beyond two are joined here
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
// bindings split into the primary and secondary slots. A keyless action shows
// "menu" as its primary when the context menu offers it, "unbound" otherwise.
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
			rows = append(rows, helpRow{label: a.label, primary: marker})
			continue
		}
		labels := make([]string, len(chords))
		for i, c := range chords {
			labels[i] = chordLabel(c)
		}
		// The UI offers two slots per action, but a hand-written config may
		// bind more; the extras join the secondary column rather than vanish.
		rows = append(rows, helpRow{
			label:     a.label,
			primary:   labels[0],
			secondary: strings.Join(labels[1:], ", "),
		})
	}
	return rows
}

// Help panel geometry. Fixed widths, so the key columns line up down and across
// both halves of the panel; each carries slack over the widest string it can
// hold, since anything wider wraps or clips silently (pinned by
// TestHelpCellsFitTheirWidestText). The label column needs the most room — at
// present "Show / Hide Progress" at ~136 dp — and the key columns only ever hold
// one chord, the widest nameable being "backspace" at ~67 dp.
const (
	helpLabelW    = unit.Dp(160)
	helpKeyW      = unit.Dp(80) // primary binding
	helpAltKeyW   = unit.Dp(80) // secondary binding
	helpColumnGap = unit.Dp(24) // between the two halves of the panel
)

// helpAltColor dims the secondary binding so the primary one reads first,
// without making the alternative look unbound (contrast panelLabelColor).
var helpAltColor = color.NRGBA{R: 0xcc, G: 0xcc, B: 0xcc, A: 0xff}

// splitHelpRows deals the rows into two columns, filling the left one first.
// The registry order is preserved down each column (the left column is the
// first half, not every other row), so the reading order stays the order the
// actions are declared in. An odd count leaves the extra row on the left.
func splitHelpRows(rows []helpRow) ([]helpRow, []helpRow) {
	half := (len(rows) + 1) / 2
	return rows[:half], rows[half:]
}

// layoutHelp draws the centered help overlay: the registry in two side-by-side
// columns, so the panel is half as tall as a single list and fits a modest
// window with 19 actions in it.
func (p *Player) layoutHelp(gtx layout.Context) layout.Dimensions {
	left, right := splitHelpRows(helpRows(defaultActions(), p.keymap))
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return p.drawPanel(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return p.layoutHelpColumn(gtx, left)
						}),
						layout.Rigid(layout.Spacer{Width: helpColumnGap}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return p.layoutHelpColumn(gtx, right)
						}),
					)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					lbl := material.Body2(p.theme, "Esc closes")
					lbl.Color = panelLabelColor
					return lbl.Layout(gtx)
				}),
			)
		})
	})
}

// layoutHelpColumn draws one column: a heading naming the two binding slots,
// then a line per action. Both columns carry the heading — a single one over
// the left column would read as applying to the whole panel.
func (p *Player) layoutHelpColumn(gtx layout.Context, rows []helpRow) layout.Dimensions {
	children := make([]layout.FlexChild, 0, len(rows)+1)
	children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return p.layoutHelpLine(gtx, helpRow{primary: "primary", secondary: "secondary"},
			panelLabelColor, panelLabelColor)
	}))
	for _, r := range rows {
		r := r
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return p.layoutHelpLine(gtx, r, panelValueColor, helpAltColor)
		}))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

// layoutHelpLine draws one action line as three fixed-width cells, so the key
// columns align across every row of a column (and across both columns, since
// they share the widths).
func (p *Player) layoutHelpLine(gtx layout.Context, r helpRow, primaryColor, secondaryColor color.NRGBA) layout.Dimensions {
	cell := func(text string, w unit.Dp, col color.NRGBA) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = gtx.Dp(w)
			gtx.Constraints.Max.X = gtx.Dp(w)
			lbl := material.Body2(p.theme, text)
			lbl.Color = col
			return lbl.Layout(gtx)
		})
	}
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		cell(r.label, helpLabelW, panelLabelColor),
		cell(r.primary, helpKeyW, primaryColor),
		cell(r.secondary, helpAltKeyW, secondaryColor),
	)
}
