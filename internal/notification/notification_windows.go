//go:build windows

package notification

import (
	"log/slog"
	"os"
	"path/filepath"

	"fyne.io/fyne/v2"
	"gopkg.in/toast.v1"
)

func SendNotification(app fyne.App, title, message string, meetingLink string) {
	slog.Info("Using Windows-specific notification.")

	var actions []toast.Action
	if meetingLink != "" {
		actions = []toast.Action{
			{Type: "protocol", Label: "Launch Meeting", Arguments: meetingLink},
		}
	}
	notification := toast.Notification{
		AppID:    app.Metadata().Name,
		Title:    title,
		Message:  message,
		Audio:    toast.Reminder,
		Actions:  actions,
		Duration: toast.Long,
		Icon:     dumpIcon(app),
	}

	slog.Debug("Sending notification", "data", notification)
	err := notification.Push()
	if err != nil {
		slog.Error("Toast Error:", "error", err)
	}
}

func dumpIcon(app fyne.App) string {
	tmpDir := os.TempDir()
	iconPath := filepath.Join(tmpDir, app.Metadata().Icon.Name())

	if _, err := os.Stat(iconPath); err == nil {
		return iconPath
	}

	resource := app.Icon()
	if resource == nil {
		return ""
	}

	iconFile, err := os.Create(iconPath)
	if err != nil {
		slog.Error("Failed to create icon file", "error", err)
		return ""
	}
	defer iconFile.Close()

	_, err = iconFile.Write(resource.Content())
	if err != nil {
		slog.Error("Failed to write icon content", "error", err)
		return ""
	}

	return iconPath
}
