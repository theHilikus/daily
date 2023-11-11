package main

import (
	"flag"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strconv"
	"strings"
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
	displayDay   time.Time
	eventsList   *fyne.Container
	testCalendar *bool
	eventSource  EventSource
	preferences  fyne.Preferences
	dailyApp     fyne.App
)

const dayFormat = "Mon, Jan 02"

type EventSource interface {
	getEvents(time.Time) ([]event, error)
}

func main() {
	handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})
	slog.SetDefault(slog.New(handler))
	slog.Info("Starting app")

	parseArgs()

	displayDay = time.Now()

	dailyApp = app.NewWithID("com.github.theHilikus.daily")

	window := dailyApp.NewWindow("Daily")
	window.Resize(fyne.NewSize(400, 600))

	refreshButton := widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), refresh)
	settingsButton := widget.NewButtonWithIcon("", theme.SettingsIcon(), func() { showSettings(dailyApp) })
	toolbar := container.NewHBox(layout.NewSpacer(), refreshButton, settingsButton)

	dayLabel := widget.NewLabel(displayDay.Format(dayFormat))
	dayLabel.TextStyle = fyne.TextStyle{Bold: true}
	dayBar := container.NewHBox(layout.NewSpacer(), dayLabel, layout.NewSpacer())
	topBar := container.NewVBox(toolbar, dayBar)

	eventsList = container.NewVBox()

	previousDay := widget.NewButtonWithIcon("", theme.NavigateBackIcon(), func() { changeDay(-1, dayLabel) })
	nextDay := widget.NewButtonWithIcon("", theme.NavigateNextIcon(), func() { changeDay(1, dayLabel) })
	bottomBar := container.NewHBox(layout.NewSpacer(), previousDay, layout.NewSpacer(), nextDay, layout.NewSpacer())

	content := container.NewBorder(topBar, bottomBar, nil, nil, eventsList)
	window.SetContent(content)

	preferences = dailyApp.Preferences()
	calendarToken := preferences.String("calendar-token")
	if calendarToken == "" {
		slog.Info("Calendar config not found. Starting in Settings UI")
		showSettings(dailyApp)
	} else {
		refresh()
	}

	window.ShowAndRun()
}

func parseArgs() {
	testCalendar = flag.Bool("test-calendar", false, "Whether to use a dummy calendar instead of retrieving events from the real one")
	flag.Parse()
}

func refresh() {
	slog.Info("Refreshing data around date " + displayDay.Format("2006-01-02"))

	events, err := getEvents()
	if err != nil {
		slog.Error("Could not retrieve calendar events")
	} else {
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
			var buttons []*widget.Button
			if strings.HasPrefix(event.location, "https://") || strings.HasPrefix(event.location, "http://") {
				locationUrl, err := url.Parse(event.location)
				if err == nil {
					meetingButton := widget.NewButtonWithIcon("", theme.MediaVideoIcon(), func() { dailyApp.OpenURL(locationUrl) })
					if event.isFinished() {
						meetingButton.Disable()
					}
					buttons = append(buttons, meetingButton)
				}
			}
			eventsList.Add(ui.NewEvent(title, buttons, widget.NewRichText(&details)))
		}
	}
}

func showSettings(dailyApp fyne.App) {
	slog.Info("Opening settings panel")
}

func changeDay(offset int, dayLabel *widget.Label) {
	slog.Info("Changing day by " + strconv.Itoa(offset))
	displayDay = displayDay.AddDate(0, 0, offset)
	dayLabel.SetText(displayDay.Format(dayFormat))
	slog.Debug("New day is " + displayDay.Format("2006-01-02"))
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

func getEvents() ([]event, error) {
	if *testCalendar {
		return getDummyEvents(), nil
	}

	if eventSource == nil {
		var err error
		eventSource, err = newGoogleCalendar()
		if err != nil {
			return nil, err
		}
	}

	return eventSource.getEvents(displayDay)

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
