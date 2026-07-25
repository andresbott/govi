package player

import (
	"fmt"
	"image"
	"image/color"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/go-gl/glfw/v3.3/glfw"
)

// prefsRow is one action line in the preferences overlay; slots hold the
// clickable state for the two binding chips, clears the state for the small
// button beside each chip.
type prefsRow struct {
	id     actionID
	label  string
	slots  [2]widget.Clickable
	clears [2]widget.Clickable
}

// prefsCapture marks which binding slot is waiting for a key press.
type prefsCapture struct {
	id   actionID
	slot int
}

// buildPrefsRows returns one row per registry action, in display order.
func buildPrefsRows() []*prefsRow {
	acts := defaultActions()
	rows := make([]*prefsRow, 0, len(acts))
	for _, a := range acts {
		rows = append(rows, &prefsRow{id: a.id, label: a.label})
	}
	return rows
}

// prefsAction is what the render loop must do to reconcile the preferences
// window with the state the UI asked for.
type prefsAction int

const (
	prefsNoop prefsAction = iota
	prefsCreate
	prefsDestroy
)

// prefsWindowAction compares the requested state with the live one. GLFW
// windows may only be created and destroyed from the main loop, never from
// inside a GLFW callback, so the UI flips a flag and the loop reconciles.
func prefsWindowAction(want, have bool) prefsAction {
	switch {
	case want && !have:
		return prefsCreate
	case !want && have:
		return prefsDestroy
	}
	return prefsNoop
}

// togglePrefs requests the preferences window, or revokes it when it is
// already requested.
func (p *Player) togglePrefs() {
	if p.prefsWanted {
		p.closePrefs()
		return
	}
	p.openPrefs()
}

// openPrefs rebuilds the rows (registry order) and requests the window. The
// video overlays are untouched: preferences live in their own window.
func (p *Player) openPrefs() {
	p.prefsRows = buildPrefsRows()
	p.prefsList.Axis = layout.Vertical
	p.capture = nil
	p.prefsMsg = ""
	p.prefsWanted = true
}

// closePrefs revokes the window and drops any pending capture.
func (p *Player) closePrefs() {
	p.capture = nil
	p.prefsWanted = false
}

// handlePrefsKey is the preferences window's key handler. A capturing slot
// consumes the press (Esc cancels, every other key binds); otherwise Esc
// closes the window and every other key is ignored — player shortcuts are not
// dispatched from this window.
func (p *Player) handlePrefsKey(key glfw.Key, mods glfw.ModifierKey) {
	if p.handleCaptureKey(key, mods) {
		return
	}
	if key == glfw.KeyEscape {
		p.closePrefs()
	}
}

const (
	prefsLabelW = unit.Dp(170)
	prefsSlotW  = unit.Dp(110)
	prefsSlotH  = unit.Dp(28)
	prefsRowH   = unit.Dp(38)
	// prefsClearW is the square clear/restore button beside each chip.
	prefsClearW = unit.Dp(28)
)

// preferences window palette. Opaque throughout: unlike the video overlays
// there is nothing behind this window to show through, and translucent text
// over a translucent panel is what made the glyphs look muddy.
var (
	prefsFG      = color.NRGBA{R: 0xf0, G: 0xf0, B: 0xf2, A: 0xff}
	prefsFGDim   = color.NRGBA{R: 0xb4, G: 0xb4, B: 0xbe, A: 0xff}
	prefsFGWarn  = color.NRGBA{R: 0xff, G: 0xa0, B: 0x40, A: 0xff}
	prefsChip    = color.NRGBA{R: 0x32, G: 0x32, B: 0x3c, A: 0xff}
	prefsChipHi  = color.NRGBA{R: 0x45, G: 0x45, B: 0x52, A: 0xff}
	prefsChipCap = color.NRGBA{R: 0x4a, G: 0x5c, B: 0x82, A: 0xff}
	prefsFGOff   = color.NRGBA{R: 0x60, G: 0x60, B: 0x6a, A: 0xff}
)

// Clear-button glyphs. Both exist in the Go fonts (many symbol code points do
// not, and a missing glyph renders as a blank chip): "×" clears a binding,
// "←" reverts an action to its defaults.
const (
	prefsGlyphClear   = "×"
	prefsGlyphRestore = "←"
)

// layoutPrefsWindow fills the preferences window: a title, a hint line, a
// message line for errors, and one scrollable row per action with two
// clickable binding chips. The window itself is the panel, so there is no
// translucent background or centering here (contrast the video overlays).
// The theme is the window's own (see prefsWindow.theme), never the video
// window's.
func (p *Player) layoutPrefsWindow(gtx layout.Context, th *material.Theme) layout.Dimensions {
	return layout.UniformInset(unit.Dp(20)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		msg := p.prefsMsg
		msgColor := prefsFGWarn
		if msg == "" {
			msg = " " // keep the line height stable
			msgColor = prefsFGDim
		}

		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				lbl := material.H6(th, "Shortcuts")
				lbl.Color = prefsFG
				return lbl.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				lbl := material.Body2(th, "Click a binding, then press the new key. Esc cancels.")
				lbl.Color = prefsFGDim
				return lbl.Layout(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				lbl := material.Body2(th, prefsGlyphClear+" clears a binding — "+
					prefsGlyphRestore+" on a cleared slot restores the defaults.")
				lbl.Color = prefsFGDim
				return lbl.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				lbl := material.Body2(th, msg)
				lbl.Color = msgColor
				return lbl.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return material.List(th, &p.prefsList).Layout(gtx, len(p.prefsRows),
					func(gtx layout.Context, i int) layout.Dimensions {
						return p.layoutPrefsRow(gtx, th, p.prefsRows[i])
					})
			}),
		)
	})
}

// layoutPrefsRow draws one action row: label plus two binding chips, each
// followed by its clear/restore button.
func (p *Player) layoutPrefsRow(gtx layout.Context, th *material.Theme, row *prefsRow) layout.Dimensions {
	// Handle clicks BEFORE Layout (Layout drains pending clicks).
	for si := range row.slots {
		if row.slots[si].Clicked(gtx) {
			p.capture = &prefsCapture{id: row.id, slot: si}
			p.prefsMsg = ""
		}
	}
	keys := effectiveKeys(row.id, p.overrides)
	for si := range row.clears {
		if !row.clears[si].Clicked(gtx) {
			continue
		}
		p.capture = nil
		switch clearKind(row.id, si, keys, p.overrides) {
		case clearBinding:
			p.setBindingWithMsg(row.id, si, "")
		case clearRestore:
			p.restoreDefaultsWithMsg(row.id)
		}
	}
	keys = effectiveKeys(row.id, p.overrides)
	gtx.Constraints.Min.Y = gtx.Dp(prefsRowH)
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = gtx.Dp(prefsLabelW)
			lbl := material.Body1(th, row.label)
			lbl.Color = prefsFG
			return lbl.Layout(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return p.layoutSlotChip(gtx, th, row, 0, keys)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return p.layoutClearButton(gtx, th, row, 0, keys)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return p.layoutSlotChip(gtx, th, row, 1, keys)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return p.layoutClearButton(gtx, th, row, 1, keys)
		}),
	)
}

// clearAction is what a slot's clear button does when clicked, given the
// slot's current state.
type clearAction int

const (
	// clearNoop: nothing to clear and nothing to restore (disabled button).
	clearNoop clearAction = iota
	// clearBinding: the slot holds a key; clicking unsets it.
	clearBinding
	// clearRestore: the slot is empty and the action is overridden; clicking
	// puts the action's built-in defaults back. This is how defaults are
	// restored without a second button per row: clear a slot, then click the
	// same button again.
	clearRestore
)

// clearKind decides what slot's clear button does for action id.
func clearKind(id actionID, slot int, keys []string, overrides map[actionID][]string) clearAction {
	if slot < len(keys) && keys[slot] != "" {
		return clearBinding
	}
	if _, ok := overrides[id]; ok && len(actionDefaults()[id]) > 0 {
		return clearRestore
	}
	return clearNoop
}

// layoutClearButton draws the small button beside a binding chip: "×" while
// the slot holds a key, "←" once the slot is empty and the action can be
// restored to its defaults, dimmed and inert otherwise.
func (p *Player) layoutClearButton(gtx layout.Context, th *material.Theme, row *prefsRow, slot int, keys []string) layout.Dimensions {
	kind := clearKind(row.id, slot, keys, p.overrides)
	txt := prefsGlyphClear
	col := prefsFG
	switch kind {
	case clearRestore:
		txt = prefsGlyphRestore
	case clearNoop:
		col = prefsFGOff
	}
	w := gtx.Dp(prefsClearW)
	h := gtx.Dp(prefsSlotH)
	gtx.Constraints = layout.Exact(image.Pt(w, h))
	return row.clears[slot].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		if kind != clearNoop && row.clears[slot].Hovered() {
			rr := clip.RRect{Rect: image.Rectangle{Max: image.Pt(w, h)}, SE: 4, SW: 4, NE: 4, NW: 4}
			paint.FillShape(gtx.Ops, prefsChipHi, rr.Op(gtx.Ops))
		}
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			lbl := material.Body1(th, txt)
			lbl.Color = col
			return lbl.Layout(gtx)
		})
	})
}

// layoutSlotChip draws one clickable binding chip. The armed chip shows
// "press a key…" highlighted; an empty slot shows "—".
func (p *Player) layoutSlotChip(gtx layout.Context, th *material.Theme, row *prefsRow, slot int, keys []string) layout.Dimensions {
	capturing := p.capture != nil && p.capture.id == row.id && p.capture.slot == slot
	txt := "—"
	if slot < len(keys) && keys[slot] != "" {
		txt = keys[slot]
	}
	if capturing {
		txt = "press a key…"
	}
	w := gtx.Dp(prefsSlotW)
	h := gtx.Dp(prefsSlotH)
	gtx.Constraints = layout.Exact(image.Pt(w, h))
	return row.slots[slot].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		bg := prefsChip
		if capturing {
			bg = prefsChipCap
		} else if row.slots[slot].Hovered() {
			bg = prefsChipHi
		}
		rr := clip.RRect{Rect: image.Rectangle{Max: image.Pt(w, h)}, SE: 4, SW: 4, NE: 4, NW: 4}
		paint.FillShape(gtx.Ops, bg, rr.Op(gtx.Ops))
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			lbl := material.Body2(th, txt)
			lbl.Color = prefsFG
			return lbl.Layout(gtx)
		})
	})
}

// effectiveKeys returns the key strings currently in force for id: the user
// override when present, else the registry defaults. "none" entries become
// "" so the result is positional — index 0 is always the primary slot, even
// when it is empty — and a trailing all-empty tail is trimmed so an action
// with no bindings at all reports none rather than a list of blanks.
func effectiveKeys(id actionID, overrides map[actionID][]string) []string {
	keys, ok := overrides[id]
	if !ok {
		return actionDefaults()[id]
	}
	out := make([]string, len(keys))
	last := -1
	for i, k := range keys {
		if isNoneKey(k) {
			continue
		}
		out[i] = k
		last = i
	}
	if last < 0 {
		return nil
	}
	return out[:last+1]
}

// applyBinding sets slot (0 or 1) of action id to keyStr ("" clears the
// slot), validates the whole keymap, and on success commits the new
// overrides, rebuilds dispatch, and persists via the save callback. The
// returned error is user-facing (shown on the prefs message line). Editing
// an action truncates it to the two visible slots; extra keys a hand-written
// config may carry beyond two are dropped for that action.
//
// Slots keep their position: a cleared slot is written as "none" rather than
// dropped, so clearing the primary binding leaves the secondary where the
// user put it instead of promoting it.
func (p *Player) applyBinding(id actionID, slot int, keyStr string) error {
	if slot < 0 || slot > 1 {
		return fmt.Errorf("invalid slot %d", slot)
	}
	var slots [2]string
	for i, k := range effectiveKeys(id, p.overrides) {
		if i > 1 {
			break
		}
		slots[i] = k
	}
	slots[slot] = keyStr
	return p.commitOverrides(p.overridesWithSlots(id, slots))
}

// overridesWithSlots returns a copy of the overrides with id set to slots.
// Empty slots become "none" placeholders, a wholly empty action collapses to
// a single "none", and an action back at its defaults drops its override
// entirely (so the config only records real deviations).
func (p *Player) overridesWithSlots(id actionID, slots [2]string) map[actionID][]string {
	keys := make([]string, 0, len(slots))
	last := -1
	for i, k := range slots {
		if k != "" {
			last = i
		}
		keys = append(keys, k)
	}
	newOv := p.cloneOverrides()
	if last < 0 {
		// Nothing bound: "none" alone, unless there is no default to suppress.
		if len(actionDefaults()[id]) > 0 {
			newOv[id] = []string{"none"}
		} else {
			delete(newOv, id)
		}
		return newOv
	}
	keys = keys[:last+1]
	for i, k := range keys {
		if k == "" {
			keys[i] = "none"
		}
	}
	if equalKeyLists(keys, actionDefaults()[id]) {
		delete(newOv, id)
		return newOv
	}
	newOv[id] = keys
	return newOv
}

// restoreDefaults drops the user override for id, putting the built-in keys
// back in force. Like applyBinding it validates before committing, so an
// override that still holds one of id's default keys is reported as a
// conflict instead of silently winning.
func (p *Player) restoreDefaults(id actionID) error {
	if _, ok := p.overrides[id]; !ok {
		return nil
	}
	newOv := p.cloneOverrides()
	delete(newOv, id)
	return p.commitOverrides(newOv)
}

func (p *Player) cloneOverrides() map[actionID][]string {
	ov := make(map[actionID][]string, len(p.overrides)+1)
	for aid, ks := range p.overrides {
		ov[aid] = ks
	}
	return ov
}

// commitOverrides validates newOv as a whole, and only on success adopts it,
// rebuilds dispatch, and persists it through the save callback. A conflict
// leaves the player untouched and nothing is written.
func (p *Player) commitOverrides(newOv map[actionID][]string) error {
	km, err := buildKeymap(newOv)
	if err != nil {
		return err
	}
	p.overrides = newOv
	p.keymap = km

	if p.saveShortcuts != nil {
		sc := make(map[string][]string, len(newOv))
		for aid, ks := range newOv {
			sc[string(aid)] = ks
		}
		if err := p.saveShortcuts(sc); err != nil {
			p.log.Error("save shortcuts", "err", err)
			return fmt.Errorf("applied for this session, but saving config failed: %v", err)
		}
	}
	return nil
}

func equalKeyLists(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// captureChordString renders a pressed key as a config-syntax string and
// verifies it round-trips through parseChord, so anything the preferences
// screen persists is guaranteed to load back.
func captureChordString(key glfw.Key, mods glfw.ModifierKey) (string, error) {
	chord := keyChord{key: key, mods: relevantMods(mods)}
	s := chordLabel(chord)
	parsed, err := parseChord(s)
	if err != nil || parsed != chord {
		return "", fmt.Errorf("key cannot be represented in config")
	}
	return s, nil
}

// isModifierKey reports whether k is a modifier pressed on its own, which the
// capture handler ignores (the user is still forming a chord).
func isModifierKey(k glfw.Key) bool {
	switch k {
	case glfw.KeyLeftShift, glfw.KeyRightShift,
		glfw.KeyLeftControl, glfw.KeyRightControl,
		glfw.KeyLeftAlt, glfw.KeyRightAlt,
		glfw.KeyLeftSuper, glfw.KeyRightSuper:
		return true
	}
	return false
}

// handleCaptureKey consumes one key press while a binding slot is capturing:
// Esc cancels, a lone modifier is ignored, anything else becomes the new
// binding. Backspace and Delete are ordinary bindable keys here — clearing a
// slot is the row's "×" button, precisely so that those two keys (and chords
// built on them) can be bound. Returns true when the event was consumed and
// must not be dispatched further.
func (p *Player) handleCaptureKey(key glfw.Key, mods glfw.ModifierKey) bool {
	if p.capture == nil {
		return false
	}
	if isModifierKey(key) {
		return true
	}
	c := *p.capture
	p.capture = nil
	if key == glfw.KeyEscape {
		p.prefsMsg = ""
		return true
	}
	s, err := captureChordString(key, mods)
	if err != nil {
		p.prefsMsg = "that key cannot be bound"
		return true
	}
	p.setBindingWithMsg(c.id, c.slot, s)
	return true
}

// setBindingWithMsg applies a binding and mirrors the outcome on the prefs
// message line.
func (p *Player) setBindingWithMsg(id actionID, slot int, keyStr string) {
	if err := p.applyBinding(id, slot, keyStr); err != nil {
		p.prefsMsg = err.Error()
		return
	}
	p.prefsMsg = ""
}

// restoreDefaultsWithMsg restores an action's defaults and mirrors the outcome
// on the prefs message line.
func (p *Player) restoreDefaultsWithMsg(id actionID) {
	if err := p.restoreDefaults(id); err != nil {
		p.prefsMsg = err.Error()
		return
	}
	p.prefsMsg = ""
}
