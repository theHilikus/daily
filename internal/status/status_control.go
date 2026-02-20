package status

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/zalando/go-keyring"
)

var mmUserId string

func UpdateMattermostStatus(serverUrl string, eventEnd time.Time, eventMessage string, eventEmoji string) {
	mmAuthToken, err := keyring.Get("theHilikus-daily-app", "mattermost-token")
	if err != nil {
		slog.Error("Error retrieving mattermost token", "err", err)
		return
	}

	currentStatus, err := getCurrentStatus(serverUrl, mmAuthToken)
	if err != nil {
		slog.Error("Error retrieving current status. Skipping update", "err", err)
		return
	}
	if currentStatus.isSet() {
		slog.Info("Current custom status is set. Skipping update.")
	} else {
		status := CustomStatus{Text: eventMessage, Emoji: eventEmoji, ExpiresAt: eventEnd.Truncate(time.Minute)}
		err := status.send(serverUrl, mmAuthToken)
		if err != nil {
			slog.Error("Error setting custom status", "err", err)
			return
		}
	}
}

func GetCurrentStatus(serverUrl string) (*CustomStatus, error) {
	mmAuthToken, err := keyring.Get("theHilikus-daily-app", "mattermost-token")
	if err != nil {
		return nil, fmt.Errorf("error retrieving mattermost auth token: %w", err)
	}

	return getCurrentStatus(serverUrl, mmAuthToken)
}

func getCurrentStatus(serverUrl string, authToken string) (*CustomStatus, error) {
	slog.Debug("Getting current status from mattermost", "server", serverUrl)

	if !strings.HasPrefix(serverUrl, "https://") {
		serverUrl = "https://" + serverUrl
	}
	userEndpoint := "/api/v4/users/me"
	req, err := http.NewRequest(http.MethodGet, serverUrl+userEndpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+authToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			slog.Error("Failed to close response body", "error", err)
		}
	}(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET failed with status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %w", err)
	}

	var user userProfile
	err2 := json.Unmarshal(body, &user)
	if err2 != nil {
		return nil, fmt.Errorf("error parsing response body: %w", err2)
	}

	mmUserId = user.Id

	if user.Props.CustomStatus == "" {
		return new(CustomStatus), nil
	}

	var result CustomStatus
	if err := json.Unmarshal([]byte(user.Props.CustomStatus), &result); err != nil {
		return nil, fmt.Errorf("error parsing custom status: %w", err)
	}

	return &result, nil
}

type CustomStatus struct {
	Emoji     string    `json:"emoji"`
	Text      string    `json:"text"`
	ExpiresAt time.Time `json:"expires_at"`
}

type userProfile struct {
	Id    string `json:"id"`
	Props struct {
		CustomStatus string `json:"customStatus"`
	} `json:"props"`
}

func (s *CustomStatus) isSet() bool {
	return (s.Text != "" || s.Emoji != "") && (s.ExpiresAt.IsZero() || s.ExpiresAt.After(time.Now()))
}

func (s *CustomStatus) send(serverUrl string, authToken string) error {
	if !strings.HasPrefix(serverUrl, "https://") {
		serverUrl = "https://" + serverUrl
	}
	statusEndpoint := "/api/v4/users/me/status/custom"
	jsonData, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("error marshaling custom status: %w", err)
	}
	req, err := http.NewRequest(http.MethodPut, serverUrl+statusEndpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+authToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func(body io.ReadCloser) {
		err := body.Close()
		if err != nil {
			slog.Error("Failed to close response body", "error", err)
		}
	}(resp.Body)

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to set custom status. statusCode=%d, body=%s", resp.StatusCode, string(body))
	}

	slog.Info("Custom status set successfully")
	s.setDoNotDisturb(serverUrl, authToken)

	return nil
}

func (s *CustomStatus) setDoNotDisturb(url string, token string) {
	dndUntil := s.ExpiresAt.Unix()
	dndPayload := map[string]any{
		"user_id":      mmUserId,
		"status":       "dnd",
		"dnd_end_time": dndUntil,
	}

	dndEndpoint := "/api/v4/users/me/status"

	jsonData, err := json.Marshal(dndPayload)
	if err != nil {
		slog.Error("Error marshaling DND payload", "err", err)
		return
	}

	req, err := http.NewRequest(http.MethodPut, url+dndEndpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		slog.Error("Error creating DND request", "err", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		slog.Error("Error sending DND request", "err", err)
		return
	}

	defer func(body io.ReadCloser) {
		err := body.Close()
		if err != nil {
			slog.Error("Failed to close DND response body", "error", err)
		}
	}(resp.Body)

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		slog.Error("Failed to set DND status", "statusCode", resp.StatusCode, "body", string(body))
		return
	}

	slog.Debug("Do Not Disturb status set successfully", "expires_at", s.ExpiresAt)
}
