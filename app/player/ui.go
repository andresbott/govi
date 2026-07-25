package player

import (
	"gioui.org/layout"
)

// layoutUI draws the control overlay on top of the video. Playback controls
// are keyboard/menu driven; see "On-video widgets" in docs/agents/player.md
// for the pattern to draw clickable widgets over the video.
func (p *Player) layoutUI(gtx layout.Context) layout.Dimensions {
	if p.idle.Load() {
		dims := p.layoutIdle(gtx)
		// Keys keep dispatching while idle, so info is reachable too (its
		// fields render "—" with no file loaded); only confirm is truly
		// unreachable while idle (askDeleteFile requires a current path).
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
