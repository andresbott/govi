package player

import (
	"fmt"
	"image"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/font/gofont"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

// barHarness drives the control bar's widgets through Gio without a display: a
// router and an op list are all the pointer plumbing needs, and no GPU is
// involved because nothing is ever rendered. It mirrors the real frame order in
// player.go — lay out, then router.Frame — which is what makes queued pointer
// events reach the widgets on the following frame.
type barHarness struct {
	p      *Player
	router input.Router
	ops    op.Ops
	size   image.Point
	body   func(gtx layout.Context)
}

func newBarHarness(t *testing.T, p *Player, body func(gtx layout.Context)) *barHarness {
	t.Helper()
	p.theme = material.NewTheme()
	p.theme.Shaper = text.NewShaper(text.WithCollection(gofont.Collection()))
	return &barHarness{p: p, size: image.Pt(400, 200), body: body}
}

func (h *barHarness) frame() {
	h.ops.Reset()
	gtx := layout.Context{
		Ops:         &h.ops,
		Now:         time.Now(),
		Source:      h.router.Source(),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(h.size),
	}
	h.body(gtx)
	h.router.Frame(&h.ops)
}

// press queues a primary press at pt (framebuffer pixels) and runs the frame
// that consumes it.
func (h *barHarness) press(pt f32.Point) {
	h.router.Queue(pointer.Event{
		Kind:     pointer.Press,
		Source:   pointer.Mouse,
		Position: pt,
		Buttons:  pointer.ButtonPrimary,
	})
	h.frame()
}

// click completes a full primary click at pt: a move first, because a Clickable
// only reports a click when the release lands while it is hovered, and each event
// on its own frame, which is how they arrive from the GLFW callbacks. The
// trailing frame is the one where the widget's Clicked is drained.
func (h *barHarness) click(pt f32.Point) {
	for i, e := range []pointer.Event{
		{Kind: pointer.Move, Source: pointer.Mouse, Position: pt},
		{Kind: pointer.Press, Source: pointer.Mouse, Position: pt, Buttons: pointer.ButtonPrimary},
		{Kind: pointer.Release, Source: pointer.Mouse, Position: pt},
	} {
		// Distinct timestamps: the gesture counts presses closer than its
		// double-click window as one multi-click.
		e.Time = time.Duration(i+1) * 100 * time.Millisecond
		h.router.Queue(e)
		h.frame()
	}
	h.frame()
}

// center is the middle of the harness frame, for a widget laid out across all of
// it.
func (h *barHarness) center() f32.Point {
	return f32.Pt(float32(h.size.X)/2, float32(h.size.Y)/2)
}

// barRowMid is the vertical middle of the real control bar's row. The bar sits
// bottom-aligned inset by barInset, and the row's height comes from the icon
// buttons (the tallest children), against which the sliders are centered. At 1 px
// per dp in the harness the dp values are pixels.
func (h *barHarness) barRowMid() float32 {
	rowH := float32(buttonGlyph) + 2*float32(buttonPad)
	return float32(h.size.Y) - float32(barInset) - rowH/2
}

// sliderMid is the middle of a slider laid out at the top of the harness frame:
// halfway along its width, and inside the row, which is only 2*knobGrab tall (the
// harness runs at 1 px per dp, so knobGrab is its own value in pixels).
func (h *barHarness) sliderMid() f32.Point {
	return f32.Pt(float32(h.size.X)/2, float32(knobGrab))
}

// volumeSliderHarness lays the volume slider out over the whole frame and runs
// the bar's real pointer wiring (updateControls) against it, so a press lands at
// a known fraction of the slider's travel without the test having to reproduce
// the bar's flex geometry.
func volumeSliderHarness(t *testing.T, p *Player) *barHarness {
	t.Helper()
	return newBarHarness(t, p, func(gtx layout.Context) {
		p.updateControls(gtx)
		p.layoutVolumeBar(gtx, 1)
	})
}

// A press halfway along the volume slider sets mpv's volume to half the range.
// This is the wiring that matters: the slider drives the volume, not a seek.
func TestPressingTheVolumeSliderSetsTheVolume(t *testing.T) {
	p := headlessPlayer(t)
	h := volumeSliderHarness(t, p)
	h.frame() // first frame registers the widget with the router

	h.press(h.sliderMid())

	if got, want := p.volume.Value, float32(0.5); got < want-0.1 || got > want+0.1 {
		t.Fatalf("volume.Value = %v after a press mid-slider, want about %v", got, want)
	}
	if got := p.propInt("volume"); got < 40 || got > 60 {
		t.Errorf("mpv volume = %v after a press mid-slider, want about 50", got)
	}
}

// The two sliders share the bar, so a press on the volume one must leave the
// playback knob alone — a mix-up would jump playback on a volume change.
func TestPressingTheVolumeSliderLeavesTheProgressKnobAlone(t *testing.T) {
	p := headlessPlayer(t)
	p.notePlaybackProp("time-pos", 30.0)
	p.notePlaybackProp("duration", 120.0)
	h := volumeSliderHarness(t, p)
	h.frame()

	h.press(h.sliderMid())

	if got, want := p.progress.Value, float32(0.25); got != want {
		t.Errorf("progress.Value = %v after a volume press, want %v (the observed position)", got, want)
	}
}

// Between drags the volume knob follows mpv, the way the playback knob follows
// the position: a level changed by the shortcuts or the menu shows up on the
// slider without a pointer ever touching it.
func TestTheVolumeKnobFollowsMpvBetweenDrags(t *testing.T) {
	p := headlessPlayer(t)
	h := volumeSliderHarness(t, p)
	p.noteVolumeProp(40.0)

	h.frame()

	if got, want := p.volume.Value, float32(0.4); got != want {
		t.Errorf("volume.Value = %v, want %v", got, want)
	}
}

// The volume button toggles mute. Clicks have to be drained before Layout (see
// docs/agents/player.md), which is exactly what a test running the real
// updateControls ahead of the real layout pins.
func TestClickingTheVolumeButtonTogglesMute(t *testing.T) {
	p := headlessPlayer(t)
	h := newBarHarness(t, p, func(gtx layout.Context) {
		p.updateControls(gtx)
		p.controlButton(gtx, &p.volumeBtn, volumeIcon(p.volumeLevel(), p.muted), "Mute", 1)
	})
	h.frame()

	h.click(h.center()) // the button is laid out across the whole frame here

	if !p.muted {
		t.Error("clicking the volume button did not mute")
	}
	// The glyph is the readout; there is deliberately no text overlay.
	if got := volumeIcon(p.volumeLevel(), p.muted); got != iconVolumeOff {
		t.Error("the button still shows a speaker after muting")
	}
	if p.osdVisible(time.Now()) {
		t.Errorf("the mute click flashed %q, want no text overlay", p.osdText)
	}
}

// The volume slider keeps its fixed width whatever the viewport is, while the seek
// bar takes the rest of the row. Measured through the pointer on the real
// layoutControls: the slider is the row's last child, so its midpoint sits a
// constant distance from the window's right edge, and a press there reads 0.5 at
// any window size. A slider that grew with the viewport (Flexed, or a Rigid that
// ignored volumeBarWidth) would map that same point to a different fraction.
func TestTheVolumeSliderKeepsAFixedWidthAcrossViewports(t *testing.T) {
	// Distance from the right edge to the middle of the slider's travel: the bar's
	// inset, then half the slider. At 1 px per dp the dp values are pixels.
	midFromRight := float32(barInset) + float32(volumeBarWidth)/2

	for _, width := range []int{640, 1920} {
		t.Run(fmt.Sprintf("%dpx", width), func(t *testing.T) {
			p := &Player{}
			p.revealControls(time.Now()) // the bar draws nothing until it is revealed
			h := newBarHarness(t, p, func(gtx layout.Context) { p.layoutControls(gtx) })
			h.size = image.Pt(width, 480)
			h.frame()

			h.press(f32.Pt(float32(width)-midFromRight, h.barRowMid()))

			if !p.volume.Dragging() {
				t.Fatalf("no volume drag started at %v px from the right edge — the slider is not where the geometry says", midFromRight)
			}
			if got, want := p.volume.Value, float32(0.5); got < want-0.1 || got > want+0.1 {
				t.Errorf("a press at the slider's midpoint read %v, want about %v — the slider's width tracks the viewport", got, want)
			}
		})
	}
}

// Dragging either slider holds the bar up: a slow volume drag must not make the
// bar fade out from under the pointer, the same guarantee scrubbing has.
func TestDraggingTheVolumeSliderHoldsTheBarUp(t *testing.T) {
	p := headlessPlayer(t)
	h := volumeSliderHarness(t, p)
	h.frame()

	h.press(h.sliderMid())

	if !p.barDragging() {
		t.Fatal("barDragging() is false while the volume slider is held")
	}
	stale := time.Now().Add(-controlsHideAfter - controlsFade - time.Second)
	if !controlsVisible(time.Now(), stale, p.barDragging()) {
		t.Error("the bar hid while the volume slider was being dragged")
	}
}
