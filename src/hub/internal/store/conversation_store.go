package store

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pingo/hub/internal/model"
)

type ConversationStore struct {
	db *pgxpool.Pool
}

func NewConversationStore(db *pgxpool.Pool) *ConversationStore {
	return &ConversationStore{db: db}
}

func (s *ConversationStore) GenerateID() string {
	return "conv-" + uuid.New().String()[:16]
}

func (s *ConversationStore) Create(ctx context.Context, conv *model.Conversation, members []*model.ConversationMember) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	query := `
		INSERT INTO conversations (conversation_id, type, name, created_by, status, created_at)
		VALUES ($1, $2, $3, $4, $5, NOW())
	`
	_, err = tx.Exec(ctx, query, conv.ConversationID, conv.Type, conv.Name, conv.CreatedBy, conv.Status)
	if err != nil {
		return fmt.Errorf("create conversation: %w", err)
	}

	for _, m := range members {
		_, err = tx.Exec(ctx, `
			INSERT INTO conversation_members (conversation_id, agent_id, role, joined_at, unread_count)
			VALUES ($1, $2, $3, NOW(), 0)
		`, m.ConversationID, m.AgentID, m.Role)
		if err != nil {
			return fmt.Errorf("add member: %w", err)
		}
	}

	return tx.Commit(ctx)
}

func (s *ConversationStore) GetByID(ctx context.Context, conversationID string) (*model.Conversation, error) {
	query := `
		SELECT conversation_id, type, COALESCE(name, ''), created_by, status, created_at,
		       closed_at, last_message_at, COALESCE(last_message_preview, '')
		FROM conversations WHERE conversation_id = $1
	`
	var c model.Conversation
	err := s.db.QueryRow(ctx, query, conversationID).Scan(
		&c.ConversationID, &c.Type, &c.Name, &c.CreatedBy, &c.Status,
		&c.CreatedAt, &c.ClosedAt, &c.LastMessageAt, &c.LastMessagePreview,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get conversation: %w", err)
	}
	return &c, nil
}

func (s *ConversationStore) FindDirectConv(ctx context.Context, agentA string, agentB string) (*model.Conversation, error) {
	query := `
		SELECT c.conversation_id, c.type, COALESCE(c.name, ''), c.created_by, c.status,
		       c.created_at, c.closed_at, c.last_message_at, COALESCE(c.last_message_preview, '')
		FROM conversations c
		JOIN conversation_members m1 ON c.conversation_id = m1.conversation_id
		JOIN conversation_members m2 ON c.conversation_id = m2.conversation_id
		WHERE c.type = 'direct' AND c.status = 'active'
		  AND m1.agent_id = $1 AND m1.left_at IS NULL
		  AND m2.agent_id = $2 AND m2.left_at IS NULL
		LIMIT 1
	`
	var c model.Conversation
	err := s.db.QueryRow(ctx, query, agentA, agentB).Scan(
		&c.ConversationID, &c.Type, &c.Name, &c.CreatedBy, &c.Status,
		&c.CreatedAt, &c.ClosedAt, &c.LastMessageAt, &c.LastMessagePreview,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("find direct conv: %w", err)
	}
	return &c, nil
}

func (s *ConversationStore) IsMember(ctx context.Context, conversationID string, agentID string) (bool, error) {
	query := `
		SELECT COUNT(*) FROM conversation_members
		WHERE conversation_id = $1 AND agent_id = $2 AND left_at IS NULL
	`
	var count int
	err := s.db.QueryRow(ctx, query, conversationID, agentID).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *ConversationStore) GetMember(ctx context.Context, conversationID string, agentID string) (*model.ConversationMember, error) {
	query := `
		SELECT conversation_id, agent_id, role, joined_at, left_at,
		       COALESCE(last_read_message_id, ''), unread_count
		FROM conversation_members
		WHERE conversation_id = $1 AND agent_id = $2
	`
	var m model.ConversationMember
	err := s.db.QueryRow(ctx, query, conversationID, agentID).Scan(
		&m.ConversationID, &m.AgentID, &m.Role, &m.JoinedAt, &m.LeftAt,
		&m.LastReadMessageID, &m.UnreadCount,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &m, nil
}

func (s *ConversationStore) ListMembers(ctx context.Context, conversationID string) ([]*model.ConversationMember, error) {
	query := `
		SELECT conversation_id, agent_id, role, joined_at, left_at,
		       COALESCE(last_read_message_id, ''), unread_count
		FROM conversation_members WHERE conversation_id = $1
		ORDER BY joined_at
	`
	rows, err := s.db.Query(ctx, query, conversationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	members := []*model.ConversationMember{}
	for rows.Next() {
		var m model.ConversationMember
		if err := rows.Scan(
			&m.ConversationID, &m.AgentID, &m.Role, &m.JoinedAt, &m.LeftAt,
			&m.LastReadMessageID, &m.UnreadCount,
		); err != nil {
			return nil, err
		}
		members = append(members, &m)
	}
	return members, nil
}

func (s *ConversationStore) ListActiveMembers(ctx context.Context, conversationID string) ([]*model.ConversationMember, error) {
	query := `
		SELECT conversation_id, agent_id, role, joined_at, left_at,
		       COALESCE(last_read_message_id, ''), unread_count
		FROM conversation_members
		WHERE conversation_id = $1 AND left_at IS NULL
		ORDER BY joined_at
	`
	rows, err := s.db.Query(ctx, query, conversationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	members := []*model.ConversationMember{}
	for rows.Next() {
		var m model.ConversationMember
		if err := rows.Scan(
			&m.ConversationID, &m.AgentID, &m.Role, &m.JoinedAt, &m.LeftAt,
			&m.LastReadMessageID, &m.UnreadCount,
		); err != nil {
			return nil, err
		}
		members = append(members, &m)
	}
	return members, nil
}

func (s *ConversationStore) AddMember(ctx context.Context, conversationID string, agentID string, role string) error {
	query := `
		INSERT INTO conversation_members (conversation_id, agent_id, role, joined_at, unread_count)
		VALUES ($1, $2, $3, NOW(), 0)
	`
	_, err := s.db.Exec(ctx, query, conversationID, agentID, role)
	return err
}

func (s *ConversationStore) LeaveMember(ctx context.Context, conversationID string, agentID string) error {
	query := `
		UPDATE conversation_members SET left_at = NOW()
		WHERE conversation_id = $1 AND agent_id = $2 AND left_at IS NULL
	`
	_, err := s.db.Exec(ctx, query, conversationID, agentID)
	return err
}

func (s *ConversationStore) Close(ctx context.Context, conversationID string, closedBy string) error {
	query := `
		UPDATE conversations SET status = 'closed', closed_at = NOW()
		WHERE conversation_id = $1
	`
	_, err := s.db.Exec(ctx, query, conversationID)
	return err
}

func (s *ConversationStore) ListByAgent(ctx context.Context, agentID string, status string) ([]*model.Conversation, error) {
	query := `
		SELECT c.conversation_id, c.type, COALESCE(c.name, ''), c.created_by, c.status,
		       c.created_at, c.closed_at, c.last_message_at, COALESCE(c.last_message_preview, '')
		FROM conversations c
		JOIN conversation_members m ON c.conversation_id = m.conversation_id
		WHERE m.agent_id = $1 AND m.left_at IS NULL
	`
	args := []interface{}{agentID}

	if status != "" && status != "all" {
		query += " AND c.status = $2"
		args = append(args, status)
	}

	query += " ORDER BY c.last_message_at DESC NULLS LAST, c.created_at DESC"

	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	convs := []*model.Conversation{}
	for rows.Next() {
		var c model.Conversation
		if err := rows.Scan(
			&c.ConversationID, &c.Type, &c.Name, &c.CreatedBy, &c.Status,
			&c.CreatedAt, &c.ClosedAt, &c.LastMessageAt, &c.LastMessagePreview,
		); err != nil {
			return nil, err
		}
		convs = append(convs, &c)
	}
	return convs, nil
}

func (s *ConversationStore) IncrementUnread(ctx context.Context, conversationID string, agentID string) error {
	query := `
		UPDATE conversation_members SET unread_count = unread_count + 1
		WHERE conversation_id = $1 AND agent_id = $2 AND left_at IS NULL
	`
	_, err := s.db.Exec(ctx, query, conversationID, agentID)
	return err
}

func (s *ConversationStore) GetUnreadCount(ctx context.Context, conversationID string, agentID string) (int, error) {
	query := `
		SELECT unread_count FROM conversation_members
		WHERE conversation_id = $1 AND agent_id = $2
	`
	var count int
	err := s.db.QueryRow(ctx, query, conversationID, agentID).Scan(&count)
	if err != nil {
		if err == pgx.ErrNoRows {
			return 0, nil
		}
		return 0, err
	}
	return count, nil
}

func (s *ConversationStore) MarkRead(ctx context.Context, conversationID string, agentID string, messageID string) error {
	query := `
		UPDATE conversation_members
		SET last_read_message_id = $1, unread_count = 0
		WHERE conversation_id = $2 AND agent_id = $3
	`
	_, err := s.db.Exec(ctx, query, messageID, conversationID, agentID)
	return err
}

func (s *ConversationStore) UpdateLastMessage(ctx context.Context, conversationID string, preview string) error {
	query := `
		UPDATE conversations SET last_message_at = NOW(), last_message_preview = $1
		WHERE conversation_id = $2
	`
	_, err := s.db.Exec(ctx, query, preview, conversationID)
	return err
}

func (s *ConversationStore) ActiveMemberCount(ctx context.Context, conversationID string) (int, error) {
	query := `
		SELECT COUNT(*) FROM conversation_members
		WHERE conversation_id = $1 AND left_at IS NULL
	`
	var count int
	err := s.db.QueryRow(ctx, query, conversationID).Scan(&count)
	return count, err
}

// GetMemberRole 获取成员角色
func (s *ConversationStore) GetMemberRole(ctx context.Context, conversationID string, agentID string) (string, error) {
	query := `
		SELECT role FROM conversation_members
		WHERE conversation_id = $1 AND agent_id = $2
	`
	var role string
	err := s.db.QueryRow(ctx, query, conversationID, agentID).Scan(&role)
	if err != nil {
		if err == pgx.ErrNoRows {
			return "", nil
		}
		return "", err
	}
	return role, nil
}
