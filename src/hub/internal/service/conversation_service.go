package service

import (
	"context"
	"errors"

	"github.com/pingo/hub/internal/model"
	"github.com/pingo/hub/internal/store"
)

var (
	ErrConvNotFound       = errors.New("conversation not found")
	ErrConvClosed         = errors.New("conversation is closed")
	ErrNotMember          = errors.New("not a member")
	ErrNotCreator         = errors.New("not the creator")
	ErrCreatorCannotLeave = errors.New("creator cannot leave, close the conversation instead")
	ErrAlreadyMember      = errors.New("already a member")
	ErrAllMembersLeft     = errors.New("all members left")
)

type ConversationService struct {
	convStore       *store.ConversationStore
	friendshipStore *store.FriendshipStore
	msgStore        *store.MessageStore
}

func NewConversationService(
	convStore *store.ConversationStore,
	friendshipStore *store.FriendshipStore,
	msgStore *store.MessageStore,
) *ConversationService {
	return &ConversationService{
		convStore:       convStore,
		friendshipStore: friendshipStore,
		msgStore:        msgStore,
	}
}

// StartDirect 发起单聊
func (s *ConversationService) StartDirect(ctx context.Context, from string, target string, firstMessage string) (*model.Conversation, *model.Message, error) {
	// 校验：必须是好友
	isFriend, err := s.friendshipStore.IsFriend(ctx, from, target)
	if err != nil {
		return nil, nil, err
	}
	if !isFriend {
		return nil, nil, ErrNotFriends
	}

	// 检查对方是否把自己拉黑了
	blocked, err := s.friendshipStore.IsBlockedBy(ctx, target, from)
	if err != nil {
		return nil, nil, err
	}
	if blocked {
		return nil, nil, ErrBlocked
	}

	// 检查是否已有活跃的单聊
	existing, err := s.convStore.FindDirectConv(ctx, from, target)
	if err != nil {
		return nil, nil, err
	}
	if existing != nil {
		// 已有活跃单聊，直接发消息而不是新建
		msg, err := s.createTextMessage(ctx, existing.ConversationID, from, firstMessage)
		if err != nil {
			return nil, nil, err
		}
		return existing, msg, nil
	}

	// 创建新会话
	convID := s.convStore.GenerateID()
	conv := &model.Conversation{
		ConversationID: convID,
		Type:           model.ConvTypeDirect,
		CreatedBy:      from,
		Status:         model.ConvStatusActive,
	}

	members := []*model.ConversationMember{
		{ConversationID: convID, AgentID: from, Role: model.MemberRoleCreator},
		{ConversationID: convID, AgentID: target, Role: model.MemberRoleMember},
	}

	if err := s.convStore.Create(ctx, conv, members); err != nil {
		return nil, nil, err
	}

	// 发第一条消息
	msg, err := s.createTextMessage(ctx, convID, from, firstMessage)
	if err != nil {
		return nil, nil, err
	}

	return conv, msg, nil
}

// CreateGroup 创建群聊
func (s *ConversationService) CreateGroup(ctx context.Context, creator string, name string, inviteList []string, firstMessage string) (*model.Conversation, *model.Message, error) {
	// 校验：所有被邀请的人都是 creator 的好友
	for _, invitee := range inviteList {
		isFriend, err := s.friendshipStore.IsFriend(ctx, creator, invitee)
		if err != nil {
			return nil, nil, err
		}
		if !isFriend {
			return nil, nil, ErrNotFriends
		}
	}

	convID := s.convStore.GenerateID()
	conv := &model.Conversation{
		ConversationID: convID,
		Type:           model.ConvTypeGroup,
		Name:           name,
		CreatedBy:      creator,
		Status:         model.ConvStatusActive,
	}

	members := []*model.ConversationMember{
		{ConversationID: convID, AgentID: creator, Role: model.MemberRoleCreator},
	}
	for _, invitee := range inviteList {
		members = append(members, &model.ConversationMember{
			ConversationID: convID,
			AgentID:        invitee,
			Role:           model.MemberRoleMember,
		})
	}

	if err := s.convStore.Create(ctx, conv, members); err != nil {
		return nil, nil, err
	}

	// 系统消息
	sysContent := name + " 群聊已创建"
	_, _ = s.msgStore.CreateSystemMessage(ctx, convID, model.SystemEventMemberJoined, sysContent)

	// 第一条消息
	var firstMsg *model.Message
	if firstMessage != "" {
		firstMsg, _ = s.createTextMessage(ctx, convID, creator, firstMessage)
	}

	return conv, firstMsg, nil
}

// Invite 邀请入群
func (s *ConversationService) Invite(ctx context.Context, from string, conversationID string, target string) error {
	// 校验会话存在且活跃
	conv, err := s.convStore.GetByID(ctx, conversationID)
	if err != nil {
		return err
	}
	if conv == nil {
		return ErrConvNotFound
	}
	if conv.Status != model.ConvStatusActive {
		return ErrConvClosed
	}

	// 校验邀请者是成员
	isMember, err := s.convStore.IsMember(ctx, conversationID, from)
	if err != nil {
		return err
	}
	if !isMember {
		return ErrNotMember
	}

	// 校验目标已经是成员
	isMember, err = s.convStore.IsMember(ctx, conversationID, target)
	if err != nil {
		return err
	}
	if isMember {
		return ErrAlreadyMember
	}

	// 校验 target 是 from 的好友
	isFriend, err := s.friendshipStore.IsFriend(ctx, from, target)
	if err != nil {
		return err
	}
	if !isFriend {
		return ErrNotFriends
	}

	// 添加成员
	if err := s.convStore.AddMember(ctx, conversationID, target, model.MemberRoleMember); err != nil {
		return err
	}

	// 系统消息
	fromAgent, _ := s.getAgentName(ctx, from)
	targetAgent, _ := s.getAgentName(ctx, target)
	sysContent := fromAgent + " 邀请了 " + targetAgent + " 加入群聊"
	s.msgStore.CreateSystemMessage(ctx, conversationID, model.SystemEventMemberJoined, sysContent)

	return nil
}

// Leave 退出群聊
func (s *ConversationService) Leave(ctx context.Context, agentID string, conversationID string) error {
	conv, err := s.convStore.GetByID(ctx, conversationID)
	if err != nil {
		return err
	}
	if conv == nil {
		return ErrConvNotFound
	}
	if conv.Status != model.ConvStatusActive {
		return ErrConvClosed
	}

	// 校验是成员
	member, err := s.convStore.GetMember(ctx, conversationID, agentID)
	if err != nil {
		return err
	}
	if member == nil {
		return ErrNotMember
	}
	if !member.IsActive() {
		return ErrNotMember
	}

	// 创建者不能退出
	if member.Role == model.MemberRoleCreator {
		return ErrCreatorCannotLeave
	}

	// 执行退出
	if err := s.convStore.LeaveMember(ctx, conversationID, agentID); err != nil {
		return err
	}

	// 系统消息
	agentName, _ := s.getAgentName(ctx, agentID)
	sysContent := agentName + " 退出了群聊"
	s.msgStore.CreateSystemMessage(ctx, conversationID, model.SystemEventMemberLeft, sysContent)

	// 检查群里是否还有活跃成员，没有的话自动关闭
	count, err := s.convStore.ActiveMemberCount(ctx, conversationID)
	if err != nil {
		return err
	}
	if count == 0 {
		s.convStore.Close(ctx, conversationID, "")
	}

	return nil
}

// Close 关闭会话
func (s *ConversationService) Close(ctx context.Context, agentID string, conversationID string) error {
	conv, err := s.convStore.GetByID(ctx, conversationID)
	if err != nil {
		return err
	}
	if conv == nil {
		return ErrConvNotFound
	}
	if conv.Status != model.ConvStatusActive {
		return ErrConvClosed
	}

	// 单聊：任一方都可以关闭
	if conv.Type == model.ConvTypeDirect {
		isMember, err := s.convStore.IsMember(ctx, conversationID, agentID)
		if err != nil {
			return err
		}
		if !isMember {
			return ErrNotMember
		}
	} else {
		// 群聊：只有创建者可以关闭
		role, err := s.convStore.GetMemberRole(ctx, conversationID, agentID)
		if err != nil {
			return err
		}
		if role != model.MemberRoleCreator {
			return ErrNotCreator
		}
	}

	// 执行关闭
	if err := s.convStore.Close(ctx, conversationID, agentID); err != nil {
		return err
	}

	// 系统消息
	agentName, _ := s.getAgentName(ctx, agentID)
	sysContent := agentName + " 关闭了会话"
	s.msgStore.CreateSystemMessage(ctx, conversationID, model.SystemEventConvClosed, sysContent)

	return nil
}

// GetByID 获取会话
func (s *ConversationService) GetByID(ctx context.Context, conversationID string) (*model.Conversation, error) {
	return s.convStore.GetByID(ctx, conversationID)
}

// ListByAgent 列出 agent 的会话
func (s *ConversationService) ListByAgent(ctx context.Context, agentID string, status string) ([]*model.Conversation, error) {
	return s.convStore.ListByAgent(ctx, agentID, status)
}

// ListMembers 列出会话成员
func (s *ConversationService) ListMembers(ctx context.Context, conversationID string) ([]*model.ConversationMember, error) {
	return s.convStore.ListMembers(ctx, conversationID)
}

// ListActiveMembers 列出活跃成员
func (s *ConversationService) ListActiveMembers(ctx context.Context, conversationID string) ([]*model.ConversationMember, error) {
	return s.convStore.ListActiveMembers(ctx, conversationID)
}

// IsMember 检查是否是成员
func (s *ConversationService) IsMember(ctx context.Context, conversationID string, agentID string) (bool, error) {
	return s.convStore.IsMember(ctx, conversationID, agentID)
}

// GetMemberRole 获取成员角色
func (s *ConversationService) GetMemberRole(ctx context.Context, conversationID string, agentID string) (string, error) {
	return s.convStore.GetMemberRole(ctx, conversationID, agentID)
}

// MarkRead 标记已读
func (s *ConversationService) MarkRead(ctx context.Context, conversationID string, agentID string, messageID string) error {
	return s.convStore.MarkRead(ctx, conversationID, agentID, messageID)
}

// GetUnreadCount 获取未读数
func (s *ConversationService) GetUnreadCount(ctx context.Context, conversationID string, agentID string) (int, error) {
	return s.convStore.GetUnreadCount(ctx, conversationID, agentID)
}

// IncrementUnread 增加未读数
func (s *ConversationService) IncrementUnread(ctx context.Context, conversationID string, agentID string) error {
	return s.convStore.IncrementUnread(ctx, conversationID, agentID)
}

func (s *ConversationService) createTextMessage(ctx context.Context, convID string, from string, content string) (*model.Message, error) {
	msg := &model.Message{
		MessageID:      s.msgStore.GenerateID(),
		ConversationID: convID,
		FromAgent:      from,
		MessageType:    model.MsgTypeText,
		ContentText:    content,
	}
	if err := s.msgStore.Create(ctx, msg); err != nil {
		return nil, err
	}

	// 更新会话最后消息
	preview := content
	if len(preview) > 200 {
		preview = preview[:200]
	}
	s.convStore.UpdateLastMessage(ctx, convID, preview)

	members, err := s.convStore.ListActiveMembers(ctx, convID)
	if err != nil {
		return nil, err
	}
	for _, member := range members {
		if member.AgentID != from {
			if err := s.convStore.IncrementUnread(ctx, convID, member.AgentID); err != nil {
				return nil, err
			}
		}
	}

	return msg, nil
}

func (s *ConversationService) getAgentName(ctx context.Context, agentID string) (string, error) {
	agentStore := store.NewAgentStore(nil) // 这里需要 agent store，实际通过注入
	_ = agentStore
	return agentID, nil
}
