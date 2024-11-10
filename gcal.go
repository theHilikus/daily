package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"encoding/base64"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"
)

const (
	clientSecretFile = "secrets/client.json"
)

type googleCalendar struct {
	service          *calendar.Service
	eventsBuffer     []event
	requestStartDate time.Time
	requestEndDate   time.Time
}

func startGCalOAuthFlow() (string, error) {
	slog.Info("Starting PKCE OAuth flow for Google Calendar")

	config, err := createOAuthConfig()
	if err != nil {
		slog.Error("Failed to create config", "error", err)
		return "", err
	}
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		slog.Error("Failed to create listener", "error", err)
		return "", err
	}
	port := listener.Addr().(*net.TCPAddr).Port

	config.RedirectURL = fmt.Sprintf("http://localhost:%d/callback", port)
	state, err := generateRandomURLSafeString(16)
	if err != nil {
		slog.Error("Failed to generate state", "error", err)
		return "", err
	}
	slog.Debug("Generated state: " + state)
	codeVerifier, err := generateRandomURLSafeString(32)
	if err != nil {
		slog.Error("Failed to generate code verifier: %v", err)
		return "", err
	}
	codeChallenge := oauth2.S256ChallengeOption(codeVerifier)
	authURL := config.AuthCodeURL(state, oauth2.AccessTypeOffline, codeChallenge)

	parsedURL, err := url.Parse(authURL)
	if err != nil {
		slog.Error("Failed to parse OAuth URL", "error", err)
		return "", err
	}

	// Open the URL in the user's browser
	err = dailyApp.OpenURL(parsedURL)
	if err != nil {
		slog.Error("Failed to open OAuth URL", "error", err)
		return "", err
	}

	done := make(chan bool)

	server := &http.Server{Addr: fmt.Sprintf(":%d", port)}
	var tokenResult string
	http.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != state {
			slog.Error("State in callback didn't match original")
			http.Error(w, "Invalid state", http.StatusBadRequest)
			return
		}

		code := r.URL.Query().Get("code")
		token, err := config.Exchange(context.Background(), code, oauth2.SetAuthURLParam("code_verifier", codeVerifier))
		if err != nil {
			http.Error(w, "Failed to exchange token", http.StatusInternalServerError)
			slog.Error("Token exchange failed", "error", err, "scopes", config.Scopes, "redirect_uri", config.RedirectURL)
			return
		}

		slog.Info("Authentication successful!")

		tokenJSON, err := json.Marshal(token)
		if err != nil {
			http.Error(w, "Failed to marshal token", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<html><body><h1>Authentication Complete</h1></body></html>"))

		done <- true
		go server.Shutdown(context.Background())

		tokenResult = string(tokenJSON)
	})

	go func() {
		if err := server.Serve(listener); err != http.ErrServerClosed {
			slog.Error("Server error", "error", err)
		}
		done <- true
	}()

	<-done // Wait for the callback to complete

	return tokenResult, nil
}

func generateRandomURLSafeString(byteLength int) (string, error) {
	b := make([]byte, byteLength)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func newGoogleCalendarEventSource() (*googleCalendar, error) {
	result := googleCalendar{}

	config, err := createOAuthConfig()
	if err != nil {
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
		slog.Error("Unable to retrieve Calendar client", "error", err)
	}

	return &result, nil
}

func createOAuthConfig() (*oauth2.Config, error) {
	clientSecret, err := os.ReadFile(clientSecretFile)
	if err != nil {
		slog.Error("Unable to read client secret file: ", "error", err)
		return nil, err
	}

	config, err := google.ConfigFromJSON(clientSecret, calendar.CalendarEventsReadonlyScope)
	if err != nil {
		slog.Error("Unable to parse client secret file to config: %v", "error", err)
		return nil, err
	}

	return config, nil
}

func (gcal *googleCalendar) getEvents(day time.Time, fullRefresh bool) ([]event, bool, error) {
	refreshed := false

	if len(gcal.eventsBuffer) == 0 {
		slog.Debug("Events buffer is empty")
		err := gcal.retrieveEventsAround(day)
		if err != nil {
			return nil, false, err
		}
		refreshed = true
	}

	const minBufferThreshold = 2

	if int(day.Sub(gcal.requestStartDate).Hours()/24) < minBufferThreshold {
		slog.Debug("Too close to buffer start")
		err := gcal.retrieveEventsAround(gcal.requestStartDate)
		if err != nil {
			return nil, false, err
		}
		refreshed = true
	} else if int(gcal.requestEndDate.Sub(day).Hours()/24) < minBufferThreshold {
		slog.Debug("Too close to buffer end")
		err := gcal.retrieveEventsAround(gcal.requestEndDate)
		if err != nil {
			return nil, false, err
		}
		refreshed = true
	}

	if fullRefresh && !refreshed {
		slog.Debug("Forcing retrieve of events")
		err := gcal.retrieveEventsAround(day)
		if err != nil {
			return nil, false, err
		}
		refreshed = true
	}

	var result []event
	for _, event := range gcal.eventsBuffer {
		if isOnSameDay(day, event.start) {
			result = append(result, event)
		}
	}

	return result, refreshed, nil
}

func (gcal *googleCalendar) retrieveEventsAround(day time.Time) error {
	_, timezoneOffset := day.Zone()
	const requestHalfWindow int = 5
	gcal.requestStartDate = day.AddDate(0, 0, -requestHalfWindow).Truncate(24 * time.Hour).Add(time.Second * time.Duration(-timezoneOffset))
	gcal.requestEndDate = day.AddDate(0, 0, requestHalfWindow).Truncate(24 * time.Hour).Add(time.Second * time.Duration(-timezoneOffset))
	calendarId := dailyApp.Preferences().String("calendar-id")
	slog.Info("Retrieving events between " + gcal.requestStartDate.Format(time.RFC3339) + " and " + gcal.requestEndDate.Format(time.RFC3339) + " for calendarId = " + calendarId)
	response, err := gcal.service.Events.List(calendarId).
		SingleEvents(true).
		TimeMin(gcal.requestStartDate.Format(time.RFC3339)).
		TimeMax(gcal.requestEndDate.Format(time.RFC3339)).
		OrderBy("startTime").
		Fields("etag", "nextPageToken", "summary", "timeZone", "items(attendees, created, updated, description, start, end, etag, eventType, hangoutLink, htmlLink, id, location, status, summary, transparency)").
		Do()

	if err == nil {
		slog.Debug("Retrieved " + strconv.Itoa(len(response.Items)) + " event(s) successfully")
	} else {
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
