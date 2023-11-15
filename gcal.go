package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"
)

const (
	tokenFile        = "gcalToken.json"
	clientSecretFile = "secrets/client.json"
)

type googleCalendar struct {
	service          *calendar.Service
	eventsBuffer     []event
	requestStartDate time.Time
	requestEndDate   time.Time
}

func newGoogleCalendar() (*googleCalendar, error) {
	result := googleCalendar{}

	clientSecret, err := os.ReadFile(clientSecretFile)
	if err != nil {
		slog.Error("Unable to read client secret file: ", err)
		return nil, err
	}

	config, err := google.ConfigFromJSON(clientSecret, calendar.CalendarEventsReadonlyScope)
	if err != nil {
		slog.Error("Unable to parse client secret file to config: %v", err)
		return nil, err
	}

	tok := &oauth2.Token{}
	tokenReader := strings.NewReader(dailyApp.Preferences().String("calendar-token"))
	err = json.NewDecoder(tokenReader).Decode(tok)
	if err != nil {
		slog.Error("Error decoding token")
		return nil, err
	}

	client := config.Client(context.Background(), tok)

	ctx := context.Background()
	result.service, err = calendar.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		slog.Error("Unable to retrieve Calendar client: %v", err)
	}

	return &result, nil
}

func (gcal googleCalendar) getEvents(day time.Time) ([]event, error) {

	if len(gcal.eventsBuffer) == 0 {
		slog.Debug("Events buffer is empty")
		err := gcal.retrieveEventsAround(day)
		if err != nil {
			return nil, err
		}
	}

	const minBufferThreshold = 2

	if int(day.Sub(gcal.requestStartDate).Hours()/24) < minBufferThreshold {
		slog.Debug("Too close to buffer start")
		err := gcal.retrieveEventsAround(gcal.requestStartDate)
		if err != nil {
			return nil, err
		}
	}

	if int(gcal.requestEndDate.Sub(day).Hours()/24) < minBufferThreshold {
		slog.Debug("Too close to buffer end")
		err := gcal.retrieveEventsAround(gcal.requestEndDate)
		if err != nil {
			return nil, err
		}
	}

	var result []event
	for _, event := range gcal.eventsBuffer {
		if isOnSameDay(day, event.start) {
			result = append(result, event)
		}
	}

	return result, nil
}

func (gcal *googleCalendar) retrieveEventsAround(day time.Time) error {
	_, timezoneOffset := day.Zone()
	const requestHalfWindow int = 4
	gcal.requestStartDate = day.AddDate(0, 0, -requestHalfWindow).Truncate(24 * time.Hour).Add(time.Second * time.Duration(-timezoneOffset))
	gcal.requestEndDate = day.AddDate(0, 0, requestHalfWindow).Truncate(24 * time.Hour).Add(time.Second * time.Duration(-timezoneOffset))
	slog.Info("Retrieving events between " + gcal.requestStartDate.Format(time.RFC3339) + " and " + gcal.requestEndDate.Format(time.RFC3339))
	response, err := gcal.service.Events.List(dailyApp.Preferences().String("calendar-id")).SingleEvents(true).TimeMin(gcal.requestStartDate.Format(time.RFC3339)).TimeMax(gcal.requestEndDate.Format(time.RFC3339)).OrderBy("startTime").Do()
	if err != nil {
		slog.Error("Unable to retrieve events from google:", err)
		return err
	}

	var allEvents []event
	for _, item := range response.Items {
		if item.Start.DateTime != "" {
			//for now, ignore day events
			eventStart, err := time.Parse(time.RFC3339, item.Start.DateTime)
			if err != nil {
				return err
			}

			eventEnd, err := time.Parse(time.RFC3339, item.End.DateTime)
			if err != nil {
				return err
			}

			var selfResponse responseStatus
			for _, attendee := range item.Attendees {
				if attendee.Self {
					selfResponse = responseStatus(attendee.ResponseStatus)
					break
				}
			}

			newEvent := event{
				title:      item.Summary,
				start:      eventStart,
				end:        eventEnd,
				details:    item.Description,
				notifiable: selfResponse != "declined" && item.Transparency != "transparent",
				response:   selfResponse,
			}
			if item.HangoutLink != "" {
				newEvent.location = item.HangoutLink
			} else {
				newEvent.location = item.Location
			}
			allEvents = append(allEvents, newEvent)
		}
	}
	gcal.eventsBuffer = allEvents

	return nil
}
