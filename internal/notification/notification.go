//go:build !windows

package notification

import (
	"fyne.io/fyne/v2"
)

func SendNotification(app fyne.App, title, message string, _ string, _ fyne.Resource) {
	notification := fyne.NewNotification(title, message)
	app.SendNotification(notification)
}
