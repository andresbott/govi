package player

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log/slog"
	"os"
	"path/filepath"

	_ "embed"

	"gioui.org/layout"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

//go:embed assets/idle.png
var embeddedIdlePNG []byte

// decodePNG decodes PNG bytes into an image.
func decodePNG(b []byte) (image.Image, error) {
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("decode png: %w", err)
	}
	return img, nil
}

// loadPlaceholderFile loads the user-configured placeholder at path, reporting
// false (after logging why) for an unset path or any read/decode failure so the
// caller falls back to the embedded image.
func loadPlaceholderFile(path string, log *slog.Logger) (image.Image, bool) {
	if path == "" {
		return nil, false
	}
	b, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		log.Warn("placeholder image read failed, using embedded", "path", path, "err", err)
		return nil, false
	}
	img, err := decodePNG(b)
	if err != nil {
		log.Warn("placeholder image decode failed, using embedded", "path", path, "err", err)
		return nil, false
	}
	return img, true
}

// loadPlaceholder returns the idle-screen image. If path is set and loads, it
// is used; otherwise (empty path or load failure) the embedded image is
// returned. A load failure logs a warning but never fails.
func loadPlaceholder(path string, log *slog.Logger) image.Image {
	if img, ok := loadPlaceholderFile(path, log); ok {
		return img
	}
	img, err := decodePNG(embeddedIdlePNG)
	if err != nil {
		// The embedded asset is compiled in; a failure here is a build bug.
		log.Error("embedded placeholder failed to decode", "err", err)
		return image.NewNRGBA(image.Rect(0, 0, 1, 1))
	}
	return img
}

// layoutIdle draws the idle screen: the placeholder image centered with a
// "drop a video file here" hint below it.
func (p *Player) layoutIdle(gtx layout.Context) layout.Dimensions {
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return widget.Image{
					Src:   p.placeholderSrc,
					Fit:   widget.Contain,
					Scale: 1.0 / gtx.Metric.PxPerDp,
				}.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				lbl := material.Body1(p.theme, "drop a video file here")
				lbl.Color = color.NRGBA{R: 0xcc, G: 0xcc, B: 0xcc, A: 0xff}
				lbl.Alignment = text.Middle
				return lbl.Layout(gtx)
			}),
		)
	})
}
