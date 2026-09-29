package notify

import (
	"fmt"
	"os"
)

type TerminalNotifier struct {
	enabled bool
}

func NewTerminalNotifier(enabled bool) *TerminalNotifier {
	return &TerminalNotifier{enabled: enabled}
}

func (t *TerminalNotifier) Name() string { return "terminal" }

func (t *TerminalNotifier) Notify(n Notification) error {
	if !t.enabled {
		return nil
	}

	// 写入 stderr（避免污染标准输出）
	icon := "📨"
	switch n.Type {
	case "friend_request":
		icon = "👤"
	case "approval":
		icon = "🔔"
	case "system":
		icon = "⚡"
	}

	fmt.Fprintf(os.Stderr, "\n\033[1;36m%s [%s] %s\033[0m\n", icon, n.AgentName, n.Title)
	if n.Message != "" {
		fmt.Fprintf(os.Stderr, "   %s\n", n.Message)
	}

	return nil
}
