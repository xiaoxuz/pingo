package hub

import (
	"strings"
	"testing"
	"time"

	"github.com/pingo/sidecar/internal/notify"
	"github.com/pingo/sidecar/internal/session"
	"github.com/pingo/sidecar/internal/store"
)

func TestFriendRequestWakesOnlyMatchingSession(t *testing.T) {
	storage, err := store.NewSQLiteStore(t.TempDir() + "/pingo.db")
	if err != nil {
		t.Fatal(err)
	}
	defer storage.Close()
	registry := session.NewRegistry()
	registry.Register(session.RegisterRequest{ID: "recipient", AgentID: "recipient", Provider: "claude"})
	registry.Register(session.RegisterRequest{ID: "other", AgentID: "other", Provider: "claude"})
	receiver := NewMessageReceiver(storage, notify.NewManager(), nil, registry, nil)
	receiver.HandleMessage("recipient", MessageEnvelope{
		Type: "friend.request.notify",
		Data: map[string]any{"from_agent": "sender", "name": "Sender", "message": "hello", "created_at": "now"},
	})
	select {
	case event := <-registry.Subscribe("recipient"):
		if event.Type != "friend_request" || event.Message != "Sender: hello" {
			t.Fatalf("unexpected event: %+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("expected friend request event")
	}
	select {
	case event := <-registry.Subscribe("other"):
		t.Fatalf("wrong agent received event: %+v", event)
	default:
	}
}

func TestMessageWakePreviewAllowsOneThousandUnicodeCharacters(t *testing.T) {
	for _, test := range []struct {
		name, message string
		wantRunes     int
		truncated     bool
	}{
		{"long chinese preserved", strings.Repeat("中", 500), 500, false},
		{"over limit truncated", strings.Repeat("界", 1001), 1003, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			preview := messagePreview(test.message)
			if len([]rune(preview)) != test.wantRunes {
				t.Fatalf("preview rune count = %d", len([]rune(preview)))
			}
			if strings.HasSuffix(preview, "...") != test.truncated {
				t.Fatalf("preview truncated = %v", strings.HasSuffix(preview, "..."))
			}
		})
	}
}
