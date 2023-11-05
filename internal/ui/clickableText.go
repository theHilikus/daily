package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"
)

type ClickableText struct {
	widget.BaseWidget

	text *canvas.Text
	TapHandler func(*fyne.PointEvent)
}

func NewClickableText(text string, style fyne.TextStyle, colour color.Color) *ClickableText {
	size := float32(0)
	if fyne.CurrentApp() != nil { // nil app possible if app not started
		size = fyne.CurrentApp().Settings().Theme().Size("text") // manually name the size to avoid import loop
	}
	result := &ClickableText{
		text: &canvas.Text{
			Text: text,
			TextSize:  size,
			TextStyle: style,
		},
	}
	result.ExtendBaseWidget(result)

	return result
}

func (clickable *ClickableText) Tapped(event *fyne.PointEvent) {
	if clickable.TapHandler != nil {
		clickable.TapHandler(event)
	}
}

func (textWidget *ClickableText) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(textWidget.text)
}
