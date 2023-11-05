package main

import (
	"fmt"
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"github.com/theHilikus/daily/internal/ui"
)

func main() {
	app := app.New()
	window := app.NewWindow("Clickable text demo")
	window.Resize(fyne.NewSize(400, 600))

	sample := ui.NewClickableText("Test", fyne.TextStyle{Bold: true}, color.Black)
	sample.TapHandler = func(pe *fyne.PointEvent) {
		fmt.Println("Tapppped")
	}
	window.SetContent(sample)
	window.ShowAndRun()
}
