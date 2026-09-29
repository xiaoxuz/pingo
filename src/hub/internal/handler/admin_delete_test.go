package handler

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestDeleteAgentPreservesMessagesAndRemovesIdentity(t *testing.T) {
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
	peerID := "agt_" + uuid.NewString()
	conversationID := "conv-" + uuid.NewString()
	messageID := "msg-" + uuid.NewString()
	for _, id := range []string{agentID, peerID} {
		if _, err := db.Exec(ctx, `INSERT INTO agents(agent_id,name,token) VALUES($1,$2,$1)`, id, id); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		db.Exec(ctx, `DELETE FROM conversations WHERE conversation_id=$1`, conversationID)
		db.Exec(ctx, `DELETE FROM agents WHERE agent_id IN ($1,$2)`, agentID, peerID)
	})
	if _, err := db.Exec(ctx, `INSERT INTO conversations(conversation_id,type,created_by) VALUES($1,'group',$2)`, conversationID, agentID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO conversation_members(conversation_id,agent_id) VALUES($1,$2),($1,$3)`, conversationID, agentID, peerID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO messages(message_id,conversation_id,from_agent,message_type,content_text) VALUES($1,$2,$3,'text','hello')`, messageID, conversationID, agentID); err != nil {
		t.Fatal(err)
	}
	handler := NewAdminHandler(db, nil)
	if err := handler.deleteAgent(ctx, "tester", agentID, "127.0.0.1"); err != nil {
		t.Fatal(err)
	}
	var agents, messages, members int
	var sender *string
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM agents WHERE agent_id=$1`, agentID).Scan(&agents); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `SELECT COUNT(*),MAX(from_agent) FROM messages WHERE message_id=$1`, messageID).Scan(&messages, &sender); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM conversation_members WHERE conversation_id=$1`, conversationID).Scan(&members); err != nil {
		t.Fatal(err)
	}
	if agents != 0 || messages != 1 || sender != nil || members != 1 {
		t.Fatalf("agents=%d messages=%d sender=%v members=%d", agents, messages, sender, members)
	}
}

func TestDeleteConversationRemovesConversationData(t *testing.T) {
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
	messageID := "msg-" + uuid.NewString()
	if _, err := db.Exec(ctx, `INSERT INTO agents(agent_id,name,token) VALUES($1,$1,$1)`, agentID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Exec(ctx, `DELETE FROM agents WHERE agent_id=$1`, agentID) })
	if _, err := db.Exec(ctx, `INSERT INTO conversations(conversation_id,type,created_by) VALUES($1,'direct',$2)`, conversationID, agentID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO conversation_members(conversation_id,agent_id) VALUES($1,$2)`, conversationID, agentID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO messages(message_id,conversation_id,from_agent,message_type) VALUES($1,$2,$3,'text')`, messageID, conversationID, agentID); err != nil {
		t.Fatal(err)
	}
	handler := NewAdminHandler(db, nil)
	if _, err := handler.deleteConversation(ctx, "tester", conversationID, "127.0.0.1"); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM conversations WHERE conversation_id=$1`, conversationID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("conversation remains: %d", count)
	}
}
