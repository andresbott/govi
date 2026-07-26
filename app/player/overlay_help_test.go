package player

import (
	"image"
	"strings"
	"testing"
	"time"

	"gioui.org/font/gofont"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget/material"

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

// helpRowsByLabel indexes the generated rows for the assertions below.
func helpRowsByLabel(t *testing.T, overrides map[actionID][]string) map[string]helpRow {
	t.Helper()
	km, err := buildKeymap(overrides)
	if err != nil {
		t.Fatal(err)
	}
	byLabel := map[string]helpRow{}
	for _, r := range helpRows(defaultActions(), km) {
		byLabel[r.label] = r
	}
	return byLabel
}

func TestHelpRowsMenuOnlyMarked(t *testing.T) {
	byLabel := helpRowsByLabel(t, nil)
	if got := byLabel["Play / Pause"].primary; got != "space" {
		t.Errorf("Play / Pause primary = %q, want \"space\"", got)
	}
	// stop has no default binding -> "menu", in the primary column.
	stop := byLabel["Stop"]
	if stop.primary != "menu" {
		t.Errorf("Stop primary = %q, want \"menu\"", stop.primary)
	}
	if stop.secondary != "" {
		t.Errorf("Stop secondary = %q, want empty", stop.secondary)
	}
}

// The two default bindings of an action land in separate columns, so the help
// overlay can label them "primary" and "secondary".
func TestHelpRowsSplitPrimaryAndSecondary(t *testing.T) {
	pp := helpRowsByLabel(t, nil)["Play / Pause"]
	if pp.primary != "space" || pp.secondary != "k" {
		t.Errorf("Play / Pause = {%q, %q}, want {\"space\", \"k\"}", pp.primary, pp.secondary)
	}
}

// An action bound to one key has an empty secondary column rather than a
// repeat of the primary.
func TestHelpRowsSingleBindingLeavesSecondaryEmpty(t *testing.T) {
	mute := helpRowsByLabel(t, nil)["Mute"]
	if mute.primary != "m" {
		t.Errorf("Mute primary = %q, want \"m\"", mute.primary)
	}
	if mute.secondary != "" {
		t.Errorf("Mute secondary = %q, want empty", mute.secondary)
	}
}

func TestHelpRowsReflectRebinding(t *testing.T) {
	pp := helpRowsByLabel(t, map[actionID][]string{actPlayPause: {"p"}})["Play / Pause"]
	if pp.primary != "p" {
		t.Errorf("rebinding not reflected: primary = %q, want \"p\"", pp.primary)
	}
	if strings.Contains(pp.secondary, "space") || strings.Contains(pp.secondary, "k") {
		t.Errorf("replaced defaults still shown: secondary = %q", pp.secondary)
	}
}

func TestHelpRowsPreferencesShowsMenuMarker(t *testing.T) {
	byLabel := helpRowsByLabel(t, nil)
	prefs, ok := byLabel["Preferences"]
	if !ok {
		t.Fatal("no Preferences row in help")
	}
	if prefs.primary != "menu" {
		t.Errorf("Preferences primary = %q, want \"menu\"", prefs.primary)
	}
}

// The panel is split so it is not one tall list: each half gets its share of
// the registry, the left one keeps the extra row on an odd count, and together
// they are the whole registry in declaration order.
func TestSplitHelpRowsBalancesTheColumns(t *testing.T) {
	all := helpRows(defaultActions(), nil)
	left, right := splitHelpRows(all)

	if len(left) < len(right) || len(left)-len(right) > 1 {
		t.Errorf("columns %d/%d, want them within one row of each other, longer on the left",
			len(left), len(right))
	}
	joined := append(append([]helpRow{}, left...), right...)
	if len(joined) != len(all) {
		t.Fatalf("split covers %d rows, want all %d", len(joined), len(all))
	}
	for i := range all {
		if joined[i] != all[i] {
			t.Errorf("row %d = %+v, want %+v — the split must not reorder the registry", i, joined[i], all[i])
		}
	}
}

func TestSplitHelpRowsHandlesShortLists(t *testing.T) {
	for _, n := range []int{0, 1, 2, 3} {
		left, right := splitHelpRows(make([]helpRow, n))
		if len(left)+len(right) != n {
			t.Errorf("splitting %d rows yielded %d+%d", n, len(left), len(right))
		}
		if len(right) > len(left) {
			t.Errorf("splitting %d rows put %d on the right and %d on the left", n, len(right), len(left))
		}
	}
}

// helpTheme builds the shaper the width measurements below need. No display or
// GPU is involved: text shaping is pure measurement.
func helpTheme(t *testing.T) *material.Theme {
	t.Helper()
	th := material.NewTheme()
	th.Shaper = text.NewShaper(text.WithCollection(gofont.Collection()))
	return th
}

// measureHelpText lays a Body2 label out unconstrained and returns the width it
// wants, in dp (the harness runs at 1 px per dp).
func measureHelpText(t *testing.T, th *material.Theme, s string) int {
	t.Helper()
	var ops op.Ops
	gtx := layout.Context{
		Ops:         &ops,
		Now:         time.Time{},
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Constraints{Max: image.Pt(1e6, 1e6)},
	}
	return material.Body2(th, s).Layout(gtx).Size.X
}

// The three cells are fixed widths, so anything wider than its cell is silently
// truncated or wrapped rather than reported. Check every string the panel can
// actually draw against the cell it lands in — a longer action label or a new
// named key is exactly the change that would overflow one.
func TestHelpCellsFitTheirWidestText(t *testing.T) {
	th := helpTheme(t)
	km, err := buildKeymap(nil)
	if err != nil {
		t.Fatal(err)
	}

	cells := []struct {
		name  string
		width unit.Dp
		texts []string
	}{
		{"label", helpLabelW, []string{"primary"}},
		{"primary key", helpKeyW, []string{"primary"}},
		{"secondary key", helpAltKeyW, []string{"secondary"}},
	}
	for _, r := range helpRows(defaultActions(), km) {
		cells[0].texts = append(cells[0].texts, r.label)
		cells[1].texts = append(cells[1].texts, r.primary)
		cells[2].texts = append(cells[2].texts, r.secondary)
	}
	// Every chord the parser can name, in the column that has to hold it: a key
	// the user rebinds to is not in the defaults above.
	for name := range namedKeys {
		cells[1].texts = append(cells[1].texts, name)
		cells[2].texts = append(cells[2].texts, name)
	}

	for _, c := range cells {
		for _, s := range c.texts {
			if got, limit := measureHelpText(t, th, s), int(c.width); got > limit {
				t.Errorf("%s cell: %q wants %d dp but the cell is %d dp — it will wrap or clip",
					c.name, s, got, limit)
			}
		}
	}
}

// measureHelpColumns lays the panel's column block out at the real widths and
// returns its size in dp. Not layoutHelp itself: layout.Center reports the
// constraints it was handed, which would measure the window, not the panel.
func measureHelpColumns(t *testing.T, p *Player, window image.Point, columns ...[]helpRow) layout.Dimensions {
	t.Helper()
	var ops op.Ops
	gtx := layout.Context{
		Ops:         &ops,
		Now:         time.Time{},
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(window),
	}
	// Min zeroed, matching what layout.Center hands the panel: with Min == Max a
	// Flex fills its parent and every measurement comes back as the window size.
	gtx.Constraints.Min = image.Point{}
	children := make([]layout.FlexChild, 0, 2*len(columns))
	for i, col := range columns {
		col := col
		if i > 0 {
			children = append(children, layout.Rigid(layout.Spacer{Width: helpColumnGap}.Layout))
		}
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return p.layoutHelpColumn(gtx, col)
		}))
	}
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, children...)
}

// The point of the split: the panel is materially shorter than the single tall
// list it replaced, and still fits the window govi opens at. (`minWidth` is not
// the yardstick — it is a 240 dp floor for the *video*, where no overlay fits.)
func TestHelpPanelIsShorterInTwoColumns(t *testing.T) {
	km, err := buildKeymap(nil)
	if err != nil {
		t.Fatal(err)
	}
	p := &Player{theme: helpTheme(t), keymap: km}
	window := image.Pt(initialWidth, initialHeight)
	all := helpRows(defaultActions(), km)
	left, right := splitHelpRows(all)

	split := measureHelpColumns(t, p, window, left, right)
	single := measureHelpColumns(t, p, window, all)

	if split.Size.Y*3 > single.Size.Y*2 {
		t.Errorf("two columns are %d dp tall against one column's %d dp, want appreciably shorter",
			split.Size.Y, single.Size.Y)
	}
	if split.Size.X <= single.Size.X {
		t.Errorf("two columns are %d dp wide against one column's %d dp — the split should trade height for width",
			split.Size.X, single.Size.X)
	}
	if split.Size.Y > window.Y || split.Size.X > window.X {
		t.Errorf("help panel is %v in a %v window", split.Size, window)
	}
}
