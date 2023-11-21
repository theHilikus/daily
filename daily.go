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
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"fyne.io/systray"
	"github.com/robfig/cron/v3"
	"github.com/theHilikus/daily/internal/ui"
)

var (
	displayDay   time.Time
	eventsList   *fyne.Container
	testCalendar = flag.Bool("test-calendar", false, "Whether to use a dummy calendar instead of retrieving events from the real one")
	verbose      = flag.Bool("verbose", false, "Enable extra debug logs")

	eventSource EventSource
	dailyApp    fyne.App
)

const dayFormat = "Mon, Jan 02"

// An entity that can retrieve calendar events
type EventSource interface {
	// Gets a slice of events for the particular day specified
	getEvents(time.Time) ([]event, error)
}

func main() {
	flag.Parse()
	configureLog()

	slog.Info("Starting app")

	window := buildUi()

	calendarToken := dailyApp.Preferences().String("calendar-token")
	if calendarToken != "" {
		refresh()
	} else {
		slog.Info("Calendar config not found. Starting in Settings UI")
		showSettings(dailyApp)
	}

	cronHandler := cron.New()
	cronHandler.AddFunc("* * * * *", refresh)
	cronHandler.Start()

	window.ShowAndRun()
}

func configureLog() {
	replacer := func(groups []string, attr slog.Attr) slog.Attr {
		if attr.Key == slog.TimeKey {
			time := attr.Value.Time()
			return slog.String("time", time.Format("15:04:05.000"))
		}
		return attr
	}

	lvl := new(slog.LevelVar)
	lvl.Set(slog.LevelInfo)
	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: lvl, ReplaceAttr: replacer})
	if *verbose {
		lvl.Set(slog.LevelDebug)
	}
	slog.SetDefault(slog.New(handler))
}

func buildUi() fyne.Window {
	displayDay = time.Now()

	dailyApp = app.NewWithID("com.github.theHilikus.daily")
	dailyApp.SetIcon(ui.ResourceAppIconPng)

	window := dailyApp.NewWindow("Daily")
	window.Resize(fyne.NewSize(400, 600))

	if desk, ok := dailyApp.(desktop.App); ok {
		showItem := fyne.NewMenuItem("Show", func() {
			window.Show()
		})
		menu := fyne.NewMenu("Daily Systray Menu", showItem)
		desk.SetSystemTrayMenu(menu)
		systray.SetTitle("Daily")
		window.SetCloseIntercept(func() {
			window.Hide()
		})
	}

	refreshButton := widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), refresh)
	settingsButton := widget.NewButtonWithIcon("", theme.SettingsIcon(), func() { showSettings(dailyApp) })
	toolbar := container.NewHBox(layout.NewSpacer(), refreshButton, settingsButton)

	dayLabel := widget.NewLabel(displayDay.Format(dayFormat))
	dayLabel.TextStyle = fyne.TextStyle{Bold: true}
	dayBar := container.NewHBox(layout.NewSpacer(), dayLabel, layout.NewSpacer())
	topBar := container.NewVBox(toolbar, dayBar)

	eventsList = container.NewVBox()

	previousDay := widget.NewButtonWithIcon("", theme.NavigateBackIcon(), func() { changeDay(displayDay.AddDate(0, 0, -1), dayLabel) })
	nextDay := widget.NewButtonWithIcon("", theme.NavigateNextIcon(), func() { changeDay(displayDay.AddDate(0, 0, 1), dayLabel) })
	bottomBar := container.NewHBox(layout.NewSpacer(), previousDay, layout.NewSpacer(), nextDay, layout.NewSpacer())

	content := container.NewBorder(topBar, bottomBar, nil, nil, eventsList)
	window.SetContent(content)

	return window
}

func refresh() {
	slog.Info("Refreshing UI for date " + displayDay.Format("2006-01-02"))
	eventsList.RemoveAll()
	events, err := getEvents()
	if err != nil {
		slog.Error("Could not retrieve calendar events")
		return
	}

	for pos := range events {
		event := &events[pos]
		eventText := event.start.Format("3:04-") + event.end.Format("3:04PM ") + event.title
		eventStyle := fyne.TextStyle{}
		eventColour := theme.DefaultTheme().Color(theme.ColorNameForeground, theme.VariantLight)
		if event.isFinished() {
			//past events
			eventColour = theme.DisabledColor()
		} else if event.isStarted() {
			//ongoing events
			timeToEnd := time.Until(event.end).Round(time.Minute)
			if int(timeToEnd.Hours()) > 0 {
				eventText += " (" + fmt.Sprintf("%dh%dm", int(timeToEnd.Hours()), int(timeToEnd.Minutes())%60) + " remaining)"
			} else {
				eventText += " (" + fmt.Sprintf("%dm", int(timeToEnd.Minutes())) + " remaining)"
			}
			eventStyle.Bold = true
		} else {
			//future events
			timeToStart := time.Until(event.start).Round(time.Minute)
			if timeToStart >= 0 {
				if int(timeToStart.Hours()) > 0 {
					eventText += " (in " + fmt.Sprintf("%dh%dm", int(timeToStart.Hours()), int(timeToStart.Minutes())%60) + ")"
				} else {
					eventText += " (in " + fmt.Sprintf("%dm", int(timeToStart.Minutes())) + ")"
				}
			}

			if int(timeToStart.Round(time.Minute).Minutes()) <= dailyApp.Preferences().IntWithFallback("notification-time", 1) {
				if event.notifiable {
					notify(event, timeToStart)
				} else {
					slog.Debug("Not notifying for `" + event.title + "` because it is not notifiable")
				}
			}
		}

		switch event.response {
		case needsAction:
			eventText = "🚩 " + eventText
		case declined:
			eventText = "❎ " + eventText
		case tentative:
			eventText = "❓ " + eventText
		case accepted:
			eventText = "✅ " + eventText
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
	eventsList.Refresh()
}

func notify(event *event, timeToStart time.Duration) {
	slog.Debug("Sending notification for '" + event.title + "'. Time to start: " + timeToStart.String())
	remaining := int(timeToStart.Round(time.Minute).Minutes())
	notifTitle := "'" + event.title + "' is starting soon"
	notifBody := strconv.Itoa(remaining) + " minutes to event"
	if remaining == 1 {
		notifBody = strconv.Itoa(remaining) + " minute to event"
	} else if remaining <= 0 {
		notifTitle = "'" + event.title + "' is starting now"
	}
	notification := fyne.NewNotification(notifTitle, notifBody)
	dailyApp.SendNotification(notification)
	event.notifiable = false
}

func showSettings(dailyApp fyne.App) {
	slog.Info("Opening settings panel")
}

func changeDay(newDate time.Time, dayLabel *widget.Label) {
	slog.Info("Changing day to " + newDate.Format(time.RFC3339))
	displayDay = newDate
	dayLabel.SetText(displayDay.Format(dayFormat))
	slog.Debug("New day is " + displayDay.Format("2006-01-02"))
	refresh()
}

func isOnSameDay(one time.Time, other time.Time) bool {
	year1, month1, day1 := one.Date()
	year2, month2, day2 := other.Date()
	return year1 == year2 && month1 == month2 && day1 == day2
}

type event struct {
	title      string
	start      time.Time
	end        time.Time
	location   string
	details    string
	notifiable bool
	response   responseStatus
}

type responseStatus string

const (
	needsAction responseStatus = "needsAction"
	declined    responseStatus = "declined"
	tentative   responseStatus = "tentative"
	accepted    responseStatus = "accepted"
)

func (otherEvent *event) isFinished() bool {
	return otherEvent.end.Before(time.Now())
}

func (otherEvent *event) isStarted() bool {
	now := time.Now()
	return otherEvent.start.Before(now) && otherEvent.end.After(now)
}

func getEvents() ([]event, error) {
	if eventSource == nil {
		slog.Info("No event source found. Creating one")
		if *testCalendar {
			eventSource = newDummyEventSource()
		} else {
			var err error
			eventSource, err = newGoogleCalendar()
			if err != nil {
				return nil, err
			}
		}
	}

	return eventSource.getEvents(displayDay)

}

type dummyEventSource struct {
	originalNow time.Time
	yesterday   []event
	today       []event
	tomorrow    []event
}

func newDummyEventSource() *dummyEventSource {
	now := time.Now().Truncate(time.Minute)
	start1 := now.Add(-3 * time.Hour)
	end1 := start1.Add(30 * time.Minute)
	return &dummyEventSource{
		originalNow: now,
		yesterday: []event{
			{title: "past event yesterday with zoom", location: "http://www.zoom.us/1234", details: "Past event", start: start1.Add(-24 * time.Hour), end: time.Now().Add(-24*time.Hour + 30*time.Minute)},
		},
		today: []event{
			{title: "past event", location: "location1", details: "details1", start: start1, end: end1, response: accepted},
			{title: "past event with zoom meeting", location: "http://www.zoom.us/1234", details: "detauls2", start: start1.Add(time.Hour), end: end1.Add(time.Hour), response: declined},
			{title: "current event", location: "location3", details: "detauls3", start: now.Add(-10 * time.Minute), end: now.Add(30 * time.Minute), response: declined},
			{title: "A very long current event with zoom meeting that is longer than the rest", location: "https://www.zoom.us/2345", details: "details4", start: now, end: now.Add(time.Hour), response: tentative},
			{title: "future event today", location: "location5", details: "details5", start: now.Add(1 * time.Minute), end: time.Now().Add(6*time.Hour + 30*time.Minute), response: needsAction},
			{title: "future event today with gmeeting", location: "https://meet.google.com/3456", details: "details6", start: now.Add(2 * time.Minute), end: time.Now().Add(7*time.Hour + 30*time.Minute), notifiable: true, response: accepted},
		},
		tomorrow: []event{
			{title: "future event tomorrow with gmeeting", location: "https://meet.google.com/3456", details: "Future Event", start: start1.Add(24 * time.Hour), end: time.Now().Add(24*time.Hour + 30*time.Minute)},
		},
	}
}

func (dummy dummyEventSource) getEvents(day time.Time) ([]event, error) {
	slog.Debug("Returning dummy events")

	var result []event
	if isOnSameDay(dummy.originalNow, day) {
		result = dummy.today
	} else if day.Before(dummy.originalNow) {
		//past
		result = dummy.yesterday
	} else {
		//future
		result = dummy.tomorrow
	}

	return result, nil
}
