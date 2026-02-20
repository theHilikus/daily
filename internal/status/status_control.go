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

type MattermostStatus struct {
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

func UpdateMattermostStatus(serverUrl string, eventEnd time.Time, statusMessage string, statusEmoji string) {
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
	if !currentStatus.isSet() {
		status := MattermostStatus{Text: statusMessage, Emoji: statusEmoji, ExpiresAt: eventEnd.Truncate(time.Minute)}
		err := status.publish(serverUrl, mmAuthToken)
		if err != nil {
			slog.Error("Error setting mattermost status", "err", err)
			return
		}
		slog.Info("Mattermost status updated successfully")
	} else {
		slog.Info("Mattermost status is already set. Skipping update")
	}
}

func GetCurrentStatus(serverUrl string) (*MattermostStatus, error) {
	mmAuthToken, err := keyring.Get("theHilikus-daily-app", "mattermost-token")
	if err != nil {
		return nil, fmt.Errorf("error retrieving mattermost auth token: %w", err)
	}

	return getCurrentStatus(serverUrl, mmAuthToken)
}

func getCurrentStatus(serverUrl string, authToken string) (*MattermostStatus, error) {
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
		return new(MattermostStatus), nil
	}

	var result MattermostStatus
	if err := json.Unmarshal([]byte(user.Props.CustomStatus), &result); err != nil {
		return nil, fmt.Errorf("error parsing custom status: %w", err)
	}

	return &result, nil
}

func (s *MattermostStatus) isSet() bool {
	return (s.Text != "" || s.Emoji != "") && (s.ExpiresAt.IsZero() || s.ExpiresAt.After(time.Now()))
}

func (s *MattermostStatus) publish(serverUrl string, authToken string) error {
	if !strings.HasPrefix(serverUrl, "https://") {
		serverUrl = "https://" + serverUrl
	}
	err := s.sendCustomStatus(serverUrl, authToken)
	if err != nil {
		return err
	}
	slog.Debug("Custom status set successfully", "expiry", s.ExpiresAt)

	err = s.sendDoNotDisturbStatus(serverUrl, authToken)
	if err != nil {
		return err
	}
	slog.Debug("Do Not Disturb status set successfully", "expiry", s.ExpiresAt)

	return nil
}

func (s *MattermostStatus) sendCustomStatus(serverUrl string, authToken string) error {
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

	return nil
}

func (s *MattermostStatus) sendDoNotDisturbStatus(url string, token string) error {
	dndUntil := s.ExpiresAt.Unix()
	dndPayload := map[string]any{
		"user_id":      mmUserId,
		"status":       "dnd",
		"dnd_end_time": dndUntil,
	}

	dndEndpoint := "/api/v4/users/me/status"

	jsonData, err := json.Marshal(dndPayload)
	if err != nil {
		return fmt.Errorf("error marshaling DND payload: %w", err)
	}

	req, err := http.NewRequest(http.MethodPut, url+dndEndpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("error creating DND request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error sending DND request: %w", err)
	}

	defer func(body io.ReadCloser) {
		err := body.Close()
		if err != nil {
			slog.Error("Failed to close DND response body", "error", err)
		}
	}(resp.Body)

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to set DND status. statusCode=%d, body=%s", resp.StatusCode, string(body))
	}

	return nil
}
