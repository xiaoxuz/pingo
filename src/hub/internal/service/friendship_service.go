package service

import (
	"context"
	"errors"

	"github.com/pingo/hub/internal/model"
	"github.com/pingo/hub/internal/store"
)

var (
	ErrAlreadyFriends    = errors.New("already friends")
	ErrPendingRequest    = errors.New("pending request exists")
	ErrBlocked           = errors.New("blocked")
	ErrNotFriends        = errors.New("not friends")
	ErrNoPendingRequest  = errors.New("no pending request")
	ErrCannotAcceptSelf  = errors.New("cannot add self as friend")
)

type FriendshipService struct {
	friendshipStore *store.FriendshipStore
	agentStore      *store.AgentStore
}

func NewFriendshipService(friendshipStore *store.FriendshipStore, agentStore *store.AgentStore) *FriendshipService {
	return &FriendshipService{
		friendshipStore: friendshipStore,
		agentStore:      agentStore,
	}
}

// SendRequest 发送好友请求
func (s *FriendshipService) SendRequest(ctx context.Context, from string, target string, message string) (*model.Friendship, error) {
	if from == target {
		return nil, ErrCannotAcceptSelf
	}

	// 检查 target 是否存在
	targetAgent, err := s.agentStore.GetByID(ctx, target)
	if err != nil {
		return nil, err
	}
	if targetAgent == nil {
		return nil, ErrAgentNotFound
	}

	// 检查是否已有关系
	existing, err := s.friendshipStore.GetByAgents(ctx, from, target)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		if existing.Status == model.FriendStatusAccepted {
			return nil, ErrAlreadyFriends
		}
		if existing.Status == model.FriendStatusPending {
			return nil, ErrPendingRequest
		}
	}

	// 检查对方是否把自己拉黑了
	blocked, err := s.friendshipStore.IsBlockedBy(ctx, target, from)
	if err != nil {
		return nil, err
	}
	if blocked {
		// 静默处理，不告诉对方被拉黑了，但实际上不创建
		return nil, ErrBlocked
	}

	// 创建好友请求
	return s.friendshipStore.CreateRequest(ctx, from, target, message)
}

// Accept 接受好友请求
func (s *FriendshipService) Accept(ctx context.Context, agentID string, requester string) error {
	existing, err := s.friendshipStore.GetByAgents(ctx, agentID, requester)
	if err != nil {
		return err
	}
	if existing == nil {
		return ErrNoPendingRequest
	}
	if existing.Status != model.FriendStatusPending {
		return ErrNoPendingRequest
	}
	// 必须是对方发起的请求
	if existing.InitiatedBy != requester {
		return ErrNoPendingRequest
	}

	return s.friendshipStore.Accept(ctx, agentID, requester)
}

// Reject 拒绝好友请求
func (s *FriendshipService) Reject(ctx context.Context, agentID string, requester string) error {
	existing, err := s.friendshipStore.GetByAgents(ctx, agentID, requester)
	if err != nil {
		return err
	}
	if existing == nil {
		return ErrNoPendingRequest
	}
	if existing.Status != model.FriendStatusPending {
		return ErrNoPendingRequest
	}
	if existing.InitiatedBy != requester {
		return ErrNoPendingRequest
	}

	return s.friendshipStore.Reject(ctx, agentID, requester)
}

// Remove 删除好友
func (s *FriendshipService) Remove(ctx context.Context, agentID string, target string) error {
	isFriend, err := s.friendshipStore.IsFriend(ctx, agentID, target)
	if err != nil {
		return err
	}
	if !isFriend {
		return ErrNotFriends
	}
	return s.friendshipStore.Remove(ctx, agentID, target)
}

// Block 拉黑
func (s *FriendshipService) Block(ctx context.Context, agentID string, target string) error {
	existing, err := s.friendshipStore.GetByAgents(ctx, agentID, target)
	if err != nil {
		return err
	}
	if existing == nil {
		// 没有好友关系也可以拉黑，先创建一条 blocked 记录
		_, err := s.friendshipStore.CreateRequest(ctx, agentID, target, "")
		if err != nil {
			return err
		}
	}
	return s.friendshipStore.Block(ctx, agentID, target)
}

// SetNickname 设置备注
func (s *FriendshipService) SetNickname(ctx context.Context, agentID string, target string, nickname string) error {
	isFriend, err := s.friendshipStore.IsFriend(ctx, agentID, target)
	if err != nil {
		return err
	}
	if !isFriend {
		return ErrNotFriends
	}
	return s.friendshipStore.SetNickname(ctx, agentID, target, nickname)
}

// SetGroup 设置分组
func (s *FriendshipService) SetGroup(ctx context.Context, agentID string, target string, group string) error {
	isFriend, err := s.friendshipStore.IsFriend(ctx, agentID, target)
	if err != nil {
		return err
	}
	if !isFriend {
		return ErrNotFriends
	}

	// 确保分组存在
	if group != "" {
		if err := s.friendshipStore.UpsertFriendGroup(ctx, agentID, group, 0); err != nil {
			return err
		}
	}

	return s.friendshipStore.SetGroup(ctx, agentID, target, group)
}

// SetTrust 设置信任等级
func (s *FriendshipService) SetTrust(ctx context.Context, agentID string, target string, level string) error {
	if level != model.TrustLevelNormal && level != model.TrustLevelTrusted && level != model.TrustLevelBlocked {
		return errors.New("invalid trust level")
	}
	isFriend, err := s.friendshipStore.IsFriend(ctx, agentID, target)
	if err != nil {
		return err
	}
	if !isFriend {
		return ErrNotFriends
	}
	return s.friendshipStore.SetTrust(ctx, agentID, target, level)
}

// ListFriends 列出好友
func (s *FriendshipService) ListFriends(ctx context.Context, agentID string) ([]*model.Friendship, error) {
	return s.friendshipStore.ListFriends(ctx, agentID)
}

// ListReceivedRequests 列出收到的好友请求
func (s *FriendshipService) ListReceivedRequests(ctx context.Context, agentID string) ([]*model.Friendship, error) {
	return s.friendshipStore.ListReceivedRequests(ctx, agentID)
}

// IsFriend 检查是否是好友
func (s *FriendshipService) IsFriend(ctx context.Context, agentA string, agentB string) (bool, error) {
	return s.friendshipStore.IsFriend(ctx, agentA, agentB)
}

// GetFriendship 获取好友关系
func (s *FriendshipService) GetFriendship(ctx context.Context, agentA string, agentB string) (*model.Friendship, error) {
	return s.friendshipStore.GetByAgents(ctx, agentA, agentB)
}

// IsBlockedBy 检查 target 是否被 viewer 拉黑
func (s *FriendshipService) IsBlockedBy(ctx context.Context, viewer string, target string) (bool, error) {
	return s.friendshipStore.IsBlockedBy(ctx, viewer, target)
}

// GetTrustLevel 获取 viewer 对 target 的信任等级
func (s *FriendshipService) GetTrustLevel(ctx context.Context, viewer string, target string) (string, error) {
	return s.friendshipStore.GetTrustLevel(ctx, viewer, target)
}

// IncrementInteraction 增加交互计数
func (s *FriendshipService) IncrementInteraction(ctx context.Context, agentA string, agentB string) {
	s.friendshipStore.IncrementInteraction(ctx, agentA, agentB)
}

// ListFriendGroups 列出好友分组
func (s *FriendshipService) ListFriendGroups(ctx context.Context, agentID string) ([]*model.FriendGroup, error) {
	return s.friendshipStore.ListFriendGroups(ctx, agentID)
}

// SearchWithFriendStatus 搜索 agent 并附带好友状态
func (s *FriendshipService) SearchWithFriendStatus(ctx context.Context, viewerID string, keyword string, skill string, tags []string, onlineOnly bool, limit int, offset int) ([]*model.Agent, []bool, int, error) {
	agents, total, err := s.agentStore.Search(ctx, keyword, skill, tags, onlineOnly, limit, offset)
	if err != nil {
		return nil, nil, 0, err
	}

	isFriendList := make([]bool, len(agents))
	for i, agent := range agents {
		if agent.AgentID == viewerID {
			isFriendList[i] = false
			continue
		}
		isFriend, _ := s.friendshipStore.IsFriend(ctx, viewerID, agent.AgentID)
		isFriendList[i] = isFriend
	}

	return agents, isFriendList, total, nil
}
