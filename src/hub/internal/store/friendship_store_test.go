package store

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestFriendshipNullableFields(t *testing.T) {
	url := os.Getenv("PINGO_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set PINGO_TEST_DATABASE_URL to a disposable, migrated database")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	first := "agt_" + uuid.NewString()
	second := "agt_" + uuid.NewString()
	for _, agentID := range []string{first, second} {
		if _, err := pool.Exec(ctx, `INSERT INTO agents (agent_id, name, token) VALUES ($1, 'test', $2)`, agentID, agentID); err != nil {
			t.Fatal(err)
		}
		defer pool.Exec(ctx, `DELETE FROM agents WHERE agent_id = $1`, agentID)
	}
	defer pool.Exec(ctx, `DELETE FROM friendships WHERE agent_a = $1 OR agent_b = $1`, first)

	store := NewFriendshipStore(pool)
	created, err := store.CreateRequest(ctx, first, second, "")
	if err != nil {
		t.Fatal(err)
	}
	if created.Status != "pending" || created.ANicknameForB != "" || created.BGroupForA != "" {
		t.Fatalf("unexpected created friendship: %+v", created)
	}
	got, err := store.GetByAgents(ctx, second, first)
	if err != nil || got == nil || got.ID != created.ID {
		t.Fatalf("get pending friendship: %+v, %v", got, err)
	}
	requests, err := store.ListReceivedRequests(ctx, second)
	if err != nil || len(requests) != 1 || requests[0].ID != created.ID {
		t.Fatalf("list requests: %+v, %v", requests, err)
	}
	if err := store.Accept(ctx, first, second); err != nil {
		t.Fatal(err)
	}
	friends, err := store.ListFriends(ctx, first)
	if err != nil || len(friends) != 1 || friends[0].ID != created.ID {
		t.Fatalf("list friends: %+v, %v", friends, err)
	}
}
