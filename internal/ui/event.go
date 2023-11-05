package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
)

type Event struct {
	widget.BaseWidget

	Title        *ClickableText
	TitleButtons []*widget.Button
	Detail       fyne.CanvasObject
	open         bool
	container    *fyne.Container
}

func NewEvent(title *ClickableText, titleButtons []*widget.Button, detail fyne.CanvasObject) *Event {
	titleBox := container.NewHBox(title, layout.NewSpacer())
	for _, button := range titleButtons {
		titleBox.Add(button)
	}

	detail.Hide()
	rootContainer := container.NewVBox(titleBox, detail, widget.NewSeparator())
	result := &Event{
		Title:        title,
		TitleButtons: titleButtons,
		Detail:       detail,
		open:         false,
		container:    rootContainer,
	}
	result.ExtendBaseWidget(result)

	title.OnTapped = func(pe *fyne.PointEvent) {
		if result.open {
			result.Close()
		} else {
			result.Open()
		}
	}

	return result
}

func (event *Event) Close() {
	event.open = false
	event.Detail.Hide()
	event.Refresh()
}

func (event *Event) Open() {
	event.open = true
	event.Detail.Show()
	event.Refresh()
}

func (event *Event) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(event.container)
}
