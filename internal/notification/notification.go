//go:build !windows

package notification

import (
	"log/slog"

	"fyne.io/fyne/v2"
)

func SendNotification(app fyne.App, title, message string, _ string, _ fyne.Resource) {
	notification := fyne.NewNotification(title, message)
	slog.Debug("Sending notification", "data", notification)
	app.SendNotification(notification)
}
