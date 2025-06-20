//go:build windows

package notification

import (
	"log/slog"

	"fyne.io/fyne/v2"
	"github.com/go-toast/toast"
)

func sendCustomNotification(app fyne.App, title, message string) {
	slog.Info("Using Windows-specific notification.")

	notification := toast.Notification{
		AppID:   app.Metadata().Name,
		Title:   title,
		Message: message,
		Audio:   toast.Reminder,
		Actions: []toast.Action{
			{Type: "protocol", Label: "Launch Meeting", Arguments: ""},
		},
	}

	err := notification.Push()
	if err != nil {
		slog.Error("Toast Error:", "error", err)
	}
}
