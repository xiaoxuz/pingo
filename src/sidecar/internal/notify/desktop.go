package notify

import (
	"fmt"
	"os/exec"
	"runtime"
)

type DesktopNotifier struct {
	enabled bool
}

func NewDesktopNotifier(enabled bool) *DesktopNotifier {
	return &DesktopNotifier{enabled: enabled}
}

func (d *DesktopNotifier) Name() string { return "desktop" }

func (d *DesktopNotifier) Notify(n Notification) error {
	if !d.enabled {
		return nil
	}

	title := fmt.Sprintf("[%s] %s", n.AgentName, n.Title)

	switch runtime.GOOS {
	case "darwin":
		return d.notifyDarwin(title, n.Message)
	case "linux":
		return d.notifyLinux(title, n.Message)
	default:
		return nil
	}
}

func (d *DesktopNotifier) notifyDarwin(title, message string) error {
	script := fmt.Sprintf(`display notification "%s" with title "%s"`, escapeApplescript(message), escapeApplescript(title))
	cmd := exec.Command("osascript", "-e", script)
	return cmd.Run()
}

func (d *DesktopNotifier) notifyLinux(title, message string) error {
	cmd := exec.Command("notify-send", title, message)
	return cmd.Run()
}

func escapeApplescript(s string) string {
	result := ""
	for _, c := range s {
		if c == '"' {
			result += "\\" + string(c)
		} else if c == '\\' {
			result += "\\\\"
		} else {
			result += string(c)
		}
	}
	return result
}
