package local

import (
	"path/filepath"
	"testing"

	"github.com/pingo/sidecar/internal/store"
)

func TestSaveSentMessageStoresCurrentAgentMessageAndUpdatesConversation(t *testing.T) {
	db, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "sidecar.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	err = db.UpsertConversation("agent-a", store.ConversationRecord{
		ConversationID: "conv-1",
		Type:           "direct",
		Status:         "active",
		CreatedAt:      "2026-09-28T10:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}

	response := map[string]any{
		"message_id": "msg-1",
		"created_at": "2026-09-28T10:01:00Z",
	}
	if err := saveSentMessage(db, "agent-a", "conv-1", "text", "你好", nil, nil, "", response); err != nil {
		t.Fatal(err)
	}

	messages, err := db.ListMessages("agent-a", "conv-1", 50, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}
	message := messages[0]
	if message.MessageID != "msg-1" || message.FromAgent != "agent-a" || message.ContentText != "你好" {
		t.Fatalf("unexpected stored message: %+v", message)
	}

	conversation, err := db.GetConversation("agent-a", "conv-1")
	if err != nil {
		t.Fatal(err)
	}
	if conversation.LastMessagePreview != "你好" || conversation.LastMessageAt != "2026-09-28T10:01:00Z" {
		t.Fatalf("unexpected conversation summary: %+v", conversation)
	}

	if err := saveSentMessage(db, "agent-a", "conv-1", "text", "你好", nil, nil, "", response); err != nil {
		t.Fatal(err)
	}
	messages, err = db.ListMessages("agent-a", "conv-1", 50, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 {
		t.Fatalf("duplicate response should not duplicate message, got %d", len(messages))
	}
}

func TestSaveSentMessageUsesCurrentTimeWhenHubOmitsCreatedAt(t *testing.T) {
	db, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "sidecar.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := saveSentMessage(db, "agent-a", "conv-new", "text", "第一条消息", nil, nil, "", map[string]any{
		"message_id": "msg-first",
	}); err != nil {
		t.Fatal(err)
	}

	messages, err := db.ListMessages("agent-a", "conv-new", 50, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 || messages[0].CreatedAt == "" {
		t.Fatalf("expected first message with fallback timestamp, got %+v", messages)
	}
}

func TestSaveStartedDirectConversationCreatesConversationAndFirstMessage(t *testing.T) {
	db, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "sidecar.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	response := map[string]any{
		"conversation_id": "conv-new",
		"message_id":      "msg-first",
	}
	if err := saveStartedDirectConversation(db, "agent-a", "agent-b", "你好", response); err != nil {
		t.Fatal(err)
	}

	conversation, err := db.GetConversation("agent-a", "conv-new")
	if err != nil {
		t.Fatal(err)
	}
	if conversation == nil || conversation.Type != "direct" || conversation.LastMessagePreview != "你好" {
		t.Fatalf("unexpected conversation: %+v", conversation)
	}
	members, err := db.ListMembers("agent-a", "conv-new")
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 2 {
		t.Fatalf("expected both direct members, got %+v", members)
	}
	messages, err := db.ListMessages("agent-a", "conv-new", 50, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 || messages[0].FromAgent != "agent-a" {
		t.Fatalf("unexpected first message: %+v", messages)
	}
}
