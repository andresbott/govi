package player

import (
	"gioui.org/layout"
)

// layoutUI draws the control overlay on top of the video: the auto-hiding
// control bar (controls.go), the status flash, and whichever overlay is open.
// See "Control bar" in docs/agents/player.md for the pattern used to draw
// clickable widgets over the video.
func (p *Player) layoutUI(gtx layout.Context) layout.Dimensions {
	if p.idle.Load() {
		dims := p.layoutIdle(gtx)
		// Keys keep dispatching while idle, so info is reachable too (its
		// fields render "—" with no file loaded), and so is the volume flash;
		// only confirm is truly unreachable while idle (askDeleteFile requires
		// a current path).
		p.layoutOSD(gtx)
		switch p.overlay {
		case overlayMenu:
			p.layoutMenu(gtx)
		case overlayHelp:
			p.layoutHelp(gtx)
		case overlayInfo:
			p.layoutInfo(gtx)
		}
		return dims
	}

	dims := layout.Dimensions{Size: gtx.Constraints.Max}

	// The control bar sits below the flash and the overlays: it is the bottom
	// layer of the chrome, so a panel or a flash wins any pixel they share.
	p.layoutControls(gtx)

	// The status flash is drawn before the overlays: it is transient feedback,
	// so an overlay panel wins any pixel they share.
	p.layoutOSD(gtx)

	switch p.overlay {
	case overlayInfo:
		p.layoutInfo(gtx)
	case overlayHelp:
		p.layoutHelp(gtx)
	case overlayMenu:
		p.layoutMenu(gtx)
	case overlayConfirm:
		p.layoutConfirm(gtx)
	}
	return dims
}
