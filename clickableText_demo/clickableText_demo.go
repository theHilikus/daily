package main

import (
	"fmt"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/theme"
	"github.com/theHilikus/daily/internal/ui"
)

func main() {
	demoApp := app.New()
	window := demoApp.NewWindow("Clickable text demo")
	window.Resize(fyne.NewSize(400, 600))

	sample := ui.NewClickableText("Test is a very long text meeeeeeeessssssaaaaaagggggggeeeeeeee", fyne.TextStyle{Bold: true}, theme.ColorNameForeground)
	sample.OnTapped = func(pe *fyne.PointEvent) {
		fmt.Println("Tapppped")
	}
	window.SetContent(sample)
	window.ShowAndRun()
}
