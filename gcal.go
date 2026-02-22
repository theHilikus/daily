package main

import (
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"encoding/base64"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

//go:embed secrets/client.json
var clientSecret []byte

type oAuthResult struct {
	Token string
	Err   error
}

func executeCancellableGoogleOAuthFlow() (context.CancelFunc, <-chan oAuthResult, error) {
	slog.Info("Starting PKCE OAuth flow for Google Calendar")

	config, err := createOAuthConfig()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create config: %w", err)
	}
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create listener: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port

	config.RedirectURL = fmt.Sprintf("http://localhost:%d/callback", port)
	state, err := generateRandomURLSafeString(16)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate state: %w", err)
	}
	codeVerifier, err := generateRandomURLSafeString(32)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate code verifier: %w", err)
	}
	codeChallenge := oauth2.S256ChallengeOption(codeVerifier)
	authURL := config.AuthCodeURL(state, oauth2.AccessTypeOffline, codeChallenge)
	parsedURL, err := url.Parse(authURL)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse OAuth URL: %w", err)
	}
	// Open the URL in the user's browser
	err = dailyApp.OpenURL(parsedURL)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open OAuth URL: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Buffered channel ensures the sender (HTTP handler) doesn't block.
	resultChan := make(chan oAuthResult, 1)

	mux := http.NewServeMux()
	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}

	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		slog.Debug("Received OAuth callback")

		if r.URL.Query().Get("state") != state {
			slog.Error("State in callback didn't match original")
			http.Error(w, "Bad request", http.StatusBadRequest)
			cancel()
			return
		}

		code := r.URL.Query().Get("code")
		token, err := config.Exchange(context.Background(), code, oauth2.SetAuthURLParam("code_verifier", codeVerifier))
		if err != nil {
			slog.Error("Token exchange failed", "error", err, "scopes", config.Scopes, "redirect_uri", config.RedirectURL)
			http.Error(w, "Bad request", http.StatusBadRequest)
			cancel()
			return
		}

		tokenJSON, err := json.Marshal(token)
		if err != nil {
			slog.Error("Failed to marshal token", "error", err)
			http.Error(w, "Bad request", http.StatusBadRequest)
			cancel()
			return
		}

		w.Header().Set("Content-Type", "text/html")
		_, err = w.Write([]byte("<html><body><h1>Authentication Complete</h1>You can close this window and go back to the app</body></html>"))
		if err != nil {
			slog.Error("Failed to write success response", "error", err)
			cancel()
			return
		}

		resultChan <- oAuthResult{Token: string(tokenJSON)}

		cancel()
	})

	go func() {
		<-ctx.Done()
		shutdownCtx, timeoutCancel := context.WithTimeout(context.Background(), 5*time.Second) //to deal with zombie connections
		defer timeoutCancel()
		err := server.Shutdown(shutdownCtx)
		if err != nil {
			slog.Error("Server shutdown error", "error", err)
		} else {
			slog.Debug("Server shut down successfully")
		}

		close(resultChan)
	}()

	go func() {
		slog.Debug("Starting HTTP server", "port", port)
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			resultChan <- oAuthResult{Err: err}
			cancel()
		}
	}()

	return cancel, resultChan, nil
}

func createOAuthConfig() (*oauth2.Config, error) {
	config, err := google.ConfigFromJSON(clientSecret, calendar.CalendarEventsOwnedReadonlyScope)
	if err != nil {
		slog.Error("Unable to parse client secret file to config: %v", "error", err)
		return nil, err
	}

	return config, nil
}

func generateRandomURLSafeString(byteLength int) (string, error) {
	b := make([]byte, byteLength)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

type googleCalendarSource struct {
	service          *calendar.Service
	eventsBuffer     []event
	requestStartDate time.Time
	requestEndDate   time.Time
}

func newGoogleCalendarEventSource(calendarToken string) (EventSource, error) {
	result := googleCalendarSource{}

	config, err := createOAuthConfig()
	if err != nil {
		return nil, err
	}

	token := &oauth2.Token{}
	tokenReader := strings.NewReader(calendarToken)
	err = json.NewDecoder(tokenReader).Decode(token)
	if err != nil {
		slog.Error("Error decoding token")
		return nil, err
	}

	client := config.Client(context.Background(), token)

	ctx := context.Background()
	result.service, err = calendar.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		slog.Error("Unable to retrieve Calendar client", "error", err)
		return nil, err
	}

	return &result, nil
}

// gets the events from the buffer unless forceRetrieve is true or the buffer is empty or too close to the requested date
func (gcal *googleCalendarSource) getDayEvents(day time.Time, forceRetrieve bool) ([]event, error) {
	refreshed := false

	if len(gcal.eventsBuffer) == 0 {
		slog.Debug("Events buffer is empty")
		err := gcal.retrieveEventsAround(day)
		if err != nil {
			return nil, err
		}
		refreshed = true
	}

	const minBufferThreshold = 2

	if int(day.Sub(gcal.requestStartDate).Hours()/24) < minBufferThreshold {
		slog.Debug("Too close to buffer start")
		err := gcal.retrieveEventsAround(day)
		if err != nil {
			return nil, err
		}
		refreshed = true
	} else if int(gcal.requestEndDate.Sub(day).Hours()/24) < minBufferThreshold {
		slog.Debug("Too close to buffer end")
		err := gcal.retrieveEventsAround(day)
		if err != nil {
			return nil, err
		}
		refreshed = true
	}

	if forceRetrieve && !refreshed {
		slog.Debug("Forcing retrieval of events")
		err := gcal.retrieveEventsAround(day)
		if err != nil {
			return nil, err
		}
		refreshed = true
	}

	var result []event
	for _, event := range gcal.eventsBuffer {
		if isOnSameDay(day, event.start) {
			result = append(result, event)
		}
	}

	return result, nil
}

func (gcal *googleCalendarSource) retrieveEventsAround(day time.Time) error {
	_, timezoneOffset := day.Zone()
	const requestHalfWindow int = 5
	syncToken := dailyApp.Preferences().String("calendar-sync-token")
	newRequestStartDate := day.AddDate(0, 0, -requestHalfWindow).Truncate(24 * time.Hour).Add(time.Second * time.Duration(-timezoneOffset))
	isIncremental := syncToken != "" && len(gcal.eventsBuffer) > 0 && gcal.requestStartDate == newRequestStartDate

	gcal.requestStartDate = newRequestStartDate
	gcal.requestEndDate = day.AddDate(0, 0, requestHalfWindow).Truncate(24 * time.Hour).Add(time.Second * time.Duration(-timezoneOffset))
	calendarId := dailyApp.Preferences().String("calendar-id")

	slog.Info("Retrieving events from gCal between " + gcal.requestStartDate.Format(time.RFC3339) + " and " + gcal.requestEndDate.Format(time.RFC3339) + " for calendarId = " + calendarId)
	listCall := gcal.service.Events.List(calendarId)

	if isIncremental {
		slog.Debug("Performing incremental sync with syncToken")
		listCall.SyncToken(syncToken)
	} else {
		slog.Debug("Performing full sync")
		listCall.TimeMin(gcal.requestStartDate.Format(time.RFC3339)).TimeMax(gcal.requestEndDate.Format(time.RFC3339))
	}

	response, err := listCall.
		SingleEvents(true).
		Fields("etag", "nextPageToken", "nextSyncToken", "summary", "timeZone", "items(attendees, conferenceData, created, updated, description, start, end, etag, eventType, hangoutLink, htmlLink, id, location, status, summary, transparency, recurringEventId)").
		Do()

	if err != nil {
		// A 410 GONE status indicates the sync token is invalid. Perform a full sync to get a new sync token.
		if googleErr, ok := err.(*googleapi.Error); ok && googleErr.Code == http.StatusGone {
			slog.Warn("Sync token is invalid. Performing a full sync.")
			dailyApp.Preferences().SetString("calendar-sync-token", "")
			gcal.eventsBuffer = nil
			return gcal.retrieveEventsAround(day)
		}
		return err
	}

	if response.NextPageToken != "" {
		slog.Warn("There is a next page. Ignoring it for now")
	}

	dailyApp.Preferences().SetString("calendar-sync-token", response.NextSyncToken)

	slog.Debug("Retrieved "+strconv.Itoa(len(response.Items))+" changed event(s) successfully", "calendarId", calendarId)

	eventsMap, err2 := gcal.createEventsFromResponse(isIncremental, response)
	if err2 != nil {
		return err2
	}

	if len(response.Items) != len(eventsMap) {
		slog.Debug("Kept " + strconv.Itoa(len(eventsMap)) + " event(s) to be used")
	}

	gcal.eventsBuffer = make([]event, 0, len(eventsMap))
	for _, e := range eventsMap {
		gcal.eventsBuffer = append(gcal.eventsBuffer, e)
	}

	slices.SortFunc(gcal.eventsBuffer, func(a, b event) int {
		if a.start.Before(b.start) {
			return -1
		} else if a.start.After(b.start) {
			return 1
		} else {
			return strings.Compare(a.title, b.title)
		}
	})

	return nil
}

func (gcal *googleCalendarSource) createEventsFromResponse(isIncremental bool, response *calendar.Events) (map[string]event, error) {
	// Create a map to hold the final list of events.
	// If it's an incremental sync, prepopulate it with the existing events.
	// If it's a full sync, it will start empty, effectively replacing the old buffer.
	result := make(map[string]event)
	if isIncremental {
		for _, e := range gcal.eventsBuffer {
			result[e.id] = e
		}
	}

	for _, item := range response.Items {
		// If an event is "cancelled", it means it was deleted. Remove it from our map.
		if item.Status == "cancelled" {
			delete(result, item.Id)
			continue
		}

		if item.Start.DateTime != "" {
			//for now, ignore day events
			eventStart, err := time.Parse(time.RFC3339, item.Start.DateTime)
			if err != nil {
				return nil, err
			}

			eventEnd, err := time.Parse(time.RFC3339, item.End.DateTime)
			if err != nil {
				return nil, err
			}

			var selfResponse responseStatus
			for _, attendee := range item.Attendees {
				if attendee.Self {
					selfResponse = responseStatus(attendee.ResponseStatus)
					break
				}
			}

			newEvent := event{
				id:         item.Id,
				title:      item.Summary,
				start:      eventStart,
				end:        eventEnd,
				details:    item.Description,
				notifiable: selfResponse != "declined" && item.Transparency != "transparent" && eventStart.After(time.Now()),
				response:   selfResponse,
				recurring:  item.RecurringEventId != "",
			}

			if item.ConferenceData != nil {
				for _, entryPoint := range item.ConferenceData.EntryPoints {
					if entryPoint.EntryPointType == "video" {
						newEvent.location = entryPoint.Uri
						break
					}
				}
			} else if item.HangoutLink != "" {
				newEvent.location = item.HangoutLink
			} else {
				newEvent.location = item.Location
			}

			result[newEvent.id] = newEvent
		}
	}

	return result, nil
}
