package player

import (
	"fmt"

	"gioui.org/layout"
	"gioui.org/unit"
	mpv "github.com/gen2brain/go-mpv"
)

// infoRow is one label/value pair in the info overlay.
type infoRow struct {
	label string
	value string
}

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

// dash returns "—" for empty strings so unavailable fields render uniformly.
func dash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

// infoRows reads the current media properties from mpv and formats them.
func (p *Player) infoRows() []infoRow {
	filename := p.propStr("filename")
	path := p.propStr("path")

	width := p.propInt("width")
	height := p.propInt("height")
	res := "—"
	if width > 0 && height > 0 {
		res = fmt.Sprintf("%d×%d", width, height)
		if fps := p.propFloat("container-fps"); fps > 0 {
			res += fmt.Sprintf(" @ %.3g fps", fps)
		}
	}

	size := "—"
	if b := p.propInt("file-size"); b > 0 {
		size = humanBytes(b)
	}

	vbr := humanBitrate(p.propFloat("video-bitrate"))
	abr := humanBitrate(p.propFloat("audio-bitrate"))

	audio := dash(p.propStr("audio-codec-name"))
	if ch := p.propStr("audio-params/channel-count"); ch != "" {
		if sr := p.propFloat("audio-params/samplerate"); sr > 0 {
			audio = fmt.Sprintf("%s, %s ch, %s", audio, ch, humanRate(sr))
		}
	}

	return []infoRow{
		{"File", dash(filename)},
		{"Path", dash(path)},
		{"Size", size},
		{"Resolution", res},
		{"Video bitrate", vbr},
		{"Audio bitrate", abr},
		{"Video codec", dash(p.propStr("video-codec"))},
		{"Audio codec", audio},
	}
}

// layoutInfo draws the info overlay in the top-left corner.
func (p *Player) layoutInfo(gtx layout.Context) layout.Dimensions {
	rows := make([][2]string, len(p.infoCache))
	for i, r := range p.infoCache {
		rows[i] = [2]string{r.label, r.value}
	}
	return layout.NW.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(unit.Dp(20)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return p.drawPanel(gtx, func(gtx layout.Context) layout.Dimensions {
				return p.rowGrid(gtx, rows)
			})
		})
	})
}
