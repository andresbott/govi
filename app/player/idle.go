package player

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"log/slog"
	"os"
	"path/filepath"

	_ "embed"
	_ "image/jpeg" // decoders for the embedded asset and user overrides
	_ "image/png"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget"
)

// The embedded idle art is a JPEG: at 1456x832 it is the largest asset in the
// binary, and lossy compression keeps it ~7x smaller than the PNG equivalent
// with no visible difference once scaled up to a high-DPI viewport.
//
//go:embed assets/idle.jpg
var embeddedIdleJPEG []byte

// The govi logo drawn over the idle art. Unlike the backdrop this stays a PNG:
// it has an alpha channel, and JPEG would composite the transparency to black
// and leave ringing around the hard edges.
//
//go:embed assets/logo.png
var embeddedLogoPNG []byte

// decodeImage decodes JPEG or PNG bytes into an image, so a user-supplied
// placeholder may be either format.
func decodeImage(b []byte) (image.Image, error) {
	img, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
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
	img, err := decodeImage(b)
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
	img, err := decodeImage(embeddedIdleJPEG)
	if err != nil {
		// The embedded asset is compiled in; a failure here is a build bug.
		log.Error("embedded placeholder failed to decode", "err", err)
		return image.NewNRGBA(image.Rect(0, 0, 1, 1))
	}
	return img
}

// loadLogo returns the govi logo drawn on the idle screen. It has no config
// override — unlike the backdrop it is app identity, not decoration.
func loadLogo(log *slog.Logger) image.Image {
	img, err := decodeImage(embeddedLogoPNG)
	if err != nil {
		// The embedded asset is compiled in; a failure here is a build bug.
		log.Error("embedded logo failed to decode", "err", err)
		return image.NewNRGBA(image.Rect(0, 0, 1, 1))
	}
	return img
}

// idleLogoFraction is how much of the viewport's shorter side the logo spans.
// Small enough to read as a mark on a backdrop rather than as content, and it
// scales with the window so it holds on both a small window and a 5K display.
const idleLogoFraction = 0.28

// idleLogoOpacity draws the logo at 70% strength so it settles into the backdrop
// instead of sitting on top of it. This is a layer opacity, not a flat alpha: it
// scales the logo's own transparency proportionally, leaving its soft outline
// soft instead of punching a uniform value through it.
const idleLogoOpacity = 0.7

// idleScrim darkens the idle art so the overlays, control bar and logo drawn on
// top of it stay readable. 0xb3 alpha is 70% black: the art stays legible but
// reads as a backdrop rather than competing with the chrome over it.
var idleScrim = color.NRGBA{A: 0xb3}

// layoutIdle draws the idle screen: the placeholder image filling the whole
// viewport, a half-transparent black scrim over it, and the logo centered on top.
//
// widget.Cover scales the image to cover the viewport and crops the overflowing
// axis, so the art always bleeds to every edge whatever the window's aspect
// ratio. Scale stays at 1 (not 1/PxPerDp) because Cover derives its factor from
// the constraints alone — a dp-corrected scale would only change which side
// gets cropped.
func (p *Player) layoutIdle(gtx layout.Context) layout.Dimensions {
	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			return widget.Image{
				Src:      p.placeholderSrc,
				Fit:      widget.Cover,
				Position: layout.Center,
			}.Layout(gtx)
		}),
		// Expanded, not Stacked: the scrim takes its size from the art rather
		// than growing the stack, so it covers exactly the viewport.
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			size := gtx.Constraints.Min
			paint.FillShape(gtx.Ops, idleScrim, clip.Rect{Max: size}.Op())
			return layout.Dimensions{Size: size}
		}),
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, p.layoutIdleLogo)
		}),
	)
}

// layoutIdleLogo draws the logo at a fixed fraction of the viewport's shorter
// side. The box is square and the source is square, so Contain neither crops nor
// letterboxes; sizing off the shorter side keeps the mark fully visible in a
// wide window and in a tall one alike.
func (p *Player) layoutIdleLogo(gtx layout.Context) layout.Dimensions {
	side := min(gtx.Constraints.Max.X, gtx.Constraints.Max.Y)
	box := int(float32(side) * idleLogoFraction)
	gtx.Constraints = layout.Exact(image.Pt(box, box))
	defer paint.PushOpacity(gtx.Ops, idleLogoOpacity).Pop()
	return widget.Image{
		Src:      p.logoSrc,
		Fit:      widget.Contain,
		Position: layout.Center,
	}.Layout(gtx)
}
