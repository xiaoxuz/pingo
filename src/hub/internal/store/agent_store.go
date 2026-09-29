package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pingo/hub/internal/model"
)

type AgentStore struct {
	db *pgxpool.Pool
}

func NewAgentStore(db *pgxpool.Pool) *AgentStore {
	return &AgentStore{db: db}
}

func (s *AgentStore) Create(ctx context.Context, agent *model.Agent) error {
	query := `
		INSERT INTO agents (agent_id, name, token, owner_name, owner_email, avatar_url, status_text, capabilities, availability)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING created_at, updated_at
	`
	err := s.db.QueryRow(ctx, query,
		agent.AgentID, agent.Name, agent.Token,
		agent.OwnerName, agent.OwnerEmail, agent.AvatarURL,
		agent.StatusText, agent.Capabilities, agent.Availability,
	).Scan(&agent.CreatedAt, &agent.UpdatedAt)
	if err != nil {
		return fmt.Errorf("create agent: %w", err)
	}
	return nil
}

func (s *AgentStore) GetByID(ctx context.Context, agentID string) (*model.Agent, error) {
	query := `
		SELECT agent_id, name, token, COALESCE(owner_name, ''), COALESCE(owner_email, ''), COALESCE(avatar_url, ''),
		       COALESCE(status_text, ''), capabilities, availability, online, last_heartbeat,
		       created_at, updated_at
		FROM agents WHERE agent_id = $1
	`
	var a model.Agent
	err := s.db.QueryRow(ctx, query, agentID).Scan(
		&a.AgentID, &a.Name, &a.Token, &a.OwnerName, &a.OwnerEmail,
		&a.AvatarURL, &a.StatusText, &a.Capabilities, &a.Availability,
		&a.Online, &a.LastHeartbeat, &a.CreatedAt, &a.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get agent: %w", err)
	}
	return &a, nil
}

func (s *AgentStore) GetByToken(ctx context.Context, token string) (*model.Agent, error) {
	query := `
		SELECT agent_id, name, token, COALESCE(owner_name, ''), COALESCE(owner_email, ''), COALESCE(avatar_url, ''),
		       COALESCE(status_text, ''), capabilities, availability, online, last_heartbeat,
		       created_at, updated_at
		FROM agents WHERE token = $1
	`
	var a model.Agent
	err := s.db.QueryRow(ctx, query, token).Scan(
		&a.AgentID, &a.Name, &a.Token, &a.OwnerName, &a.OwnerEmail,
		&a.AvatarURL, &a.StatusText, &a.Capabilities, &a.Availability,
		&a.Online, &a.LastHeartbeat, &a.CreatedAt, &a.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get agent by token: %w", err)
	}
	return &a, nil
}

func (s *AgentStore) GetAgentIDByToken(ctx context.Context, token string) (string, error) {
	query := `SELECT agent_id FROM agents WHERE token = $1`
	var id string
	err := s.db.QueryRow(ctx, query, token).Scan(&id)
	if err != nil {
		if err == pgx.ErrNoRows {
			return "", nil
		}
		return "", fmt.Errorf("get agent id by token: %w", err)
	}
	return id, nil
}

func (s *AgentStore) UpdateStatus(ctx context.Context, agentID string, statusText string) error {
	query := `
		UPDATE agents SET status_text = $1, updated_at = NOW()
		WHERE agent_id = $2
	`
	_, err := s.db.Exec(ctx, query, statusText, agentID)
	if err != nil {
		return fmt.Errorf("update status: %w", err)
	}
	return nil
}

func (s *AgentStore) UpdateCapabilities(ctx context.Context, agentID string, capabilities []model.Capability) error {
	capsJSON, err := json.Marshal(capabilities)
	if err != nil {
		return fmt.Errorf("marshal capabilities: %w", err)
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// 更新 agents 表
	_, err = tx.Exec(ctx,
		`UPDATE agents SET capabilities = $1, updated_at = NOW() WHERE agent_id = $2`,
		capsJSON, agentID,
	)
	if err != nil {
		return fmt.Errorf("update agent capabilities: %w", err)
	}

	// 重建 capability_index
	_, err = tx.Exec(ctx, `DELETE FROM capability_index WHERE agent_id = $1`, agentID)
	if err != nil {
		return fmt.Errorf("clear capability index: %w", err)
	}

	for _, cap := range capabilities {
		for _, tag := range cap.Tags {
			_, err = tx.Exec(ctx,
				`INSERT INTO capability_index (agent_id, skill, tag) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`,
				agentID, cap.Skill, tag,
			)
			if err != nil {
				return fmt.Errorf("insert capability index: %w", err)
			}
		}
	}

	return tx.Commit(ctx)
}

func (s *AgentStore) UpdateOwner(ctx context.Context, agentID string, ownerName string, ownerEmail string) error {
	query := `
		UPDATE agents SET owner_name = $1, owner_email = $2, updated_at = NOW()
		WHERE agent_id = $3
	`
	_, err := s.db.Exec(ctx, query, ownerName, ownerEmail, agentID)
	if err != nil {
		return fmt.Errorf("update owner: %w", err)
	}
	return nil
}

func (s *AgentStore) UpdateProfile(ctx context.Context, agentID string, name string, avatarURL string, statusText string) error {
	query := `
		UPDATE agents SET name = $1, avatar_url = $2, status_text = $3, updated_at = NOW()
		WHERE agent_id = $4
	`
	_, err := s.db.Exec(ctx, query, name, avatarURL, statusText, agentID)
	if err != nil {
		return fmt.Errorf("update profile: %w", err)
	}
	return nil
}

func (s *AgentStore) UpdateAvailability(ctx context.Context, agentID string, availability model.Availability) error {
	availabilityJSON, err := json.Marshal(availability)
	if err != nil {
		return fmt.Errorf("marshal availability: %w", err)
	}
	_, err = s.db.Exec(ctx,
		`UPDATE agents SET availability = $1, updated_at = NOW() WHERE agent_id = $2`,
		availabilityJSON, agentID,
	)
	if err != nil {
		return fmt.Errorf("update availability: %w", err)
	}
	return nil
}

func (s *AgentStore) UpdateOnline(ctx context.Context, agentID string, online bool) error {
	query := `
		UPDATE agents SET online = $1, last_heartbeat = NOW(), updated_at = NOW()
		WHERE agent_id = $2
	`
	_, err := s.db.Exec(ctx, query, online, agentID)
	if err != nil {
		return fmt.Errorf("update online: %w", err)
	}
	return nil
}

func (s *AgentStore) UpdateHeartbeat(ctx context.Context, agentID string) error {
	query := `
		UPDATE agents SET last_heartbeat = NOW(), online = TRUE
		WHERE agent_id = $1
	`
	_, err := s.db.Exec(ctx, query, agentID)
	if err != nil {
		return fmt.Errorf("update heartbeat: %w", err)
	}
	return nil
}

func (s *AgentStore) Search(ctx context.Context, keyword string, skill string, tags []string, onlineOnly bool, limit int, offset int) ([]*model.Agent, int, error) {
	conditions := []string{}
	args := []interface{}{}
	argIdx := 1

	if keyword != "" {
		conditions = append(conditions, fmt.Sprintf("(a.agent_id ILIKE $%d OR a.name ILIKE $%d OR a.status_text ILIKE $%d)", argIdx, argIdx, argIdx))
		args = append(args, "%"+keyword+"%")
		argIdx++
	}

	if skill != "" {
		conditions = append(conditions, fmt.Sprintf("a.agent_id IN (SELECT agent_id FROM capability_index WHERE skill = $%d)", argIdx))
		args = append(args, skill)
		argIdx++
	}

	if len(tags) > 0 {
		placeholders := make([]string, len(tags))
		for i, tag := range tags {
			placeholders[i] = fmt.Sprintf("$%d", argIdx)
			args = append(args, tag)
			argIdx++
		}
		conditions = append(conditions, fmt.Sprintf("a.agent_id IN (SELECT agent_id FROM capability_index WHERE tag IN (%s))", joinStrings(placeholders, ",")))
	}

	if onlineOnly {
		conditions = append(conditions, "a.online = TRUE")
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + joinStrings(conditions, " AND ")
	}

	// count
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM agents a %s", whereClause)
	var total int
	err := s.db.QueryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count agents: %w", err)
	}

	// query
	if limit <= 0 {
		limit = 20
	}
	query := fmt.Sprintf(`
		SELECT a.agent_id, a.name, a.token, COALESCE(a.owner_name, ''), COALESCE(a.owner_email, ''),
		       COALESCE(a.avatar_url, ''), COALESCE(a.status_text, ''), a.capabilities, a.availability,
		       a.online, a.last_heartbeat, a.created_at, a.updated_at
		FROM agents a
		%s
		ORDER BY a.online DESC, a.name ASC
		LIMIT $%d OFFSET $%d
	`, whereClause, argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("search agents: %w", err)
	}
	defer rows.Close()

	agents := []*model.Agent{}
	for rows.Next() {
		var a model.Agent
		err := rows.Scan(
			&a.AgentID, &a.Name, &a.Token, &a.OwnerName, &a.OwnerEmail,
			&a.AvatarURL, &a.StatusText, &a.Capabilities, &a.Availability,
			&a.Online, &a.LastHeartbeat, &a.CreatedAt, &a.UpdatedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("scan agent: %w", err)
		}
		agents = append(agents, &a)
	}

	return agents, total, nil
}

func (s *AgentStore) GetAll(ctx context.Context, limit int, offset int) ([]*model.Agent, int, error) {
	var total int
	err := s.db.QueryRow(ctx, `SELECT COUNT(*) FROM agents`).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(ctx, `
		SELECT agent_id, name, token, COALESCE(owner_name, ''), COALESCE(owner_email, ''),
		       COALESCE(avatar_url, ''), COALESCE(status_text, ''), capabilities, availability,
		       online, last_heartbeat, created_at, updated_at
		FROM agents ORDER BY created_at DESC LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	agents := []*model.Agent{}
	for rows.Next() {
		var a model.Agent
		err := rows.Scan(
			&a.AgentID, &a.Name, &a.Token, &a.OwnerName, &a.OwnerEmail,
			&a.AvatarURL, &a.StatusText, &a.Capabilities, &a.Availability,
			&a.Online, &a.LastHeartbeat, &a.CreatedAt, &a.UpdatedAt,
		)
		if err != nil {
			return nil, 0, err
		}
		agents = append(agents, &a)
	}
	return agents, total, nil
}

func (s *AgentStore) ResetToken(ctx context.Context, agentID string, newToken string) error {
	query := `UPDATE agents SET token = $1, updated_at = NOW() WHERE agent_id = $2`
	_, err := s.db.Exec(ctx, query, newToken, agentID)
	return err
}

func (s *AgentStore) GetStats(ctx context.Context) (map[string]interface{}, error) {
	var totalAgents, onlineAgents int
	s.db.QueryRow(ctx, `SELECT COUNT(*) FROM agents`).Scan(&totalAgents)
	s.db.QueryRow(ctx, `SELECT COUNT(*) FROM agents WHERE online = TRUE`).Scan(&onlineAgents)

	var totalConvs, activeConvs int
	s.db.QueryRow(ctx, `SELECT COUNT(*) FROM conversations`).Scan(&totalConvs)
	s.db.QueryRow(ctx, `SELECT COUNT(*) FROM conversations WHERE status = 'active'`).Scan(&activeConvs)

	var totalMessages int
	s.db.QueryRow(ctx, `SELECT COUNT(*) FROM messages`).Scan(&totalMessages)

	return map[string]interface{}{
		"total_agents":         totalAgents,
		"online_agents":        onlineAgents,
		"total_conversations":  totalConvs,
		"active_conversations": activeConvs,
		"total_messages":       totalMessages,
		"collected_at":         time.Now(),
	}, nil
}

func joinStrings(items []string, sep string) string {
	result := ""
	for i, s := range items {
		if i > 0 {
			result += sep
		}
		result += s
	}
	return result
}
