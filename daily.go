package main

import (
	"errors"
	"flag"
	"fmt"
	"github.com/theHilikus/daily/internal/notification"
	"log/slog"
	"net/url"
	"os"
	"regexp"
	"runtime/debug"
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
	md "github.com/JohannesKaufmann/html-to-markdown/v2"
	"github.com/robfig/cron/v3"
	"github.com/theHilikus/daily/internal/ui"
	"github.com/zalando/go-keyring"
	"google.golang.org/api/googleapi"
)

var (
	displayDay      time.Time
	eventsList      *fyne.Container
	testCalendar    = flag.Bool("test-calendar", false, "Whether to use a dummy calendar instead of retrieving events from the real one")
	debugFlag       = flag.Bool("debug", false, "Enable debug mode")
	lastFullRefresh time.Time
	lastErrorButton *widget.Button
	dayButton       *widget.Button
	settingsWindow  fyne.Window

	currentEventSource EventSource
	dailyApp           fyne.App
	appVersion         string
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
	if *debugFlag {
		lvl.Set(slog.LevelDebug)
	}
	slog.SetDefault(slog.New(handler))
}

func buildUi() fyne.Window {
	dailyApp = app.NewWithID("com.github.theHilikus.daily")
	appVersion = dailyApp.Metadata().Version + "_" + getGitCommit()
	slog.Info("Starting app version " + appVersion)
	dailyApp.SetIcon(ui.ResourceAppIconPng)

	displayDay = time.Now()
	window := dailyApp.NewWindow("Daily")
	width := dailyApp.Preferences().FloatWithFallback("window-width", 400)
	height := dailyApp.Preferences().FloatWithFallback("window-height", 600)
	window.Resize(fyne.NewSize(float32(width), float32(height)))

	createSystray(window)

	toolbar := createToolbar()

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

	startCronJobs()

	return window
}

func getGitCommit() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" {
				return "g" + setting.Value[:7]
			}
		}
	}

	return "unknown"
}

func createSystray(window fyne.Window) {
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
		dailyApp.Lifecycle().SetOnStopped(func() {
			size := window.Canvas().Size()
			dailyApp.Preferences().SetFloat("window-width", float64(size.Width))
			dailyApp.Preferences().SetFloat("window-height", float64(size.Height))
		})
	}
}

func createToolbar() *fyne.Container {
	notifCount := 0
	notifTestButton := widget.NewButtonWithIcon("", theme.MailSendIcon(), func() {
		notifCount++
		link := ""
		if notifCount%2 == 0 {
			link = "https://www.example.com/meeting/123?foo=bar&baz=qux"
		}
		notification.SendNotification(dailyApp, "Test notification", "This is a test notification", link)
	})
	notifTestButton.Hidden = !*debugFlag
	lastErrorButton = widget.NewButtonWithIcon("", theme.ErrorIcon(), func() {})
	lastErrorButton.Importance = widget.DangerImportance
	lastErrorButton.Hidden = true
	refreshButton := widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), func() { refresh(true) })
	settingsButton := widget.NewButtonWithIcon("", theme.SettingsIcon(), func() { showSettings(dailyApp) })
	toolbar := container.NewHBox(layout.NewSpacer(), notifTestButton, lastErrorButton, refreshButton, settingsButton)
	return toolbar
}

func changeDay(newDate time.Time, dayButton *widget.Button) {
	slog.Info("Changing day to " + newDate.Format(dayFormat))
	displayDay = newDate
	dayButton.SetText(displayDay.Format(dayFormat))
	refresh(false)
}

func startCronJobs() {
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
}

func refresh(retrieveEvents bool) {
	msg := "Refreshing UI for date " + displayDay.Format("2006-01-02") + ". retrieveEvents = " + strconv.FormatBool(retrieveEvents)
	if retrieveEvents {
		slog.Info(msg)
	} else {
		slog.Debug(msg)
	}

	if isOnSameDay(displayDay, time.Now()) {
		dayButton.Importance = widget.HighImportance
	} else {
		dayButton.Importance = widget.MediumImportance
	}
	dayButton.Refresh()

	eventsList.RemoveAll()
	events, err := getEvents(retrieveEvents)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			slog.Warn("Not refreshing. No calendar-token found")
			return
		}
		handleEventRetrievalError(err)
		return
	} else if !lastErrorButton.Hidden {
		slog.Info("Clearing last user error")
		lastErrorButton.Hidden = true
	}

	if len(events) == 0 {
		showNoEvents()
	}

	processEvents(events)
}

func getEvents(forceRetrieve bool) ([]event, error) {
	if currentEventSource == nil {
		slog.Info("No event source found. Creating one")
		if *testCalendar {
			currentEventSource = newDummyEventSource()
		} else {
			var err error
			calendarToken, err := keyring.Get("theHilikus-daily-app", "calendar-token")
			if err != nil {
				return nil, err
			}
			if calendarToken == "" {
				return nil, errors.New("empty token")
			}
			currentEventSource, err = newGoogleCalendarEventSource(calendarToken)
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

	events, fullRefreshed, err := currentEventSource.getEvents(displayDay, forceRetrieve)

	if fullRefreshed {
		lastFullRefresh = time.Now()
	}

	return events, err
}

func handleEventRetrievalError(err error) {
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
}

func reportUserError(errorMessage string) {
	slog.Info("Reporting user error: " + errorMessage)
	lastErrorButton.Hidden = false
	lastErrorButton.OnTapped = func() {
		dialog.ShowError(errors.New(errorMessage), dailyApp.Driver().AllWindows()[0])
	}
}

func processEvents(events []event) {
	expandedState := make(map[string]bool)
	for _, obj := range eventsList.Objects {
		if eventWidget, ok := obj.(*ui.Event); ok {
			if eventWidget.IsOpen() {
				expandedState[eventWidget.Id] = true
			}
		}
	}

	for pos := range events {
		event := &events[pos]
		title := createEventTitle(event)

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

		buttons := createEventButtons(event)

		cleanedDetails := cleanEventDetails(event.details)
		detailsWidget := widget.NewRichTextFromMarkdown(cleanedDetails)
		detailsWidget.Wrapping = fyne.TextWrapWord

		eventWidget := ui.NewEvent(event.id, responseIcon, title, buttons, detailsWidget)
		if expandedState[eventWidget.Id] {
			eventWidget.Open()
		}
		eventsList.Add(eventWidget)
	}

	eventsList.Refresh()
}

func createEventTitle(event *event) *ui.ClickableText {
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

	title := ui.NewClickableText(eventText, eventStyle, eventColour)
	return title
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

func createEventButtons(event *event) []*widget.Button {
	var buttons []*widget.Button
	if event.isVirtualMeeting() {
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

	return buttons
}

func cleanEventDetails(details string) string {
	result := details
	if isHTML(details) {

		markdown, err := md.ConvertString(details)
		if err != nil {
			slog.Error("Could not convert details '"+details+"' to markdown", "error", err)
		} else {
			result = markdown
		}
	}

	const rawUrlPattern = `(?m)(https?://[-a-zA-Z0-9@:%._+~#=]{1,256}\.[a-zA-Z0-9()]{1,6}\b[-a-zA-Z0-9()@:%_+.~#?&/=]*)`
	combinedPattern := regexp.MustCompile(`\[[^\]]+\]\([^)]+\)|` + rawUrlPattern)

	result = combinedPattern.ReplaceAllStringFunc(result, func(match string) string {
		// If the match is already markdown, return as is
		if strings.HasPrefix(match, "[") {
			return match
		}
		return fmt.Sprintf("[%s](%s)", match, match)
	})

	return result
}

func isHTML(s string) bool {
	// This regular expression looks for a '<' character, followed by an optional '/',
	// then one or more characters that are not '>', and finally a '>'.
	// This is a simplified pattern and might have false positives/negatives.
	re := regexp.MustCompile(`</?[a-zA-Z][^>]*>`)
	return re.MatchString(s)
}

func showNoEvents() {
	noEventsLabel := widget.NewLabel("No events today")
	eventsList.Add(layout.NewSpacer())
	eventsList.Add(container.NewCenter(noEventsLabel))
	eventsList.Add(layout.NewSpacer())
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

	var meetingLink string
	if event.isVirtualMeeting() {
		meetingLink = event.location
	}
	notification.SendNotification(dailyApp, notifTitle, notifBody, meetingLink)
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
		gCalToken, err = executeGoogleOAuthFlow()
		if err != nil {
			dialog.ShowError(err, settingsWindow)
			return
		}
	})

	connectBox := container.NewHBox(connectButton, calendarIdLabel, container.NewGridWrap(fyne.NewSize(100, calendarIdBox.MinSize().Height), calendarIdBox))

	saveButton := widget.NewButton("Save", func() {
		dailyApp.Preferences().SetString("calendar-id", calendarIdBox.Text)
		if gCalToken != "" {
			err := keyring.Set("theHilikus-daily-app", "calendar-token", gCalToken)
			if err != nil {
				slog.Error("Could not save calendar token", "error", err)
				return
			}
			currentEventSource = nil // blank it so that it gets re-instantiated with the new token
			refresh(true)
		}

		slog.Info("Preferences saved")
		settingsWindow.Close()
	})

	cancelButton := widget.NewButton("Cancel", func() {
		settingsWindow.Close()
	})

	versionLabel := widget.NewLabel("Version: " + appVersion)
	content := container.NewVBox(
		widget.NewLabel("Connect to"),
		connectBox,
		layout.NewSpacer(),
		versionLabel,
		container.NewHBox(layout.NewSpacer(), saveButton, cancelButton),
	)

	settingsWindow.SetContent(content)
	settingsWindow.Show()
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

func (otherEvent *event) isVirtualMeeting() bool {
	return strings.HasPrefix(otherEvent.location, "https://") || strings.HasPrefix(otherEvent.location, "http://")
}
