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

type userStatusValues string

const (
	offline      userStatusValues = "offline"
	doNotDisturb                  = "dnd"
)

type userAvailability struct {
	Id           string           `json:"user_id"`
	Availability userStatusValues `json:"status"`
	SetManually  bool             `json:"manual"`
}

func UpdateMattermostStatus(serverUrl string, eventEnd time.Time, statusMessage string, statusEmoji string) {
	mmAuthToken, err := keyring.Get("theHilikus-daily-app", "mattermost-token")
	if err != nil {
		slog.Error("Error retrieving mattermost token", "err", err)
		return
	}

	if !strings.HasPrefix(serverUrl, "https://") {
		serverUrl = "https://" + serverUrl
	}

	currentUserStatus, err := getUserAvailability(serverUrl, mmAuthToken)
	if err != nil {
		slog.Error("Error retrieving current user availability. Skipping update", "err", err)
		return
	}
	if currentUserStatus.Availability == offline && currentUserStatus.SetManually {
		slog.Info("Mattermost user availability is offline set manually. Skipping update")
		return
	}

	currentCustomStatus, err := getCurrentCustomStatus(serverUrl, mmAuthToken)
	if err != nil {
		slog.Error("Error retrieving current custom status. Skipping update", "err", err)
		return
	}
	if !currentCustomStatus.isSet() {
		status := MattermostStatus{Text: statusMessage, Emoji: statusEmoji, ExpiresAt: eventEnd.Truncate(time.Minute)}
		err := status.sendCustomStatus(serverUrl, mmAuthToken)
		if err != nil {
			slog.Error("Error setting custom mattermost status", "err", err)
			return
		}
		slog.Debug("Custom status set successfully")

		if !currentUserStatus.SetManually {
			err = status.sendAvailability(serverUrl, mmAuthToken)
			if err != nil {
				slog.Error("Error setting mattermost availability", "err", err)
				return
			}
			slog.Debug("Do Not Disturb status set successfully")
		} else {
			slog.Info("Mattermost user availability is set manually. Skipping availability update")
		}

		slog.Info("Mattermost status updated successfully", "expiry", status.ExpiresAt)
	} else {
		slog.Info("Mattermost custom status is already set until " + currentCustomStatus.ExpiresAt.String() + ". Skipping update")
	}
}

func getUserAvailability(serverUrl string, authToken string) (*userAvailability, error) {
	slog.Debug("Getting current user availability from mattermost", "server", serverUrl)

	userEndpoint := "/api/v4/users/me/status"
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

	var availability userAvailability
	err2 := json.Unmarshal(body, &availability)
	if err2 != nil {
		return nil, fmt.Errorf("error parsing response body: %w", err2)
	}
	mmUserId = availability.Id

	return &availability, nil
}

func GetCurrentStatus(serverUrl string, mmAuthToken string) (*MattermostStatus, error) {
	if !strings.HasPrefix(serverUrl, "https://") {
		serverUrl = "https://" + serverUrl
	}

	return getCurrentCustomStatus(serverUrl, mmAuthToken)
}

func getCurrentCustomStatus(serverUrl string, authToken string) (*MattermostStatus, error) {
	slog.Debug("Getting current custom status from mattermost", "server", serverUrl)

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

func (s *MattermostStatus) sendAvailability(url string, token string) error {
	dndUntil := s.ExpiresAt.Unix()
	dndPayload := map[string]any{
		"user_id":      mmUserId,
		"status":       doNotDisturb,
		"dnd_end_time": dndUntil,
	}

	dndEndpoint := "/api/v4/users/me/status"

	jsonData, err := json.Marshal(dndPayload)
	if err != nil {
		return fmt.Errorf("error marshaling availability payload: %w", err)
	}

	req, err := http.NewRequest(http.MethodPut, url+dndEndpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("error creating availability request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error sending availability request: %w", err)
	}

	defer func(body io.ReadCloser) {
		err := body.Close()
		if err != nil {
			slog.Error("Failed to close availability response body", "error", err)
		}
	}(resp.Body)

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to set availability. statusCode=%d, body=%s", resp.StatusCode, string(body))
	}

	return nil
}
