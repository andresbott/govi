package player

import (
	"fmt"
	"image"
	"image/color"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

// menuItem is one row in the context menu. A row is either a separator, a
// leaf with onSelect, or a parent with a non-empty sub slice.
type menuItem struct {
	label     string
	checked   bool
	separator bool
	onSelect  func()
	sub       []*menuItem
	click     widget.Clickable
	open      bool
}

func sep() *menuItem { return &menuItem{separator: true} }

// closeSubtree recursively clears the open flag of it and its descendants.
func closeSubtree(it *menuItem) {
	it.open = false
	for _, s := range it.sub {
		closeSubtree(s)
	}
}

// actionItem builds a leaf that invokes a registry action by id.
func (p *Player) actionItem(id actionID) *menuItem {
	a := p.actions[id]
	return &menuItem{
		label:    a.label,
		onSelect: func() { p.runAction(id) },
	}
}

// runAction dispatches a registry action (shared with keyboard dispatch).
func (p *Player) runAction(id actionID) {
	if a, ok := p.actions[id]; ok && a.fn != nil {
		a.fn(p)
	}
}

// trackSubmenu builds the "Track ▸" list for a track kind, with a check on
// the selected one and (for audio/sub) a trailing "Disabled" entry.
func (p *Player) trackSubmenu(kind string, allowDisable bool) []*menuItem {
	var items []*menuItem
	if p.mpv != nil {
		for _, t := range p.trackList() {
			if t.kind != kind {
				continue
			}
			id := t.id
			items = append(items, &menuItem{
				label:    t.label(),
				checked:  t.selected,
				onSelect: func() { p.setTrack(kind, id) },
			})
		}
	}
	if allowDisable {
		items = append(items, &menuItem{
			label:    "Disabled",
			onSelect: func() { p.setTrack(kind, 0) },
		})
	}
	return items
}

// buildMenu assembles the context menu. When idle, only the entries that make
// sense on the idle screen are shown.
func (p *Player) buildMenu(idle bool) []*menuItem {
	if idle {
		return []*menuItem{
			p.actionItem(actHelp),
			p.actionItem(actFullscreen),
			sep(),
			p.actionItem(actPreferences),
			p.actionItem(actQuit),
		}
	}

	// Audio submenu: Track, Mute.
	audio := &menuItem{label: "Audio", sub: []*menuItem{
		{label: "Track", sub: p.trackSubmenu("audio", true)},
		p.actionItem(actMute),
	}}

	// Video submenu: Track (only if >1 video track), Aspect Ratio, Zoom.
	videoSub := []*menuItem{}
	if vt := p.videoTracks(); len(vt) > 1 {
		videoSub = append(videoSub, &menuItem{label: "Track", sub: p.trackSubmenu("video", false)})
	}
	aspect := &menuItem{label: "Aspect Ratio"}
	for _, opt := range aspectOptions() {
		value := opt.value
		aspect.sub = append(aspect.sub, &menuItem{label: opt.label, onSelect: func() { p.setAspect(value) }})
	}
	zoom := &menuItem{label: "Zoom"}
	for _, f := range zoomOptions() {
		factor := f
		zoom.sub = append(zoom.sub, &menuItem{label: fmt.Sprintf("%g×", f), onSelect: func() { p.setZoom(factor) }})
	}
	videoSub = append(videoSub, aspect, zoom)
	video := &menuItem{label: "Video", sub: videoSub}

	subtitles := &menuItem{label: "Subtitles", sub: []*menuItem{
		{label: "Track", sub: p.trackSubmenu("sub", true)},
	}}

	return []*menuItem{
		p.actionItem(actPlayPause),
		p.actionItem(actStop),
		p.actionItem(actNextVideo),
		p.actionItem(actPrevVideo),
		p.actionItem(actVolumeUp),
		p.actionItem(actVolumeDown),
		sep(),
		audio,
		video,
		subtitles,
		sep(),
		p.actionItem(actInfo),
		p.actionItem(actHelp),
		p.actionItem(actPreferences),
		p.actionItem(actFullscreen),
		sep(),
		p.actionItem(actQuit),
	}
}

// videoTracks returns just the video tracks (nil when mpv is unavailable).
func (p *Player) videoTracks() []trackInfo {
	if p.mpv == nil {
		return nil
	}
	var out []trackInfo
	for _, t := range p.trackList() {
		if t.kind == "video" {
			out = append(out, t)
		}
	}
	return out
}

const (
	menuWidth = unit.Dp(220)
	menuRowH  = unit.Dp(28)
)

// layoutMenu positions the top-level menu at the cursor (clamped to the
// window) and recursively draws any open submenus beside their hovered parent.
func (p *Player) layoutMenu(gtx layout.Context) {
	if len(p.menuItems) == 0 {
		return
	}
	wpx := gtx.Dp(menuWidth)
	rowpx := gtx.Dp(menuRowH)

	x := int(p.menuPos.X)
	y := int(p.menuPos.Y)
	// Clamp so the menu stays inside the window.
	if x+wpx > gtx.Constraints.Max.X {
		x = gtx.Constraints.Max.X - wpx
	}
	if x < 0 {
		x = 0
	}
	menuH := rowpx * len(p.menuItems)
	if y+menuH > gtx.Constraints.Max.Y {
		y = gtx.Constraints.Max.Y - menuH
	}
	if y < 0 {
		y = 0
	}

	p.drawMenuList(gtx, p.menuItems, image.Pt(x, y), wpx, rowpx)
}

// drawMenuList draws one column of items at origin and, for any hovered parent
// with a submenu, draws the submenu to its right (clamped).
func (p *Player) drawMenuList(gtx layout.Context, items []*menuItem, origin image.Point, wpx, rowpx int) {
	defer op.Offset(origin).Push(gtx.Ops).Pop()

	// Background panel.
	h := rowpx * len(items)
	rr := clip.RRect{Rect: image.Rectangle{Max: image.Pt(wpx, h)}, SE: 6, SW: 6, NE: 6, NW: 6}
	paint.FillShape(gtx.Ops, panelBG, rr.Op(gtx.Ops))

	var openSub, hovered *menuItem
	var openSubY, hoveredY int
	for i, it := range items {
		iy := rowpx * i
		if it.separator {
			// thin divider line
			off := op.Offset(image.Pt(0, iy+rowpx/2)).Push(gtx.Ops)
			paint.FillShape(gtx.Ops, color.NRGBA{R: 0x88, G: 0x88, B: 0x88, A: 0x80},
				clip.Rect{Min: image.Pt(8, 0), Max: image.Pt(wpx-8, 1)}.Op())
			off.Pop()
			continue
		}
		// Handle selection before Layout: Clickable.Layout drains pending
		// click events, so a Clicked check after it never fires.
		if it.click.Clicked(gtx) {
			if it.onSelect != nil {
				it.onSelect()
				p.closeMenu()
			}
		}
		// Clickable row.
		rowOff := op.Offset(image.Pt(0, iy)).Push(gtx.Ops)
		rgtx := gtx
		rgtx.Constraints = layout.Exact(image.Pt(wpx, rowpx))
		it.click.Layout(rgtx, func(gtx layout.Context) layout.Dimensions {
			if it.click.Hovered() {
				paint.FillShape(gtx.Ops, color.NRGBA{R: 0x55, G: 0x55, B: 0x66, A: 0xa0},
					clip.Rect{Max: image.Pt(wpx, rowpx)}.Op())
			}
			return p.menuRowContent(gtx, it, wpx)
		})
		rowOff.Pop()

		if it.click.Hovered() {
			hovered = it
			hoveredY = iy
		}
		if it.open {
			openSub = it
			openSubY = iy
		}
	}

	// Hovering a row in this column switches the open submenu; while the
	// pointer is elsewhere (e.g. inside the open submenu itself), the open
	// state sticks so the submenu stays clickable.
	if hovered != nil && hovered != openSub {
		if openSub != nil {
			closeSubtree(openSub)
			openSub = nil
		}
		if len(hovered.sub) > 0 {
			hovered.open = true
			openSub = hovered
			openSubY = hoveredY
		}
	}

	// Draw the hovered submenu beside this column.
	if openSub != nil {
		subOrigin := image.Pt(origin.X+wpx, origin.Y+openSubY)
		if subOrigin.X+wpx > gtx.Constraints.Max.X {
			subOrigin.X = origin.X - wpx // flip to the left
		}
		p.drawSubmenu(gtx, openSub.sub, subOrigin, wpx, rowpx, origin)
	}
}

// drawSubmenu draws a submenu at an absolute origin. parentOrigin is the
// offset already applied by the caller, subtracted so the submenu lands at
// its true screen position.
func (p *Player) drawSubmenu(gtx layout.Context, items []*menuItem, absOrigin image.Point, wpx, rowpx int, parentOrigin image.Point) {
	rel := absOrigin.Sub(parentOrigin)
	p.drawMenuList(gtx, items, rel, wpx, rowpx)
}

// menuRowContent draws the label, an optional leading check, and a trailing
// "▸" for parents.
func (p *Player) menuRowContent(gtx layout.Context, it *menuItem, wpx int) layout.Dimensions {
	return layout.UniformInset(unit.Dp(6)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceBetween}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				txt := it.label
				if it.checked {
					txt = "✓ " + txt
				}
				lbl := material.Body2(p.theme, txt)
				lbl.Color = color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
				return lbl.Layout(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if len(it.sub) == 0 {
					return layout.Dimensions{}
				}
				lbl := material.Body2(p.theme, "▸")
				lbl.Color = color.NRGBA{R: 0xcc, G: 0xcc, B: 0xcc, A: 0xff}
				return lbl.Layout(gtx)
			}),
		)
	})
}
