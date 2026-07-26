package player

import (
	"image/color"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget/material"
	mpv "github.com/gen2brain/go-mpv"
)

// propStr reads a string property, returning "" on error (rendered as "—").
func (p *Player) propStr(name string) string {
	v, err := p.mpv.GetProperty(name, mpv.FormatString)
	if err != nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// propFloat reads a numeric property as float64, 0 on error.
func (p *Player) propFloat(name string) float64 {
	v, err := p.mpv.GetProperty(name, mpv.FormatDouble)
	if err != nil {
		return 0
	}
	if f, ok := v.(float64); ok {
		return f
	}
	return 0
}

// propInt reads an integer property as int64, 0 on error.
func (p *Player) propInt(name string) int64 {
	v, err := p.mpv.GetProperty(name, mpv.FormatInt64)
	if err != nil {
		return 0
	}
	if n, ok := v.(int64); ok {
		return n
	}
	return 0
}

// mediaInfo collects the current file's properties from mpv. The only place
// that reads them; the formatting works off the returned value.
func (p *Player) mediaInfo() mediaInfo {
	return mediaInfo{
		path:     p.propStr("path"),
		filename: p.propStr("filename"),
		// Observed, not read: the duration mpv reports through the pump
		// (invariant 6), the same value the control bar uses.
		duration: p.playbackDur(),
		fileSize: p.propInt("file-size"),

		width:        p.propInt("width"),
		height:       p.propInt("height"),
		fps:          p.propFloat("container-fps"),
		videoBitrate: p.propFloat("video-bitrate"),

		audioBitrate:    p.propFloat("audio-bitrate"),
		audioChannels:   p.propStr("audio-params/channel-count"),
		audioSampleRate: p.propFloat("audio-params/samplerate"),

		tracks: p.trackList(),
	}
}

// infoSections reads the current media properties from mpv and groups them for
// display.
func (p *Player) infoSections() []infoSection {
	return infoSections(p.mediaInfo())
}

// sectionTitleColor sets section headings apart from the label column, which
// rowGrid draws in grey.
var sectionTitleColor = color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}

// layoutInfo draws the info overlay in the top-left corner: one titled block
// per section, in the order infoSections returns them.
func (p *Player) layoutInfo(gtx layout.Context) layout.Dimensions {
	children := make([]layout.FlexChild, 0, 2*len(p.infoCache))
	for i, sec := range p.infoCache {
		sec, i := sec, i
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: sectionGap(i)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				title := material.Body1(p.theme, sec.title)
				title.Color = sectionTitleColor
				return title.Layout(gtx)
			})
		}))
		rows := make([][2]string, len(sec.rows))
		for j, r := range sec.rows {
			rows[j] = [2]string{r.label, r.value}
		}
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return p.rowGrid(gtx, rows)
			})
		}))
	}
	return layout.NW.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(unit.Dp(20)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return p.drawPanel(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
			})
		})
	})
}

// sectionGap is the space above a section heading: none for the first, which
// drawPanel's padding already separates from the panel edge.
func sectionGap(index int) unit.Dp {
	if index == 0 {
		return 0
	}
	return unit.Dp(8)
}
