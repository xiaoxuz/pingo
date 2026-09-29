package notify

import (
	"bytes"
	"encoding/json"
	"net/http"
	"time"
)

type WebhookNotifier struct {
	enabled bool
	url     string
}

func NewWebhookNotifier(enabled bool, url string) *WebhookNotifier {
	return &WebhookNotifier{enabled: enabled, url: url}
}

func (w *WebhookNotifier) Name() string { return "webhook" }

func (w *WebhookNotifier) Notify(n Notification) error {
	if !w.enabled || w.url == "" {
		return nil
	}

	payload := map[string]interface{}{
		"agent_id":   n.AgentID,
		"agent_name": n.AgentName,
		"title":      n.Title,
		"message":    n.Message,
		"type":       n.Type,
		"timestamp":  time.Now().Format(time.RFC3339),
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(w.url, "application/json", bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}
