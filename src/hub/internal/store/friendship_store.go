package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pingo/hub/internal/model"
)

type FriendshipStore struct {
	db *pgxpool.Pool
}

func NewFriendshipStore(db *pgxpool.Pool) *FriendshipStore {
	return &FriendshipStore{db: db}
}

// normalizeOrder 确保 agent_a < agent_b
func normalizeOrder(a, b string) (string, string) {
	if a < b {
		return a, b
	}
	return b, a
}

func (s *FriendshipStore) CreateRequest(ctx context.Context, from string, target string, message string) (*model.Friendship, error) {
	agentA, agentB := normalizeOrder(from, target)
	initiatedBy := from

	query := `
		INSERT INTO friendships (agent_a, agent_b, status, initiated_by, request_message)
		VALUES ($1, $2, 'pending', $3, $4)
		RETURNING id, agent_a, agent_b, status, COALESCE(initiated_by, ''), COALESCE(request_message, ''),
		          COALESCE(a_nickname_for_b, ''), COALESCE(a_group_for_b, ''), a_trust_level, a_trust_rules,
		          COALESCE(b_nickname_for_a, ''), COALESCE(b_group_for_a, ''), b_trust_level, b_trust_rules,
		          interaction_count, last_interaction, created_at, updated_at
	`
	var f model.Friendship
	err := s.db.QueryRow(ctx, query, agentA, agentB, initiatedBy, message).Scan(
		&f.ID, &f.AgentA, &f.AgentB, &f.Status, &f.InitiatedBy, &f.RequestMessage,
		&f.ANicknameForB, &f.AGroupForB, &f.ATrustLevel, &f.ATrustRules,
		&f.BNicknameForA, &f.BGroupForA, &f.BTrustLevel, &f.BTrustRules,
		&f.InteractionCount, &f.LastInteraction, &f.CreatedAt, &f.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create friend request: %w", err)
	}
	return &f, nil
}

func (s *FriendshipStore) GetByAgents(ctx context.Context, agentA string, agentB string) (*model.Friendship, error) {
	a, b := normalizeOrder(agentA, agentB)
	query := `
		SELECT id, agent_a, agent_b, status, COALESCE(initiated_by, ''), COALESCE(request_message, ''),
		       COALESCE(a_nickname_for_b, ''), COALESCE(a_group_for_b, ''), a_trust_level, a_trust_rules,
		       COALESCE(b_nickname_for_a, ''), COALESCE(b_group_for_a, ''), b_trust_level, b_trust_rules,
		       interaction_count, last_interaction, created_at, updated_at
		FROM friendships WHERE agent_a = $1 AND agent_b = $2
	`
	var f model.Friendship
	err := s.db.QueryRow(ctx, query, a, b).Scan(
		&f.ID, &f.AgentA, &f.AgentB, &f.Status, &f.InitiatedBy, &f.RequestMessage,
		&f.ANicknameForB, &f.AGroupForB, &f.ATrustLevel, &f.ATrustRules,
		&f.BNicknameForA, &f.BGroupForA, &f.BTrustLevel, &f.BTrustRules,
		&f.InteractionCount, &f.LastInteraction, &f.CreatedAt, &f.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get friendship: %w", err)
	}
	return &f, nil
}

func (s *FriendshipStore) Accept(ctx context.Context, agentA string, agentB string) error {
	a, b := normalizeOrder(agentA, agentB)
	query := `
		UPDATE friendships SET status = 'accepted', updated_at = NOW()
		WHERE agent_a = $1 AND agent_b = $2 AND status = 'pending'
	`
	cmdTag, err := s.db.Exec(ctx, query, a, b)
	if err != nil {
		return fmt.Errorf("accept friend: %w", err)
	}
	if cmdTag.RowsAffected() == 0 {
		return fmt.Errorf("no pending request found")
	}
	return nil
}

func (s *FriendshipStore) Reject(ctx context.Context, agentA string, agentB string) error {
	a, b := normalizeOrder(agentA, agentB)
	query := `DELETE FROM friendships WHERE agent_a = $1 AND agent_b = $2 AND status = 'pending'`
	_, err := s.db.Exec(ctx, query, a, b)
	if err != nil {
		return fmt.Errorf("reject friend: %w", err)
	}
	return nil
}

func (s *FriendshipStore) Remove(ctx context.Context, agentA string, agentB string) error {
	a, b := normalizeOrder(agentA, agentB)
	query := `DELETE FROM friendships WHERE agent_a = $1 AND agent_b = $2 AND status = 'accepted'`
	_, err := s.db.Exec(ctx, query, a, b)
	if err != nil {
		return fmt.Errorf("remove friend: %w", err)
	}
	return nil
}

func (s *FriendshipStore) Block(ctx context.Context, blocker string, target string) error {
	a, b := normalizeOrder(blocker, target)

	var query string
	if blocker == a {
		query = `UPDATE friendships SET a_trust_level = 'blocked', updated_at = NOW() WHERE agent_a = $1 AND agent_b = $2`
	} else {
		query = `UPDATE friendships SET b_trust_level = 'blocked', updated_at = NOW() WHERE agent_a = $1 AND agent_b = $2`
	}
	_, err := s.db.Exec(ctx, query, a, b)
	return err
}

func (s *FriendshipStore) SetNickname(ctx context.Context, viewer string, target string, nickname string) error {
	a, b := normalizeOrder(viewer, target)
	var query string
	if viewer == a {
		query = `UPDATE friendships SET a_nickname_for_b = $1, updated_at = NOW() WHERE agent_a = $2 AND agent_b = $3`
	} else {
		query = `UPDATE friendships SET b_nickname_for_a = $1, updated_at = NOW() WHERE agent_a = $2 AND agent_b = $3`
	}
	_, err := s.db.Exec(ctx, query, nickname, a, b)
	return err
}

func (s *FriendshipStore) SetGroup(ctx context.Context, viewer string, target string, group string) error {
	a, b := normalizeOrder(viewer, target)
	var query string
	if viewer == a {
		query = `UPDATE friendships SET a_group_for_b = $1, updated_at = NOW() WHERE agent_a = $2 AND agent_b = $3`
	} else {
		query = `UPDATE friendships SET b_group_for_a = $1, updated_at = NOW() WHERE agent_a = $2 AND agent_b = $3`
	}
	_, err := s.db.Exec(ctx, query, group, a, b)
	return err
}

func (s *FriendshipStore) SetTrust(ctx context.Context, viewer string, target string, level string) error {
	a, b := normalizeOrder(viewer, target)
	var query string
	if viewer == a {
		query = `UPDATE friendships SET a_trust_level = $1, updated_at = NOW() WHERE agent_a = $2 AND agent_b = $3`
	} else {
		query = `UPDATE friendships SET b_trust_level = $1, updated_at = NOW() WHERE agent_a = $2 AND agent_b = $3`
	}
	_, err := s.db.Exec(ctx, query, level, a, b)
	return err
}

// ListFriends 列出某个 agent 的所有已接受好友
func (s *FriendshipStore) ListFriends(ctx context.Context, agentID string) ([]*model.Friendship, error) {
	query := `
		SELECT id, agent_a, agent_b, status, COALESCE(initiated_by, ''), COALESCE(request_message, ''),
		       COALESCE(a_nickname_for_b, ''), COALESCE(a_group_for_b, ''), a_trust_level, a_trust_rules,
		       COALESCE(b_nickname_for_a, ''), COALESCE(b_group_for_a, ''), b_trust_level, b_trust_rules,
		       interaction_count, last_interaction, created_at, updated_at
		FROM friendships
		WHERE (agent_a = $1 OR agent_b = $1) AND status = 'accepted'
		ORDER BY last_interaction DESC NULLS LAST, created_at DESC
	`
	rows, err := s.db.Query(ctx, query, agentID)
	if err != nil {
		return nil, fmt.Errorf("list friends: %w", err)
	}
	defer rows.Close()

	friends := []*model.Friendship{}
	for rows.Next() {
		var f model.Friendship
		err := rows.Scan(
			&f.ID, &f.AgentA, &f.AgentB, &f.Status, &f.InitiatedBy, &f.RequestMessage,
			&f.ANicknameForB, &f.AGroupForB, &f.ATrustLevel, &f.ATrustRules,
			&f.BNicknameForA, &f.BGroupForA, &f.BTrustLevel, &f.BTrustRules,
			&f.InteractionCount, &f.LastInteraction, &f.CreatedAt, &f.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan friend: %w", err)
		}
		friends = append(friends, &f)
	}
	return friends, nil
}

// ListReceivedRequests 列出收到的好友请求
func (s *FriendshipStore) ListReceivedRequests(ctx context.Context, agentID string) ([]*model.Friendship, error) {
	query := `
		SELECT f.id, f.agent_a, f.agent_b, f.status, COALESCE(f.initiated_by, ''), COALESCE(f.request_message, ''),
		       COALESCE(f.a_nickname_for_b, ''), COALESCE(f.a_group_for_b, ''), f.a_trust_level, f.a_trust_rules,
		       COALESCE(f.b_nickname_for_a, ''), COALESCE(f.b_group_for_a, ''), f.b_trust_level, f.b_trust_rules,
		       f.interaction_count, f.last_interaction, f.created_at, f.updated_at
		FROM friendships f
		WHERE f.status = 'pending'
		  AND f.initiated_by != $1
		  AND (f.agent_a = $1 OR f.agent_b = $1)
		ORDER BY f.created_at DESC
	`
	rows, err := s.db.Query(ctx, query, agentID)
	if err != nil {
		return nil, fmt.Errorf("list received requests: %w", err)
	}
	defer rows.Close()

	requests := []*model.Friendship{}
	for rows.Next() {
		var f model.Friendship
		err := rows.Scan(
			&f.ID, &f.AgentA, &f.AgentB, &f.Status, &f.InitiatedBy, &f.RequestMessage,
			&f.ANicknameForB, &f.AGroupForB, &f.ATrustLevel, &f.ATrustRules,
			&f.BNicknameForA, &f.BGroupForA, &f.BTrustLevel, &f.BTrustRules,
			&f.InteractionCount, &f.LastInteraction, &f.CreatedAt, &f.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan request: %w", err)
		}
		requests = append(requests, &f)
	}
	return requests, nil
}

// IsFriend 检查两个 agent 是否是好友
func (s *FriendshipStore) IsFriend(ctx context.Context, agentA string, agentB string) (bool, error) {
	f, err := s.GetByAgents(ctx, agentA, agentB)
	if err != nil {
		return false, err
	}
	if f == nil {
		return false, nil
	}
	return f.Status == model.FriendStatusAccepted, nil
}

// GetTrustLevel 返回 viewer 对 target 的信任等级
func (s *FriendshipStore) GetTrustLevel(ctx context.Context, viewer string, target string) (string, error) {
	f, err := s.GetByAgents(ctx, viewer, target)
	if err != nil {
		return "", err
	}
	if f == nil {
		return "", nil
	}
	_, _, _, trustLevel, _ := f.GetFriendSide(viewer)
	return trustLevel, nil
}

// IsBlockedBy 检查 target 是否被 viewer 拉黑
func (s *FriendshipStore) IsBlockedBy(ctx context.Context, viewer string, target string) (bool, error) {
	level, err := s.GetTrustLevel(ctx, viewer, target)
	if err != nil {
		return false, err
	}
	return level == model.TrustLevelBlocked, nil
}

func (s *FriendshipStore) IncrementInteraction(ctx context.Context, agentA string, agentB string) error {
	a, b := normalizeOrder(agentA, agentB)
	query := `
		UPDATE friendships
		SET interaction_count = interaction_count + 1,
		    last_interaction = NOW(),
		    updated_at = NOW()
		WHERE agent_a = $1 AND agent_b = $2 AND status = 'accepted'
	`
	_, err := s.db.Exec(ctx, query, a, b)
	return err
}

// ListFriendGroups 列出好友分组
func (s *FriendshipStore) ListFriendGroups(ctx context.Context, agentID string) ([]*model.FriendGroup, error) {
	query := `
		SELECT agent_id, group_name, sort_order
		FROM friend_groups WHERE agent_id = $1 ORDER BY sort_order, group_name
	`
	rows, err := s.db.Query(ctx, query, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	groups := []*model.FriendGroup{}
	for rows.Next() {
		var g model.FriendGroup
		if err := rows.Scan(&g.AgentID, &g.GroupName, &g.SortOrder); err != nil {
			return nil, err
		}
		groups = append(groups, &g)
	}
	return groups, nil
}

// UpsertFriendGroup 添加或更新好友分组
func (s *FriendshipStore) UpsertFriendGroup(ctx context.Context, agentID string, groupName string, sortOrder int) error {
	query := `
		INSERT INTO friend_groups (agent_id, group_name, sort_order)
		VALUES ($1, $2, $3)
		ON CONFLICT (agent_id, group_name) DO UPDATE SET sort_order = EXCLUDED.sort_order
	`
	_, err := s.db.Exec(ctx, query, agentID, groupName, sortOrder)
	return err
}

func (s *FriendshipStore) DeleteFriendGroup(ctx context.Context, agentID string, groupName string) error {
	query := `DELETE FROM friend_groups WHERE agent_id = $1 AND group_name = $2`
	_, err := s.db.Exec(ctx, query, agentID, groupName)
	return err
}
