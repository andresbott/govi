package player

import (
	"image"
	"image/color"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

// panelBG is the translucent dark background shared by all overlays, matching
// the play button's alpha.
var panelBG = color.NRGBA{A: 0xb0}

// Shared panel text colours. The label column is grey so the value column
// reads as the content rather than competing with its own heading.
var (
	panelLabelColor = color.NRGBA{R: 0xaa, G: 0xaa, B: 0xaa, A: 0xff}
	panelValueColor = color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
)

// rowGrid lays out label/value pairs as two columns. Used by the info overlay;
// help has its own three-cell, two-column layout (overlay_help.go).
func (p *Player) rowGrid(gtx layout.Context, rows [][2]string) layout.Dimensions {
	children := make([]layout.FlexChild, 0, len(rows))
	for _, r := range rows {
		r := r
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.X = gtx.Dp(unit.Dp(140))
					lbl := material.Body2(p.theme, r[0])
					lbl.Color = panelLabelColor
					return lbl.Layout(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					lbl := material.Body2(p.theme, r[1])
					lbl.Color = panelValueColor
					return lbl.Layout(gtx)
				}),
			)
		}))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

// drawPanel paints the rounded translucent background sized to content, then
// lays out w inside padding.
func (p *Player) drawPanel(gtx layout.Context, w layout.Widget) layout.Dimensions {
	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			rr := clip.RRect{Rect: image.Rectangle{Max: gtx.Constraints.Min}, SE: 8, SW: 8, NE: 8, NW: 8}
			paint.FillShape(gtx.Ops, panelBG, rr.Op(gtx.Ops))
			return layout.Dimensions{Size: gtx.Constraints.Min}
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(unit.Dp(16)).Layout(gtx, w)
		}),
	)
}
