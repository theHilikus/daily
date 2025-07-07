//go:build windows

package notification

import (
	"bytes"
	"fmt"
	"fyne.io/fyne/v2"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"

	"syscall"
)

var toastTemplate *template.Template

type notification struct {
	// The name of your app. This value shows up in Windows 10's Action Centre, so make it
	// something readable for your users. It can contain spaces, however special characters
	// (eg. é) are not supported.
	AppID string

	// The main title/heading for the toast notification.
	Title string

	// The single/multi line message to display for the toast notification.
	Message string

	// An optional path to an image on the OS to display to the left of the title & message.
	Icon string

	// The type of notification level action (like toast.Action)
	ActivationType string

	// The activation/action arguments (invoked when the user clicks the notification)
	ActivationArguments string

	// Optional action buttons to display below the notification title & message.
	Actions []action
}

type action struct {
	Type      string
	Label     string
	Arguments string
}

func init() {
	toastTemplate = template.New("toast")
	toastTemplate.Parse(`
[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] | Out-Null
[Windows.UI.Notifications.ToastNotification, Windows.UI.Notifications, ContentType = WindowsRuntime] | Out-Null
[Windows.Data.Xml.Dom.XmlDocument, Windows.Data.Xml.Dom.XmlDocument, ContentType = WindowsRuntime] | Out-Null

$APP_ID = '{{.AppID}}'
$SOUND = "ms-winsoundevent:Notification.Reminder"

$template = @"
<toast scenario="reminder" activationType="{{.ActivationType}}" launch="{{.ActivationArguments}}">
    <visual>
        <binding template="ToastGeneric">
            <image placement="appLogoOverride" src="{{.Icon}}" />
            <text><![CDATA[{{.Title}}]]></text>
            <text><![CDATA[{{.Message}}]]></text>
        </binding>
    </visual>
    
	<audio src="$SOUND" loop="false" />

    {{if .Actions}}
    <actions>
        {{range .Actions}}
        <action activationType="{{.Type}}" content="{{.Label}}" arguments="{{.Arguments}}" />
        {{end}}
    </actions>
    {{end}}
</toast>
"@

$xml = New-Object Windows.Data.Xml.Dom.XmlDocument
$xml.LoadXml($template)
$toast = New-Object Windows.UI.Notifications.ToastNotification $xml
[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier($APP_ID).Show($toast)
    `)
}

func SendNotification(app fyne.App, title, message string, meetingLink string) {
	slog.Info("Using Windows-specific notification.")

	actions := []action{}
	if meetingLink != "" {
		actions = append(actions, action{Type: "protocol", Label: "Launch Meeting", Arguments: meetingLink})
	}
	actions = append(actions, action{Type: "protocol", Label: "Dismiss"})

	notification := notification{
		AppID:   app.Metadata().Name,
		Title:   title,
		Message: message,
		Actions: actions,
		Icon:    dumpIcon(app),
	}

	slog.Debug("Sending notification", "data", notification)
	err := notification.push()
	if err != nil {
		slog.Error("Toast Error:", "error", err)
	}
}

func (n *notification) push() error {
	n.applyDefaults()
	xml, err := n.buildXML()
	if err != nil {
		return err
	}
	return runScript("notification", xml)
}

func (n *notification) applyDefaults() {
	if n.ActivationType == "" {
		n.ActivationType = "protocol"
	}
}

func (n *notification) buildXML() (string, error) {
	var out bytes.Buffer
	err := toastTemplate.Execute(&out, n)
	if err != nil {
		return "", err
	}
	return out.String(), nil
}

func dumpIcon(app fyne.App) string {
	tmpDir := os.TempDir()
	iconPath := filepath.Join(tmpDir, app.UniqueID()+"-"+app.Metadata().Icon.Name())

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

var scriptNum = 0

func runScript(name, script string) error {
	scriptNum++
	appID := fyne.CurrentApp().UniqueID()
	fileName := fmt.Sprintf("%s-%s-%d.ps1", appID, name, scriptNum)

	tmpFilePath := filepath.Join(os.TempDir(), fileName)
	err := os.WriteFile(tmpFilePath, []byte(script), 0600)
	if err != nil {
		return err
	}
	defer os.Remove(tmpFilePath)

	launch := "(Get-Content -Encoding UTF8 -Path " + tmpFilePath + " -Raw) | Invoke-Expression"
	cmd := exec.Command("PowerShell", "-ExecutionPolicy", "Bypass", launch)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	err = cmd.Run()
	if err != nil {
		return err
	}

	return nil
}
