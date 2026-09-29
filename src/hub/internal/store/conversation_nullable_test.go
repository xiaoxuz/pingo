package store

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestConversationWithoutLastMessageScans(t *testing.T) {
	databaseURL := os.Getenv("PINGO_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set PINGO_TEST_DATABASE_URL to a disposable, migrated database")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	agentID := "agt_" + uuid.NewString()
	conversationID := "conv-" + uuid.NewString()
	if _, err := db.Exec(ctx, `INSERT INTO agents(agent_id,name,token) VALUES ($1,'test',$2)`, agentID, agentID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Exec(ctx, `DELETE FROM conversations WHERE conversation_id=$1`, conversationID)
		db.Exec(ctx, `DELETE FROM agents WHERE agent_id=$1`, agentID)
	})
	if _, err := db.Exec(ctx, `INSERT INTO conversations(conversation_id,type,created_by) VALUES($1,'direct',$2)`, conversationID, agentID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO conversation_members(conversation_id,agent_id) VALUES($1,$2)`, conversationID, agentID); err != nil {
		t.Fatal(err)
	}
	store := NewConversationStore(db)
	conversation, err := store.GetByID(ctx, conversationID)
	if err != nil {
		t.Fatal(err)
	}
	if conversation.LastMessagePreview != "" || conversation.Name != "" {
		t.Fatalf("unexpected nullable fields: %+v", conversation)
	}
	listed, err := store.ListByAgent(ctx, agentID, "active")
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].ConversationID != conversationID {
		t.Fatalf("unexpected list: %+v", listed)
	}
}
