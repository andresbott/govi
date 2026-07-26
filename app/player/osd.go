package player

import (
	"fmt"
	"image"
	"image/color"
	"path/filepath"
	"time"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

// osdDuration is how long a status flash stays on screen. The loop renders at
// least every idleFrame seconds, so the flash disappears on its own without
// any extra invalidation.
const osdDuration = 1200 * time.Millisecond

// flash shows msg over the video for osdDuration, replacing any flash still
// visible. An empty msg clears it.
//
// Every flash is a snapshot: playback progress and the volume are both the control
// bar's job (controls.go), whose sliders re-read the observed values every frame,
// so nothing here has to keep updating itself. Playlist position is the only
// remaining flash — it is genuinely a one-shot message, not a live value.
func (p *Player) flash(msg string) {
	p.osdText = msg
	p.osdUntil = time.Now().Add(osdDuration)
}

// osdVisible reports whether a flash is set and has not expired at now.
func (p *Player) osdVisible(now time.Time) bool {
	return p.osdText != "" && now.Before(p.osdUntil)
}

// positionStatus renders the flash line for playlist navigation: the 1-based
// position, the entry count, and the file name.
func positionStatus(idx, total int, path string) string {
	return fmt.Sprintf("%d / %d   %s", idx+1, total, filepath.Base(path))
}

// flashPosition flashes where the playlist now stands. It is a no-op without a
// playlist entry to report.
func (p *Player) flashPosition() {
	cur := p.pl.current()
	if cur == "" {
		return
	}
	p.flash(positionStatus(p.pl.idx, len(p.pl.entries), cur))
}

// layoutOSD draws the status flash at the bottom center, above where the control
// bar sits. Nothing is drawn once the flash expired.
func (p *Player) layoutOSD(gtx layout.Context) {
	if !p.osdVisible(gtx.Now) {
		return
	}
	layout.S.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Bottom: unit.Dp(40)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			// S only relaxes Min.Y, so the panel would report the full window
			// width and end up flush left; zeroing Min lets it shrink to its
			// text so S can center it (layout.Center does this itself).
			gtx.Constraints.Min = image.Point{}
			return p.drawPanel(gtx, func(gtx layout.Context) layout.Dimensions {
				lbl := material.Body1(p.theme, p.osdText)
				lbl.Color = color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
				return lbl.Layout(gtx)
			})
		})
	})
}
