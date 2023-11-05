package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/theHilikus/daily/internal/ui"
)

var (
	currentDay   = time.Now()
	eventsList   *fyne.Container
	testCalendar *bool
)

const dayFormat = "Mon, Jan 02"

func main() {
	handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})
	slog.SetDefault(slog.New(handler))
	slog.Info("Starting app")

	parseArgs()

	app := app.New()
	window := app.NewWindow("Daily")
	window.Resize(fyne.NewSize(400, 600))

	refreshButton := widget.NewButton("Refresh", refresh)
	settingsButton := widget.NewButton("Settings", showSettings)
	toolbar := container.NewHBox(layout.NewSpacer(), refreshButton, settingsButton)

	dayLabel := widget.NewLabel(currentDay.Format(dayFormat))
	dayLabel.TextStyle = fyne.TextStyle{Bold: true}
	dayBar := container.NewHBox(layout.NewSpacer(), dayLabel, layout.NewSpacer())
	topBar := container.NewVBox(toolbar, dayBar)

	eventsList = container.NewVBox()
	refresh()

	previousDay := widget.NewButton("Previous day", func() { changeDay(-1, dayLabel) })
	nextDay := widget.NewButton("Next day", func() { changeDay(1, dayLabel) })
	bottomBar := container.NewHBox(layout.NewSpacer(), previousDay, layout.NewSpacer(), nextDay, layout.NewSpacer())

	content := container.NewBorder(topBar, bottomBar, nil, nil, eventsList)

	window.SetContent(content)
	window.ShowAndRun()
}

func parseArgs() {
	testCalendar = flag.Bool("test-calendar", false, "Whether to use a dummy calendar instead of retrieving events from the real one")
	flag.Parse()
}

func refresh() {
	slog.Info("Refreshing data around date " + currentDay.Format("2006-01-02"))

	events := getEvents()
	for _, event := range events {
		eventText := event.start.Format("3:04-") + event.end.Format("3:04PM ") + event.title
		eventStyle := fyne.TextStyle{}
		eventColour := theme.DefaultTheme().Color(theme.ColorNameForeground, theme.VariantLight)
		if event.isFinished() {
			//past events
			eventColour = theme.DisabledColor()
		} else if event.isStarted() {
			//ongoing events
			timeToEnd := time.Until(event.end)
			if int(timeToEnd.Hours()) > 0 {
				eventText += " (for " + fmt.Sprintf("%dh%02dm", int(timeToEnd.Hours()), int(timeToEnd.Minutes())%60) + " more)"
			} else {
				eventText += " (for " + fmt.Sprintf("%02dm", int(timeToEnd.Minutes())) + " more)"
			}
			eventStyle.Bold = true
		} else {
			//future events
			timeToStart := time.Until(event.start)
			if timeToStart >= 0 {
				if int(timeToStart.Hours()) > 0 {
					eventText += " (in " + fmt.Sprintf("%dh%02dm", int(timeToStart.Hours()), int(timeToStart.Minutes())%60) + ")"
				} else {
					eventText += " (in " + fmt.Sprintf("%02dm", int(timeToStart.Minutes())) + ")"
				}
			}
		}

		title := ui.NewClickableText(eventText, eventStyle, eventColour)
		details := widget.TextSegment{
			Text: event.details,
		}
		eventsList.Add(ui.NewEvent(title, []*widget.Button{}, widget.NewRichText(&details)))
	}
}

func showSettings() {
	slog.Info("Opening settings panel")
}

func changeDay(offset int, dayLabel *widget.Label) {
	slog.Info("Changing day by " + strconv.Itoa(offset))
	currentDay = currentDay.AddDate(0, 0, offset)
	dayLabel.SetText(currentDay.Format(dayFormat))
	slog.Debug("New day is " + currentDay.Format("2006-01-02"))
}

type event struct {
	title    string
	start    time.Time
	end      time.Time
	location string
	details  string
}

func (otherEvent *event) isFinished() bool {
	return otherEvent.end.Before(time.Now())
}

func (otherEvent *event) isStarted() bool {
	now := time.Now()
	return otherEvent.start.Before(now) && otherEvent.end.After(now)
}

func getEvents() []event {
	if *testCalendar {
		return getDummyEvents()
	}
	return nil
}

func getDummyEvents() []event {
	slog.Debug("Returning dummy events")
	now := time.Now()
	start1 := now.Add(-3 * time.Hour)
	end1 := start1.Add(30 * time.Minute)
	result := []event{
		{title: "title1", location: "location1", details: "details1", start: start1, end: end1},
		{title: "title2", location: "http://www.zoom.us/1234", details: "detauls2", start: start1.Add(time.Hour), end: end1.Add(time.Hour)},
		{title: "title3", location: "location3", details: "detauls3", start: now, end: now.Add(30 * time.Minute)},
		{title: "A very long title that that is way\nlonger than the rest", location: "https://www.zoom.us/2345", details: "details4", start: now, end: now.Add(time.Hour)},
		{title: "title5", location: "location5", details: "details5", start: start1.Add(6 * time.Hour), end: time.Now().Add(6*time.Hour + 30*time.Minute)},
		{title: "title6", location: "https://meet.google.com/3456", details: "details6", start: start1.Add(7 * time.Hour), end: time.Now().Add(7*time.Hour + 30*time.Minute)},
	}

	return result
}
