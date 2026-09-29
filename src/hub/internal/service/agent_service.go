package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/pingo/hub/internal/auth"
	"github.com/pingo/hub/internal/model"
	"github.com/pingo/hub/internal/store"
)

var (
	ErrAgentNotFound = errors.New("agent not found")
)

type AgentService struct {
	agentStore *store.AgentStore
	tokenStore *auth.TokenStore
}

func NewAgentService(agentStore *store.AgentStore, tokenStore *auth.TokenStore) *AgentService {
	return &AgentService{
		agentStore: agentStore,
		tokenStore: tokenStore,
	}
}

func newAgentID() string {
	return "agt_" + uuid.NewString()
}

func (s *AgentService) Register(ctx context.Context, agent model.Agent) (*model.Agent, string, error) {
	token, err := auth.GenerateToken()
	if err != nil {
		return nil, "", fmt.Errorf("generate token: %w", err)
	}

	agent.AgentID = newAgentID()
	agent.Token = token
	if len(agent.Capabilities) == 0 {
		agent.Capabilities = json.RawMessage("[]")
	}
	if len(agent.Availability) == 0 {
		agent.Availability = json.RawMessage("{}")
	}

	if err := s.agentStore.Create(ctx, &agent); err != nil {
		return nil, "", err
	}

	// 缓存 token
	s.tokenStore.Set(token, agent.AgentID)

	return &agent, token, nil
}

func (s *AgentService) GetByID(ctx context.Context, agentID string) (*model.Agent, error) {
	return s.agentStore.GetByID(ctx, agentID)
}

func (s *AgentService) ValidateToken(ctx context.Context, token string) (string, error) {
	// 先查缓存
	if agentID, ok := s.tokenStore.Get(token); ok {
		return agentID, nil
	}

	// 再查数据库
	agentID, err := s.agentStore.GetAgentIDByToken(ctx, token)
	if err != nil {
		return "", err
	}
	if agentID == "" {
		return "", auth.ErrInvalidToken
	}

	// 写入缓存
	s.tokenStore.Set(token, agentID)
	return agentID, nil
}

func (s *AgentService) UpdateStatus(ctx context.Context, agentID string, statusText string) error {
	existing, err := s.agentStore.GetByID(ctx, agentID)
	if err != nil {
		return err
	}
	if existing == nil {
		return ErrAgentNotFound
	}
	return s.agentStore.UpdateStatus(ctx, agentID, statusText)
}

func (s *AgentService) UpdateCapabilities(ctx context.Context, agentID string, capabilities []model.Capability) error {
	existing, err := s.agentStore.GetByID(ctx, agentID)
	if err != nil {
		return err
	}
	if existing == nil {
		return ErrAgentNotFound
	}
	return s.agentStore.UpdateCapabilities(ctx, agentID, capabilities)
}

func (s *AgentService) UpdateOwner(ctx context.Context, agentID string, ownerName string, ownerEmail string) error {
	existing, err := s.agentStore.GetByID(ctx, agentID)
	if err != nil {
		return err
	}
	if existing == nil {
		return ErrAgentNotFound
	}
	return s.agentStore.UpdateOwner(ctx, agentID, ownerName, ownerEmail)
}

func (s *AgentService) UpdateProfile(ctx context.Context, agentID string, name string, avatarURL string, statusText string) error {
	existing, err := s.agentStore.GetByID(ctx, agentID)
	if err != nil {
		return err
	}
	if existing == nil {
		return ErrAgentNotFound
	}
	if name == "" {
		return errors.New("name is required")
	}
	return s.agentStore.UpdateProfile(ctx, agentID, name, avatarURL, statusText)
}

func (s *AgentService) UpdateAvailability(ctx context.Context, agentID string, availability model.Availability) error {
	existing, err := s.agentStore.GetByID(ctx, agentID)
	if err != nil {
		return err
	}
	if existing == nil {
		return ErrAgentNotFound
	}
	if availability.MaxConcurrentConversations < 0 {
		return errors.New("max_concurrent_conversations must be non-negative")
	}
	return s.agentStore.UpdateAvailability(ctx, agentID, availability)
}

func (s *AgentService) SetOnline(ctx context.Context, agentID string, online bool) error {
	return s.agentStore.UpdateOnline(ctx, agentID, online)
}

func (s *AgentService) Heartbeat(ctx context.Context, agentID string) error {
	return s.agentStore.UpdateHeartbeat(ctx, agentID)
}

func (s *AgentService) Search(ctx context.Context, viewerID string, keyword string, skill string, tags []string, onlineOnly bool, limit int, offset int) ([]*model.Agent, []bool, int, error) {
	agents, total, err := s.agentStore.Search(ctx, keyword, skill, tags, onlineOnly, limit, offset)
	if err != nil {
		return nil, nil, 0, err
	}

	// 判断每个结果是不是 viewer 的好友
	isFriendList := make([]bool, len(agents))
	// 这里需要 friendship store，暂留接口
	// 实际在 friendship_service 中补充判断
	return agents, isFriendList, total, nil
}

func (s *AgentService) GetAll(ctx context.Context, limit int, offset int) ([]*model.Agent, int, error) {
	return s.agentStore.GetAll(ctx, limit, offset)
}

func (s *AgentService) ResetToken(ctx context.Context, agentID string) (string, error) {
	existing, err := s.agentStore.GetByID(ctx, agentID)
	if err != nil {
		return "", err
	}
	if existing == nil {
		return "", ErrAgentNotFound
	}

	newToken, err := auth.GenerateToken()
	if err != nil {
		return "", err
	}

	if err := s.agentStore.ResetToken(ctx, agentID, newToken); err != nil {
		return "", err
	}

	// 清除旧 token 缓存
	s.tokenStore.Delete(existing.Token)
	// 设置新 token 缓存
	s.tokenStore.Set(newToken, agentID)

	return newToken, nil
}

func (s *AgentService) GetStats(ctx context.Context) (map[string]interface{}, error) {
	return s.agentStore.GetStats(ctx)
}
