package player

import (
	"fmt"
	"image"
	"image/color"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"golang.org/x/exp/shiny/materialdesign/icons"
)

// Control-bar geometry. The bar is a thin track with a round knob riding on it;
// trackHeight is deliberately only a few pixels so it reads as a line, not a
// panel. knobRadius is only the drawn circle; knobGrab is the larger hit radius
// (via Float's pointer margin) and sets the row height, so shrinking the visible
// knob does not shrink the target the pointer has to hit.
const (
	trackHeight = unit.Dp(2)
	knobRadius  = unit.Dp(6)
	knobGrab    = unit.Dp(7)
	// barInset is the gap between the bar row and the window edges. The row is
	// laid out at the window width, so the bar itself scales with the viewport.
	barInset = unit.Dp(16)
	// volumeBarWidth is the volume slider's fixed width. Unlike the seek bar it
	// does not grow with the viewport: the volume range is the same 0..volumeMax at
	// any window size, and a slider as wide as the window would read as the more
	// important of the two. At 120 dp a whole percent is still just over a pixel, so
	// the knob has room to be aimed rather than only nudged.
	volumeBarWidth = unit.Dp(120)
	// The icon buttons' glyph size and padding. They are the tallest thing in the
	// row, so together they set the row's height — the sliders are only
	// 2*knobGrab tall and sit centered against them.
	buttonGlyph = unit.Dp(22)
	buttonPad   = unit.Dp(8)
	// controlsHideAfter is how long the bar stays up after the last pointer
	// input. The loop renders every idleFrame, so this needs no timer — the same
	// reasoning as the status flash in osd.go.
	controlsHideAfter = 1 * time.Second
	// controlsFade is how long the bar takes to fade in and out. Deliberately
	// short: it should read as a quick fade, not an animation. The loop renders
	// every idleFrame (50 ms), so this spans only a handful of frames — going
	// much below that would quantize into a visible step or two.
	controlsFade = 150 * time.Millisecond
)

// Bar colors: the filled portion is a bright blue, the remainder a faint white
// so the track hints at the full length without competing with the filled part,
// and stays legible over both dark and bright video. Both sliders use the same
// blue — they are the same kind of control, and their differing widths and
// positions in the row are what tell them apart.
var (
	filledColor = color.NRGBA{R: 0x3d, G: 0x8b, B: 0xff, A: 0xff}
	trackColor  = color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0x22}
	knobColor   = color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
	iconColor   = color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xf0}
)

// The control icons. The Go fonts carry no media glyphs at all (U+25B6 ▶,
// U+23F8 ⏸ and U+1F50A 🔊 are all absent, verified against gofont.Collection),
// so these are vector paths from the material design set rather than text.
// mustIcon is safe at init: the data is compiled in, so a decode failure is a
// build bug, not a runtime condition. The volume glyphs live in volume.go, with
// the state they depend on.
var (
	iconPlay  = mustIcon(icons.AVPlayArrow)
	iconPause = mustIcon(icons.AVPause)
)

func mustIcon(data []byte) *widget.Icon {
	ic, err := widget.NewIcon(data)
	if err != nil {
		panic(fmt.Sprintf("decode embedded icon: %v", err))
	}
	return ic
}

// playPauseIcon is the glyph for the play button: a play triangle while paused
// (it shows what the click will do), a pause bar while playing.
func playPauseIcon(paused bool) *widget.Icon {
	if paused {
		return iconPlay
	}
	return iconPause
}

// progressFraction is how much of the file has played, in [0,1]. A non-positive
// duration (live stream, or mpv not knowing it yet) reads as empty rather than
// dividing by zero — there is no meaningful fraction of an unknown length.
func progressFraction(pos, dur float64) float32 {
	if dur <= 0 {
		return 0
	}
	f := pos / dur
	if f < 0 {
		return 0
	}
	if f > 1 {
		return 1
	}
	return float32(f)
}

// seekTarget converts a knob position in [0,1] into an absolute time in seconds,
// clamped into the file. An unknown duration has no position to seek to.
func seekTarget(frac float32, dur float64) float64 {
	if dur <= 0 {
		return 0
	}
	t := float64(frac) * dur
	if t < 0 {
		return 0
	}
	if t > dur {
		return dur
	}
	return t
}

// seekAbsoluteCommand builds the mpv command for a scrub to an absolute
// position. keyframes (rather than exact) keeps a drag responsive: mpv snaps to
// the nearest keyframe instead of decoding up to an exact frame for every
// intermediate position the knob passes through.
func seekAbsoluteCommand(sec float64) []string {
	return []string{"seek", fmt.Sprintf("%.3f", sec), "absolute+keyframes"}
}

// controlsVisible reports whether the control bar should be drawn at now, given
// when pointer input last arrived. A zero lastInput means the pointer has not
// moved yet, so nothing has asked for the bar. dragging holds the bar up
// regardless, so a slow scrub cannot make it vanish under the pointer. The
// window extends controlsFade past the hide delay so the fade-out is drawn
// rather than cut off mid-ramp.
func controlsVisible(now, lastInput time.Time, dragging bool) bool {
	if dragging {
		return true
	}
	if lastInput.IsZero() {
		return false
	}
	return now.Sub(lastInput) < controlsHideAfter+controlsFade
}

// controlsShowing reports whether the bar is up *and staying up* at now: inside
// the hold window, rather than already on its way out. This is the predicate the
// progress toggle decides on, not controlsVisible — a bar mid-fade-out is
// leaving anyway, so the key should bring it back instead of dismissing it a
// second time and appearing to do nothing.
func controlsShowing(now, lastInput time.Time, dragging bool) bool {
	if dragging {
		return true
	}
	if lastInput.IsZero() {
		return false
	}
	return now.Sub(lastInput) < controlsHideAfter
}

// controlsAlpha is the control bar's opacity in [0,1] at now: it ramps up over
// controlsFade from revealedAt, holds at 1 until controlsHideAfter past
// lastInput, then ramps back down. dragging pins it opaque so a slow scrub
// cannot fade the bar out under the pointer, and a zero lastInput (no pointer
// input yet) is fully transparent.
func controlsAlpha(now, lastInput, revealedAt time.Time, dragging bool) float32 {
	if dragging {
		return 1
	}
	if lastInput.IsZero() {
		return 0
	}
	if idle := now.Sub(lastInput); idle >= controlsHideAfter {
		return clamp01(1 - float32(idle-controlsHideAfter)/float32(controlsFade))
	}
	return clamp01(float32(now.Sub(revealedAt)) / float32(controlsFade))
}

func clamp01(f float32) float32 {
	if f < 0 {
		return 0
	}
	if f > 1 {
		return 1
	}
	return f
}

// fadeColor scales c's alpha by f, for painting the bar mid-fade.
func fadeColor(c color.NRGBA, f float32) color.NRGBA {
	c.A = uint8(clamp01(f) * float32(c.A))
	return c
}

// revealControls brings the control bar up for controlsHideAfter. Called from
// the GLFW cursor and button callbacks on any pointer activity, and from the
// seek actions — for which the bar *is* the progress readout, since its knob
// already follows playback every frame.
//
// revealedAt is the fade-in's origin, and is only moved back far enough to
// preserve the alpha the bar already has: re-revealing a half-faded bar resumes
// from where it is instead of flickering through transparent, and a continuous
// stream of mouse moves does not restart the fade-in on every event.
func (p *Player) revealControls(now time.Time) {
	a := controlsAlpha(now, p.lastInput, p.revealedAt, false)
	p.revealedAt = now.Add(-time.Duration(a * float32(controlsFade)))
	p.lastInput = now
}

// hideControls sends the control bar away, fading it out from wherever it is
// rather than cutting it. It is the mirror of revealControls: instead of moving
// revealedAt back by the alpha the bar already has, it moves lastInput back past
// controlsHideAfter by the same amount, so the fade-out resumes at the current
// alpha instead of jumping to opaque and starting over. Zeroing lastInput would
// hide the bar too — but instantly, and a later reveal would then have to fade
// in from nothing.
func (p *Player) hideControls(now time.Time) {
	a := controlsAlpha(now, p.lastInput, p.revealedAt, false)
	p.lastInput = now.Add(-controlsHideAfter - time.Duration((1-a)*float32(controlsFade)))
}

// scrub seeks to the position the knob was dragged to. Called while the drag is
// in progress, so mpv follows the knob rather than waiting for the release.
func (p *Player) scrub(frac float32) {
	if p.mpv == nil || p.idle.Load() { // unit tests build a Player without mpv
		return
	}
	dur := p.playbackDur()
	if dur <= 0 { // live stream: no position to scrub to
		return
	}
	if err := p.mpv.Command(seekAbsoluteCommand(seekTarget(frac, dur))); err != nil {
		p.log.Error("mpv scrub", "fraction", frac, "err", err)
	}
}

// barDragging reports whether either of the bar's sliders is being held. Both
// pin the bar visible and opaque, so a slow drag on either cannot make it fade
// out from under the pointer.
func (p *Player) barDragging() bool {
	return p.progress.Dragging() || p.volume.Dragging()
}

// updateControls consumes the bar's pending pointer events and applies them:
// button clicks, and the two sliders' drags. Split out of layoutControls so the
// wiring can be exercised without the bar's flex geometry.
//
// Two orderings are load-bearing. Clicks are drained *before* Layout, because
// Clickable.Layout consumes pending click events and a Clicked check after it
// never fires (see menu.go). And each slider's Update runs before its Value is
// read, so a drag acts on the position the pointer is actually at; only when a
// slider is not being dragged does its knob follow mpv instead.
func (p *Player) updateControls(gtx layout.Context) {
	if p.playBtn.Clicked(gtx) {
		p.togglePause()
	}
	if p.volumeBtn.Clicked(gtx) {
		p.toggleMute()
	}

	if p.progress.Update(gtx) {
		p.scrub(p.progress.Value)
	} else if !p.progress.Dragging() {
		p.syncProgressKnob()
	}

	if p.volume.Update(gtx) {
		p.setVolume(p.volume.Value)
	} else if !p.volume.Dragging() {
		p.syncVolumeKnob()
	}
}

// layoutControls draws the bottom control bar: play button, progress bar with a
// draggable knob, volume button and volume slider. Nothing is drawn once the
// pointer has been still for controlsHideAfter.
func (p *Player) layoutControls(gtx layout.Context) {
	dragging := p.barDragging()
	if !controlsVisible(gtx.Now, p.lastInput, dragging) {
		return
	}
	alpha := controlsAlpha(gtx.Now, p.lastInput, p.revealedAt, dragging)

	p.updateControls(gtx)

	layout.S.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{
			Left: barInset, Right: barInset, Bottom: barInset,
		}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			// The row spans the window width, so the bar between the two
			// buttons grows and shrinks with the viewport.
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return p.controlButton(gtx, &p.playBtn, playPauseIcon(p.paused), "Play / Pause", alpha)
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return p.layoutProgressBar(gtx, alpha)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return p.controlButton(gtx, &p.volumeBtn, volumeIcon(p.volumeLevel(), p.muted), "Mute", alpha)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					// Rigid at a fixed width (see volumeBarWidth); layoutSlider
					// sizes itself from Max.X, and works its own height out.
					gtx.Constraints.Max.X = gtx.Dp(volumeBarWidth)
					return p.layoutVolumeBar(gtx, alpha)
				}),
			)
		})
	})
}

// controlButton draws one icon button in the bar: no background plate, so only
// the glyph sits over the video. alpha fades the glyph with the rest of the bar.
func (p *Player) controlButton(gtx layout.Context, btn *widget.Clickable, icon *widget.Icon, desc string, alpha float32) layout.Dimensions {
	b := material.IconButton(p.theme, btn, icon, desc)
	b.Background = color.NRGBA{} // transparent: the glyph alone reads as the control
	b.Color = fadeColor(iconColor, alpha)
	b.Size = buttonGlyph
	b.Inset = layout.UniformInset(buttonPad)
	return b.Layout(gtx)
}

// layoutProgressBar draws the seek slider, spanning whatever width the flex row
// gives it.
func (p *Player) layoutProgressBar(gtx layout.Context, alpha float32) layout.Dimensions {
	return p.layoutSlider(gtx, &p.progress, alpha)
}

// layoutVolumeBar draws the volume slider. Identical to the seek bar down to the
// fill colour — same widget, so a drag behaves the same way; only its width and
// its place in the row set it apart.
func (p *Player) layoutVolumeBar(gtx layout.Context, alpha float32) layout.Dimensions {
	return p.layoutSlider(gtx, &p.volume, alpha)
}

// layoutSlider draws the track, the filled portion, and the knob for f, and
// registers its drag area. The grab radius (not the smaller drawn radius) is
// reserved at both ends, so the knob's travel matches its hit area and the circle
// stays inside the row at 0% and 100%. alpha fades the whole slider.
func (p *Player) layoutSlider(gtx layout.Context, f *widget.Float, alpha float32) layout.Dimensions {
	kr := gtx.Dp(knobRadius)
	kg := gtx.Dp(knobGrab)
	th := gtx.Dp(trackHeight)
	// Row height: the full grab diameter, so the pointer does not have to hit a
	// 2 px line or the small circle drawn on it.
	height := 2 * kg
	width := gtx.Constraints.Max.X
	trackW := width - 2*kg
	if trackW < 1 {
		trackW = 1
	}

	// The drag area spans the track and is inset by the grab radius, so
	// Float.Value maps 0..1 onto the knob's own travel.
	dragGtx := gtx
	dragGtx.Constraints.Min = image.Pt(trackW, height)
	off := op.Offset(image.Pt(kg, 0)).Push(gtx.Ops)
	f.Layout(dragGtx, layout.Horizontal, knobGrab)
	off.Pop()

	// Track and filled portion, vertically centered in the row.
	top := (height - th) / 2
	full := image.Rect(kg, top, kg+trackW, top+th)
	paint.FillShape(gtx.Ops, fadeColor(trackColor, alpha), clip.Rect(full).Op())

	filled := int(f.Value * float32(trackW))
	if filled > 0 {
		paint.FillShape(gtx.Ops, fadeColor(filledColor, alpha),
			clip.Rect(image.Rect(kg, top, kg+filled, top+th)).Op())
	}

	// Knob: a white circle centered on the filled/remaining boundary.
	cx := kg + filled
	knob := image.Rect(cx-kr, height/2-kr, cx+kr, height/2+kr)
	paint.FillShape(gtx.Ops, fadeColor(knobColor, alpha), clip.Ellipse(knob).Op(gtx.Ops))

	return layout.Dimensions{Size: image.Pt(width, height)}
}

// syncProgressKnob moves the knob to the current playback position. It reads the
// observed values rather than asking mpv: this runs on every frame the bar is
// visible, and a synchronous property read from the render thread blocks on
// mpv's 200 ms render timeout after a seek (see playback.go).
func (p *Player) syncProgressKnob() {
	p.progress.Value = progressFraction(p.playbackPos(), p.playbackDur())
}
