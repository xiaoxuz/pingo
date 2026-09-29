package store

import (
	"context"
	"fmt"
	"time"

	"github.com/pingo/hub/internal/model"
	"github.com/jackc/pgx/v5/pgxpool"
)

type OfflineStore struct {
	db *pgxpool.Pool
}

func NewOfflineStore(db *pgxpool.Pool) *OfflineStore {
	return &OfflineStore{db: db}
}

func (s *OfflineStore) Enqueue(ctx context.Context, targetAgent string, messageID string, conversationID string) error {
	query := `
		INSERT INTO offline_queue (target_agent, message_id, conversation_id, queued_at, delivered)
		VALUES ($1, $2, $3, NOW(), FALSE)
	`
	_, err := s.db.Exec(ctx, query, targetAgent, messageID, conversationID)
	if err != nil {
		return fmt.Errorf("enqueue offline: %w", err)
	}
	return nil
}

func (s *OfflineStore) GetPending(ctx context.Context, targetAgent string) ([]*model.OfflineQueueItem, error) {
	query := `
		SELECT id, target_agent, message_id, conversation_id, queued_at, delivered, delivered_at
		FROM offline_queue
		WHERE target_agent = $1 AND NOT delivered
		ORDER BY queued_at ASC
	`
	rows, err := s.db.Query(ctx, query, targetAgent)
	if err != nil {
		return nil, fmt.Errorf("get pending offline: %w", err)
	}
	defer rows.Close()

	items := []*model.OfflineQueueItem{}
	for rows.Next() {
		var item model.OfflineQueueItem
		if err := rows.Scan(
			&item.ID, &item.TargetAgent, &item.MessageID, &item.ConversationID,
			&item.QueuedAt, &item.Delivered, &item.DeliveredAt,
		); err != nil {
			return nil, fmt.Errorf("scan offline item: %w", err)
		}
		items = append(items, &item)
	}
	return items, nil
}

func (s *OfflineStore) MarkDelivered(ctx context.Context, id int) error {
	query := `
		UPDATE offline_queue SET delivered = TRUE, delivered_at = NOW()
		WHERE id = $1
	`
	_, err := s.db.Exec(ctx, query, id)
	return err
}

func (s *OfflineStore) MarkAllDelivered(ctx context.Context, targetAgent string) error {
	query := `
		UPDATE offline_queue SET delivered = TRUE, delivered_at = NOW()
		WHERE target_agent = $1 AND NOT delivered
	`
	_, err := s.db.Exec(ctx, query, targetAgent)
	return err
}

// CleanupOld 清理过期的已投递消息
func (s *OfflineStore) CleanupOld(ctx context.Context, maxAge time.Duration) (int64, error) {
	query := `
		DELETE FROM offline_queue
		WHERE delivered AND delivered_at < NOW() - $1::interval
	`
	cmdTag, err := s.db.Exec(ctx, query, maxAge.String())
	if err != nil {
		return 0, err
	}
	return cmdTag.RowsAffected(), nil
}

// GetPendingCount 获取待投递数量
func (s *OfflineStore) GetPendingCount(ctx context.Context, targetAgent string) (int, error) {
	query := `SELECT COUNT(*) FROM offline_queue WHERE target_agent = $1 AND NOT delivered`
	var count int
	err := s.db.QueryRow(ctx, query, targetAgent).Scan(&count)
	return count, err
}
