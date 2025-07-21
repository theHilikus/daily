package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

type ClickableText struct {
	widget.BaseWidget

	text          *widget.RichText
	background    *canvas.Rectangle
	rootContainer *fyne.Container
	tapAnim       *fyne.Animation

	OnTapped func(*fyne.PointEvent)
}

func NewClickableText(text string, style fyne.TextStyle, colour fyne.ThemeColorName) *ClickableText {
	richText := widget.NewRichText(&widget.TextSegment{
		Text: text,
		Style: widget.RichTextStyle{
			TextStyle: style,
			ColorName: colour,
		},
	})
	richText.Wrapping = fyne.TextWrapWord

	result := &ClickableText{
		text:       richText,
		background: canvas.NewRectangle(color.Transparent),
	}
	result.ExtendBaseWidget(result)
	result.rootContainer = container.NewStack(result.background, result.text)
	result.tapAnim = newTapAnimation(result.background, result)
	result.tapAnim.Curve = fyne.AnimationEaseOut

	return result
}

func (clickable *ClickableText) Tapped(event *fyne.PointEvent) {
	clickable.tapAnimation()
	if clickable.OnTapped != nil {
		clickable.OnTapped(event)
	}
}

func (clickable *ClickableText) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(clickable.rootContainer)
}

func (clickable *ClickableText) MouseIn(*desktop.MouseEvent) {
	clickable.changeBackground(theme.Color(theme.ColorNameHover))
}

func (clickable *ClickableText) MouseMoved(*desktop.MouseEvent) {
}

func (clickable *ClickableText) MouseOut() {
	clickable.changeBackground(color.Transparent)
}

func (clickable *ClickableText) changeBackground(colour color.Color) {
	if clickable.background == nil {
		return
	}

	clickable.background.FillColor = colour

	clickable.background.CornerRadius = theme.InputRadiusSize()
	clickable.background.Refresh()
}

func newTapAnimation(bg *canvas.Rectangle, w fyne.Widget) *fyne.Animation {
	return fyne.NewAnimation(canvas.DurationStandard, func(done float32) {
		mid := w.Size().Width / 2
		size := mid * done
		bg.Resize(fyne.NewSize(size*2, w.Size().Height))
		bg.Move(fyne.NewPos(mid-size, 0))

		r, g, bb, a := theme.Color(theme.ColorNamePressed).RGBA()
		aa := uint8(a)
		fade := aa - uint8(float32(aa)*done)
		bg.FillColor = &color.NRGBA{R: uint8(r), G: uint8(g), B: uint8(bb), A: fade}
		canvas.Refresh(bg)
	})
}

func (clickable *ClickableText) tapAnimation() {
	if clickable.tapAnim == nil {
		return
	}
	clickable.tapAnim.Stop()

	if fyne.CurrentApp().Settings().ShowAnimations() {
		clickable.tapAnim.Start()
	}
}
