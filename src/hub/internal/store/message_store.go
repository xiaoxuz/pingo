package store

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pingo/hub/internal/model"
)

type MessageStore struct {
	db *pgxpool.Pool
}

func NewMessageStore(db *pgxpool.Pool) *MessageStore {
	return &MessageStore{db: db}
}

func (s *MessageStore) GenerateID() string {
	return "msg-" + uuid.New().String()[:20]
}

func (s *MessageStore) CanDownloadFile(ctx context.Context, storageKey, agentID string) (bool, error) {
	var allowed bool
	err := s.db.QueryRow(ctx, `SELECT EXISTS (
  SELECT 1 FROM messages m JOIN conversation_members cm ON cm.conversation_id = m.conversation_id
  WHERE m.message_type = 'file' AND m.content_file->>'storage_key' = $1
    AND cm.agent_id = $2 AND cm.left_at IS NULL
 )`, storageKey, agentID).Scan(&allowed)
	return allowed, err
}

func (s *MessageStore) Create(ctx context.Context, msg *model.Message) error {
	query := `
		INSERT INTO messages (message_id, conversation_id, from_agent, message_type,
		                      content_text, content_file, mentions, reply_to, created_at, system_event)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), $9)
		RETURNING created_at
	`
	err := s.db.QueryRow(ctx, query,
		msg.MessageID, msg.ConversationID, msg.FromAgent, msg.MessageType,
		msg.ContentText, msg.ContentFile, msg.Mentions, msg.ReplyTo, msg.SystemEvent,
	).Scan(&msg.CreatedAt)
	if err != nil {
		return fmt.Errorf("create message: %w", err)
	}
	return nil
}

func (s *MessageStore) GetByID(ctx context.Context, messageID string) (*model.Message, error) {
	query := `
		SELECT message_id, conversation_id, from_agent, message_type,
		       content_text, content_file, mentions, reply_to, created_at, system_event
		FROM messages WHERE message_id = $1
	`
	var m model.Message
	err := s.db.QueryRow(ctx, query, messageID).Scan(
		&m.MessageID, &m.ConversationID, &m.FromAgent, &m.MessageType,
		&m.ContentText, &m.ContentFile, &m.Mentions, &m.ReplyTo,
		&m.CreatedAt, &m.SystemEvent,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get message: %w", err)
	}
	return &m, nil
}

func (s *MessageStore) ListByConversation(ctx context.Context, conversationID string, afterMessageID string, limit int) ([]*model.Message, error) {
	if limit <= 0 {
		limit = 50
	}

	var args []interface{}
	argIdx := 1
	query := `
		SELECT message_id, conversation_id, from_agent, message_type,
		       content_text, content_file, mentions, reply_to, created_at, system_event
		FROM messages
		WHERE conversation_id = $1
	`
	args = append(args, conversationID)
	argIdx++

	if afterMessageID != "" {
		// 获取 afterMessageID 的创建时间，然后取之后的消息
		query += fmt.Sprintf(` AND created_at > (SELECT created_at FROM messages WHERE message_id = $%d)`, argIdx)
		args = append(args, afterMessageID)
		argIdx++
	}

	query += fmt.Sprintf(` ORDER BY created_at ASC LIMIT $%d`, argIdx)
	args = append(args, limit)

	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}
	defer rows.Close()

	msgs := []*model.Message{}
	for rows.Next() {
		var m model.Message
		if err := rows.Scan(
			&m.MessageID, &m.ConversationID, &m.FromAgent, &m.MessageType,
			&m.ContentText, &m.ContentFile, &m.Mentions, &m.ReplyTo,
			&m.CreatedAt, &m.SystemEvent,
		); err != nil {
			return nil, fmt.Errorf("scan message: %w", err)
		}
		msgs = append(msgs, &m)
	}
	return msgs, nil
}

// CreateSystemMessage 创建系统消息
func (s *MessageStore) CreateSystemMessage(ctx context.Context, conversationID string, event string, content string) (*model.Message, error) {
	msg := &model.Message{
		MessageID:      s.GenerateID(),
		ConversationID: conversationID,
		FromAgent:      "system",
		MessageType:    model.MsgTypeSystem,
		ContentText:    content,
		SystemEvent:    event,
	}
	err := s.Create(ctx, msg)
	if err != nil {
		return nil, err
	}
	return msg, nil
}

// GetLastMessage 获取会话最后一条消息
func (s *MessageStore) GetLastMessage(ctx context.Context, conversationID string) (*model.Message, error) {
	query := `
		SELECT message_id, conversation_id, from_agent, message_type,
		       content_text, content_file, mentions, reply_to, created_at, system_event
		FROM messages WHERE conversation_id = $1
		ORDER BY created_at DESC LIMIT 1
	`
	var m model.Message
	err := s.db.QueryRow(ctx, query, conversationID).Scan(
		&m.MessageID, &m.ConversationID, &m.FromAgent, &m.MessageType,
		&m.ContentText, &m.ContentFile, &m.Mentions, &m.ReplyTo,
		&m.CreatedAt, &m.SystemEvent,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &m, nil
}
