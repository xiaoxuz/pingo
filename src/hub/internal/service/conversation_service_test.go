package service

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pingo/hub/internal/model"
	"github.com/pingo/hub/internal/store"
)

func TestStartDirectExistingConversationIncrementsRecipientUnread(t *testing.T) {
	databaseURL := os.Getenv("PINGO_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set PINGO_TEST_DATABASE_URL to a disposable, migrated database")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	firstAgent := "agt_" + uuid.NewString()
	secondAgent := "agt_" + uuid.NewString()
	for _, agentID := range []string{firstAgent, secondAgent} {
		if _, err := pool.Exec(ctx, `INSERT INTO agents (agent_id, name, token) VALUES ($1, 'test', $2)`, agentID, agentID); err != nil {
			t.Fatal(err)
		}
	}

	conversationStore := store.NewConversationStore(pool)
	friendshipStore := store.NewFriendshipStore(pool)
	messageStore := store.NewMessageStore(pool)
	conversationID := conversationStore.GenerateID()
	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM messages WHERE conversation_id = $1`, conversationID)
		pool.Exec(ctx, `DELETE FROM conversations WHERE conversation_id = $1`, conversationID)
		pool.Exec(ctx, `DELETE FROM friendships WHERE agent_a IN ($1, $2) OR agent_b IN ($1, $2)`, firstAgent, secondAgent)
		pool.Exec(ctx, `DELETE FROM agents WHERE agent_id IN ($1, $2)`, firstAgent, secondAgent)
	})

	request, err := friendshipStore.CreateRequest(ctx, firstAgent, secondAgent, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := friendshipStore.Accept(ctx, request.AgentA, request.AgentB); err != nil {
		t.Fatal(err)
	}
	if err := conversationStore.Create(ctx, &model.Conversation{
		ConversationID: conversationID,
		Type:           model.ConvTypeDirect,
		CreatedBy:      firstAgent,
		Status:         model.ConvStatusActive,
	}, []*model.ConversationMember{
		{ConversationID: conversationID, AgentID: firstAgent, Role: model.MemberRoleCreator},
		{ConversationID: conversationID, AgentID: secondAgent, Role: model.MemberRoleMember},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE conversations SET last_message_preview = '' WHERE conversation_id = $1`, conversationID); err != nil {
		t.Fatal(err)
	}
	members, err := conversationStore.ListActiveMembers(ctx, conversationID)
	if err != nil {
		t.Fatalf("list active members: %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("active members = %d, want 2", len(members))
	}

	conversationService := NewConversationService(conversationStore, friendshipStore, messageStore)
	conversation, message, err := conversationService.StartDirect(ctx, secondAgent, firstAgent, "早上好")
	if err != nil {
		t.Fatal(err)
	}
	if conversation.ConversationID != conversationID || message.ContentText != "早上好" {
		t.Fatalf("unexpected result: conversation=%+v message=%+v", conversation, message)
	}

	unreadCount, err := conversationStore.GetUnreadCount(ctx, conversationID, firstAgent)
	if err != nil {
		t.Fatal(err)
	}
	if unreadCount != 1 {
		t.Fatalf("recipient unread count = %d, want 1", unreadCount)
	}
}
