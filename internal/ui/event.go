package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

type Event struct {
	widget.BaseWidget

	Id           string // Unique ID for the event
	Title        *ClickableText
	TitleButtons []*widget.Button
	Detail       fyne.CanvasObject
	open         bool
	container    *fyne.Container
}

func NewEvent(id string, icon *widget.Icon, title *ClickableText, titleButtons []*widget.Button, detail fyne.CanvasObject) *Event {
	var titleBox *fyne.Container
	var rightComponent fyne.CanvasObject
	if len(titleButtons) > 0 {
		objects := make([]fyne.CanvasObject, len(titleButtons))
		for i, button := range titleButtons {
			objects[i] = button
		}
		rightComponent = container.NewHBox(objects...)
	}

	if icon == nil {
		titleBox = container.NewBorder(nil, nil, nil, rightComponent, title)
	} else {
		titleBox = container.NewBorder(nil, nil, icon, rightComponent, title)
	}

	detail.Hide()
	rootContainer := container.NewVBox(container.NewPadded(titleBox), detail, widget.NewSeparator())
	result := &Event{
		Id:           id,
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

func (event *Event) IsOpen() bool {
	return event.open
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
