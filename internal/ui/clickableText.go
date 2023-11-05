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

	text          *canvas.Text
	background    *canvas.Rectangle
	rootContainer *fyne.Container

	TapHandler func(*fyne.PointEvent)
}

func NewClickableText(text string, style fyne.TextStyle, colour color.Color) *ClickableText {
	size := float32(0)
	if fyne.CurrentApp() != nil { // nil app possible if app not started
		size = fyne.CurrentApp().Settings().Theme().Size("text") // manually name the size to avoid import loop
	}
	result := &ClickableText{
		text: &canvas.Text{
			Text:      text,
			TextSize:  size,
			TextStyle: style,
			Color:     colour,
		},
		background: canvas.NewRectangle(color.Transparent),
	}
	result.ExtendBaseWidget(result)
	result.rootContainer = container.NewStack(result.background, result.text)

	return result
}

func (clickable *ClickableText) Tapped(event *fyne.PointEvent) {
	if clickable.TapHandler != nil {
		clickable.TapHandler(event)
	}
}

func (clickable *ClickableText) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(clickable.rootContainer)
}

func (clickable *ClickableText) MouseIn(*desktop.MouseEvent) {
	clickable.changeBackground(theme.HoverColor())
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
