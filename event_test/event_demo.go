package main

import (
	"fmt"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/theHilikus/daily/internal/ui"
)

func main() {
	demoApp := app.New()
	window := demoApp.NewWindow("Event Demo")
	window.Resize(fyne.NewSize(400, 600))

	details := widget.TextSegment{
		Text: "Details",
	}

	title := ui.NewClickableText("hello this is a veeeeeeeeeery long liiiiiiiiiiiinnnneeeeeeeeeeee", fyne.TextStyle{Bold: true}, theme.ColorNameForeground)
	button1 := widget.NewButton("but1", func() { fmt.Println("button1") })
	button2 := widget.NewButton("but2", func() { fmt.Println("button2") })

	sample := ui.NewEvent("id1", nil, title, []*widget.Button{button1, button2}, widget.NewRichText(&details))
	window.SetContent(sample)
	window.ShowAndRun()
}
