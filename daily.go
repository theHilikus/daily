package main

import (
	"errors"
	"flag"
	"fmt"
	"github.com/theHilikus/daily/internal/notification"
	"log/slog"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"fyne.io/systray"
	"github.com/robfig/cron/v3"
	"github.com/theHilikus/daily/internal/ui"
	"github.com/zalando/go-keyring"
	"google.golang.org/api/googleapi"
)

var (
	displayDay      time.Time
	eventsList      *fyne.Container
	testCalendar    = flag.Bool("test-calendar", false, "Whether to use a dummy calendar instead of retrieving events from the real one")
	verbose         = flag.Bool("verbose", false, "Enable extra debug logs")
	lastFullRefresh time.Time
	lastErrorButton *widget.Button
	settingsWindow  fyne.Window

	eventSource EventSource
	dailyApp    fyne.App
)

const dayFormat = "Mon, Jan 02"

// EventSource An entity that can retrieve calendar events
type EventSource interface {
	// Gets a slice of events for the particular day specified
	getEvents(time.Time, bool) ([]event, bool, error)
}

func main() {
	flag.Parse()
	configureLog()

	slog.Info("Starting app")

	window := buildUi()

	calendarId := dailyApp.Preferences().String("calendar-id")
	if calendarId != "" {
		refresh(true)
	} else {
		slog.Info("Calendar config not found. Starting in Settings UI")
		showSettings(dailyApp)
	}

	window.ShowAndRun()
}

func configureLog() {
	replacer := func(groups []string, attr slog.Attr) slog.Attr {
		if attr.Key == slog.TimeKey {
			t := attr.Value.Time()
			return slog.String("time", t.Format("15:04:05.000"))
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

	lastErrorButton = widget.NewButtonWithIcon("", theme.WarningIcon(), func() {})
	lastErrorButton.Importance = widget.DangerImportance
	lastErrorButton.Hidden = true
	refreshButton := widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), func() { refresh(true) })
	settingsButton := widget.NewButtonWithIcon("", theme.SettingsIcon(), func() { showSettings(dailyApp) })
	toolbar := container.NewHBox(layout.NewSpacer(), lastErrorButton, refreshButton, settingsButton)

	var dayButton *widget.Button
	dayButton = widget.NewButton(displayDay.Format(dayFormat), func() {
		changeDay(time.Now(), dayButton)
	})
	dayButton.Importance = widget.HighImportance
	dayBar := container.NewHBox(layout.NewSpacer(), dayButton, layout.NewSpacer())
	topBar := container.NewVBox(toolbar, dayBar)

	eventsList = container.NewVBox()

	previousDay := widget.NewButtonWithIcon("", theme.NavigateBackIcon(), func() { changeDay(displayDay.AddDate(0, 0, -1), dayButton) })
	nextDay := widget.NewButtonWithIcon("", theme.NavigateNextIcon(), func() { changeDay(displayDay.AddDate(0, 0, 1), dayButton) })
	bottomBar := container.NewHBox(layout.NewSpacer(), previousDay, layout.NewSpacer(), nextDay, layout.NewSpacer())

	content := container.NewBorder(topBar, bottomBar, nil, nil, container.NewVScroll(eventsList))
	window.SetContent(content)

	cronHandler := cron.New()
	_, err := cronHandler.AddFunc("* * * * *", func() {
		fyne.Do(func() {
			refresh(false)
		})
	})
	if err != nil {
		slog.Error("Could not add cron job", "error", err)
	}
	_, err2 := cronHandler.AddFunc("0 0 * * *", func() { changeDay(time.Now(), dayButton) })
	if err2 != nil {
		slog.Error("Could not add cron job", "error", err2)
	}
	cronHandler.Start()

	return window
}

func refresh(forceRetrieve bool) {
	msg := "Refreshing UI for date " + displayDay.Format("2006-01-02") + ". Force retrieve = " + strconv.FormatBool(forceRetrieve)
	if forceRetrieve {
		slog.Info(msg)
	} else {
		slog.Debug(msg)
	}

	expandedState := make(map[string]bool)
	for _, obj := range eventsList.Objects {
		if eventWidget, ok := obj.(*ui.Event); ok {
			if eventWidget.IsOpen() {
				expandedState[eventWidget.Id] = true
			}
		}
	}

	eventsList.RemoveAll()
	events, err := getEvents(forceRetrieve)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			slog.Warn("Not refreshing. No calendar-token found")
			return
		}
		slog.Error("Could not retrieve calendar events", "error", err)

		userErrorMessage := "Could not retrieve calendar events:\n"
		switch e := err.(type) {
		case *googleapi.Error:
			userErrorMessage += e.Message
		case *url.Error:
			userErrorMessage += e.Err.Error()
		default:
			userErrorMessage += err.Error()
		}

		reportUserError(userErrorMessage)
		showNoEvents()
		return
	} else if !lastErrorButton.Hidden {
		reportUserError("") // clear the error
	}

	if len(events) == 0 {
		showNoEvents()
	}

	for pos := range events {
		event := &events[pos]
		eventText := event.start.Format("3:04-") + event.end.Format("3:04PM ")
		eventStyle := fyne.TextStyle{}
		eventColour := theme.ColorNameForeground
		if event.isFinished() {
			//past events
			eventText += event.title
			eventColour = theme.ColorNameDisabled
		} else if event.isStarted() {
			//ongoing events
			timeToEnd := time.Until(event.end)
			eventText += "(" + createUserFriendlyDurationText(timeToEnd) + " left) " + event.title
			eventStyle.Bold = true
		} else {
			//future events
			timeToStart := time.Until(event.start)
			eventText += "(in " + createUserFriendlyDurationText(timeToStart) + ") " + event.title

			if timeToStart.Minutes() <= float64(dailyApp.Preferences().IntWithFallback("notification-time", 1)) {
				if event.notifiable {
					notify(event, timeToStart)
					event.notified = true
				} else {
					slog.Debug("Not notifying for `" + event.title + "` because it is not notifiable")
				}
			}
		}

		if event.recurring {
			eventText += " 🗘"
		}

		var responseIcon *widget.Icon
		switch event.response {
		case needsAction:
			responseIcon = widget.NewIcon(ui.ResourceWarningPng)
		case declined:
			responseIcon = widget.NewIcon(ui.ResourceCancelPng)
		case tentative:
			responseIcon = widget.NewIcon(ui.ResourceQuestionPng)
		case accepted, empty:
			responseIcon = widget.NewIcon(ui.ResourceCheckedPng)
		}

		title := ui.NewClickableText(eventText, eventStyle, eventColour)
		details := widget.TextSegment{
			Text: event.details,
		}
		var buttons []*widget.Button
		if strings.HasPrefix(event.location, "https://") || strings.HasPrefix(event.location, "http://") {
			locationUrl, err := url.Parse(event.location)
			if err == nil {
				meetingButton := widget.NewButtonWithIcon("", theme.MediaVideoIcon(), func() {
					err := dailyApp.OpenURL(locationUrl)
					if err != nil {
						slog.Error("Could not open meeting URL", "error", err)
						return
					}
				})
				if event.isFinished() {
					meetingButton.Disable()
				} else if event.notified || event.isStarted() {
					meetingButton.Importance = widget.HighImportance
				}
				buttons = append(buttons, meetingButton)
			}
		}

		detailsWidget := widget.NewRichText(&details)
		detailsWidget.Wrapping = fyne.TextWrapWord
		eventWidget := ui.NewEvent(event.id, responseIcon, title, buttons, detailsWidget)
		if expandedState[eventWidget.Id] {
			eventWidget.Open()
		}
		eventsList.Add(eventWidget)
	}

	eventsList.Refresh()
}

func reportUserError(errorMessage string) {
	if errorMessage != "" {
		slog.Info("Reporting user error: " + errorMessage)
		lastErrorButton.Hidden = false
		lastErrorButton.OnTapped = func() {
			dialog.ShowError(errors.New(errorMessage), dailyApp.Driver().AllWindows()[0])
		}
	} else {
		slog.Info("Clearing last user error")
		lastErrorButton.Hidden = true
	}
}

func showNoEvents() {
	noEventsLabel := widget.NewLabel("No events today")
	eventsList.Add(layout.NewSpacer())
	eventsList.Add(container.NewCenter(noEventsLabel))
	eventsList.Add(layout.NewSpacer())
}

func createUserFriendlyDurationText(durationRemaining time.Duration) string {
	if int(durationRemaining.Seconds())%60 > 0 {
		//round up
		durationRemaining = durationRemaining.Truncate(time.Minute) + 1*time.Minute
	}
	var result string
	if int(durationRemaining.Hours()) > 0 {
		result = fmt.Sprintf("%dh%dm", int(durationRemaining.Hours()), int(durationRemaining.Minutes())%60)
	} else {
		result = fmt.Sprintf("%dm", int(durationRemaining.Minutes()))
	}

	return result
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

	notification.SendNotification(dailyApp, notifTitle, notifBody)
	event.notifiable = false
}

func showSettings(dailyApp fyne.App) {
	if settingsWindow != nil {
		settingsWindow.Show()
		return
	}

	slog.Info("Opening settings panel")

	settingsWindow = dailyApp.NewWindow("Settings")
	settingsWindow.SetOnClosed(func() {
		settingsWindow = nil
	})

	settingsWindow.Resize(fyne.NewSize(400, 200))
	calendarIdLabel := widget.NewLabel("Calendar ID:")
	calendarIdBox := widget.NewEntry()
	calendarIdBox.Text = "primary"
	var gCalToken string
	connectButton := widget.NewButtonWithIcon("Google Calendar", ui.ResourceGoogleCalendarPng, func() {
		var err error
		gCalToken, err = startGCalOAuthFlow()
		if err != nil {
			dialog.ShowError(err, settingsWindow)
			return
		}
	})

	connectBox := container.NewHBox(connectButton, calendarIdLabel, calendarIdBox)

	saveButton := widget.NewButton("Save", func() {
		err := keyring.Set("theHilikus-daily-app", "calendar-token", gCalToken)
		if err != nil {
			slog.Error("Could not save calendar token", "error", err)
			return
		}
		dailyApp.Preferences().SetString("calendar-id", calendarIdBox.Text)
		slog.Info("Preferences saved")
		settingsWindow.Close()
	})

	versionLabel := widget.NewLabel("Version: " + dailyApp.Metadata().Version + "-" + strconv.Itoa(dailyApp.Metadata().Build))
	content := container.NewVBox(
		widget.NewLabel("Connect to"),
		connectBox,
		layout.NewSpacer(),
		versionLabel,
		saveButton,
	)

	settingsWindow.SetContent(content)
	settingsWindow.Show()
}

func changeDay(newDate time.Time, dayButton *widget.Button) {
	slog.Info("Changing day to " + newDate.Format(dayFormat))
	displayDay = newDate
	dayButton.SetText(displayDay.Format(dayFormat))
	refresh(false)
}

func isOnSameDay(one time.Time, other time.Time) bool {
	year1, month1, day1 := one.Date()
	year2, month2, day2 := other.Date()
	return year1 == year2 && month1 == month2 && day1 == day2
}

type event struct {
	id         string
	title      string
	start      time.Time
	end        time.Time
	location   string
	details    string
	notifiable bool
	notified   bool
	response   responseStatus
	recurring  bool
}

type responseStatus string

const (
	empty       responseStatus = ""
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

func getEvents(forceRetrieve bool) ([]event, error) {
	if eventSource == nil {
		slog.Info("No event source found. Creating one")
		if *testCalendar {
			eventSource = newDummyEventSource()
		} else {
			var err error
			calendarToken, err := keyring.Get("theHilikus-daily-app", "calendar-token")
			if err != nil {
				return nil, err
			}
			if calendarToken == "" {
				return nil, errors.New("empty token")
			}
			eventSource, err = newGoogleCalendarEventSource(calendarToken)
			if err != nil {
				return nil, err
			}
		}
	}

	updateInterval := float64(dailyApp.Preferences().IntWithFallback("calendar-update-interval", 5))
	if !forceRetrieve && time.Since(lastFullRefresh).Minutes() > updateInterval {
		slog.Debug("Overwriting forceRetrieve because update interval elapsed")
		forceRetrieve = true
	}

	events, fullRefreshed, err := eventSource.getEvents(displayDay, forceRetrieve)

	if fullRefreshed {
		lastFullRefresh = time.Now()
	}

	return events, err
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
			{id: "1", title: "past event yesterday with zoom", location: "http://www.zoom.us/1234", details: "Past event", start: start1.Add(-24 * time.Hour), end: time.Now().Add(-24*time.Hour + 30*time.Minute)},
		},
		today: []event{
			{id: "2", title: "past event", location: "location1", details: "details1", start: start1, end: end1, response: accepted},
			{id: "3", title: "past event with zoom meeting", location: "http://www.zoom.us/1234", details: "detauls2", start: start1.Add(time.Hour), end: end1.Add(time.Hour), response: declined},
			{id: "4", title: "current event", location: "location3", details: "detauls3", start: now.Add(-10 * time.Minute), end: now.Add(30 * time.Minute), response: declined, recurring: true},
			{id: "5", title: "A very long current event with zoom meeting that is longer than the rest", location: "https://www.zoom.us/2345", details: "details4", start: now, end: now.Add(time.Minute), response: tentative},
			{id: "6", title: "future event today", location: "location5", details: "details5 with very long text that might need wrapping because it is so long", start: now.Add(1 * time.Minute), end: time.Now().Add(6*time.Hour + 30*time.Minute), response: needsAction},
			{id: "7", title: "future event today with gmeeting", location: "https://meet.google.com/3456", details: "details6", start: now.Add(2 * time.Minute), end: time.Now().Add(7*time.Hour + 30*time.Minute), notifiable: true, response: accepted},
		},
		tomorrow: []event{
			{id: "8", title: "future event tomorrow with gmeeting", location: "https://meet.google.com/3456", details: "Future Event", start: start1.Add(24 * time.Hour), end: time.Now().Add(24*time.Hour + 30*time.Minute)},
		},
	}
}

func (dummy dummyEventSource) getEvents(day time.Time, fullRefresh bool) ([]event, bool, error) {
	slog.Debug("Returning dummy events. Full refresh = " + strconv.FormatBool(fullRefresh))

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

	return result, fullRefresh, nil
}
