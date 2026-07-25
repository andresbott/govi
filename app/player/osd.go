package player

import (
	"fmt"
	"image"
	"image/color"
	"math"
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
func (p *Player) flash(msg string) {
	p.osdText = msg
	p.osdUntil = time.Now().Add(osdDuration)
	p.osdProgress = false
}

// osdVisible reports whether a flash is set and has not expired at now.
func (p *Player) osdVisible(now time.Time) bool {
	return p.osdText != "" && now.Before(p.osdUntil)
}

// volumeStatus renders the flash line for a volume change: the level in
// percent, or "Muted" while mute is on (the level is irrelevant then).
func volumeStatus(vol int64, muted bool) string {
	if muted {
		return "Muted"
	}
	return fmt.Sprintf("Volume %d%%", vol)
}

// positionStatus renders the flash line for playlist navigation: the 1-based
// position, the entry count, and the file name.
func positionStatus(idx, total int, path string) string {
	return fmt.Sprintf("%d / %d   %s", idx+1, total, filepath.Base(path))
}

// progressStatus renders the flash line for a position change: elapsed time,
// total time and the percentage played. A non-positive duration (live stream, or
// mpv not knowing it yet) drops the total and the percentage rather than
// printing a bogus 0:00 or a division by zero.
func progressStatus(pos, dur float64) string {
	if dur <= 0 {
		return humanClock(pos)
	}
	pct := int(math.Round(pos / dur * 100))
	return fmt.Sprintf("%s / %s   %d%%", humanClock(pos), humanClock(dur), pct)
}

// flashProgress flashes how far into the current file playback is, and keeps it
// updating for as long as the flash is visible (see refreshOSD): mpv applies a
// seek asynchronously, so the position read right after the command is still the
// old one. It is a no-op with no file loaded, where there is nothing to report.
func (p *Player) flashProgress() {
	if p.mpv == nil || p.idle.Load() { // unit tests build a Player without mpv
		return
	}
	p.flash(progressStatus(p.propFloat("time-pos"), p.propFloat("duration")))
	p.osdProgress = true
}

// refreshOSD re-reads a live progress flash, called once per loop iteration. Any
// other flash (volume, playlist position) is a snapshot and left alone.
func (p *Player) refreshOSD(now time.Time) {
	if !p.osdProgress {
		return
	}
	if !p.osdVisible(now) {
		p.osdProgress = false
		return
	}
	p.osdText = progressStatus(p.propFloat("time-pos"), p.propFloat("duration"))
}

// flashVolume flashes the current volume as reported by mpv, so the clamped
// value (volume-max) is shown rather than the requested one.
func (p *Player) flashVolume() {
	var vol int64
	if p.mpv != nil { // unit tests build a Player without mpv
		vol = p.propInt("volume")
	}
	p.flash(volumeStatus(vol, p.muted))
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

// layoutOSD draws the status flash at the bottom center, above where playback
// controls would sit. Nothing is drawn once the flash expired.
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
