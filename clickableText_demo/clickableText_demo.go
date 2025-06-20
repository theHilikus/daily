package main

import (
	"fmt"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"github.com/theHilikus/daily/internal/ui"
	"image/color"
)

func main() {
	demoApp := app.New()
	window := demoApp.NewWindow("Clickable text demo")
	window.Resize(fyne.NewSize(400, 600))

	red := color.RGBA{R: 255, A: 255}
	sample := ui.NewClickableText("Test is a very long text meeeeeeeessssssaaaaaagggggggeeeeeeee", fyne.TextStyle{Bold: true}, red)
	sample.OnTapped = func(pe *fyne.PointEvent) {
		fmt.Println("Tapppped")
	}
	window.SetContent(sample)
	window.ShowAndRun()
}
