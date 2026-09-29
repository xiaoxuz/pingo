package service

import (
	"context"
	"errors"

	"github.com/pingo/hub/internal/model"
	"github.com/pingo/hub/internal/store"
)

var (
	ErrInvalidMessageType = errors.New("invalid message type")
	ErrEmptyContent       = errors.New("empty content")
)

type MessageService struct {
	msgStore        *store.MessageStore
	convStore       *store.ConversationStore
	friendshipStore *store.FriendshipStore
	offlineStore    *store.OfflineStore
}

func NewMessageService(
	msgStore *store.MessageStore,
	convStore *store.ConversationStore,
	friendshipStore *store.FriendshipStore,
	offlineStore *store.OfflineStore,
) *MessageService {
	return &MessageService{
		msgStore:        msgStore,
		convStore:       convStore,
		friendshipStore: friendshipStore,
		offlineStore:    offlineStore,
	}
}

// Send 发送消息
func (s *MessageService) Send(ctx context.Context, from string, conversationID string, msgType string, contentText string, contentFile *model.ContentFile, mentions []string, replyTo string) (*model.Message, error) {
	// 校验会话存在
	conv, err := s.convStore.GetByID(ctx, conversationID)
	if err != nil {
		return nil, err
	}
	if conv == nil {
		return nil, ErrConvNotFound
	}
	if conv.Status != model.ConvStatusActive {
		return nil, ErrConvClosed
	}

	// 校验发送者是成员
	isMember, err := s.convStore.IsMember(ctx, conversationID, from)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, ErrNotMember
	}

	// 单聊时检查对方是否拉黑了自己
	if conv.Type == model.ConvTypeDirect {
		members, err := s.convStore.ListActiveMembers(ctx, conversationID)
		if err != nil {
			return nil, err
		}
		for _, m := range members {
			if m.AgentID != from {
				blocked, _ := s.friendshipStore.IsBlockedBy(ctx, m.AgentID, from)
				if blocked {
					return nil, ErrBlocked
				}
			}
		}
	}

	// 校验消息类型
	if msgType == "" {
		msgType = model.MsgTypeText
	}
	if msgType != model.MsgTypeText && msgType != model.MsgTypeFile {
		return nil, ErrInvalidMessageType
	}
	if msgType == model.MsgTypeText && contentText == "" {
		return nil, ErrEmptyContent
	}

	// 构造消息
	msg := &model.Message{
		MessageID:      s.msgStore.GenerateID(),
		ConversationID: conversationID,
		FromAgent:      from,
		MessageType:    msgType,
		ContentText:    contentText,
		Mentions:       mentions,
		ReplyTo:        replyTo,
	}

	if contentFile != nil {
		fileJSON, _ := marshalContentFile(contentFile)
		msg.ContentFile = fileJSON
	}

	// 存消息
	if err := s.msgStore.Create(ctx, msg); err != nil {
		return nil, err
	}

	// 更新会话最后消息
	preview := contentText
	if msgType == model.MsgTypeFile {
		if contentFile != nil {
			preview = "[文件] " + contentFile.Filename
		} else {
			preview = "[文件]"
		}
	}
	if len(preview) > 200 {
		preview = preview[:200]
	}
	s.convStore.UpdateLastMessage(ctx, conversationID, preview)

	// 更新所有其他成员的未读数 + 离线队列
	members, err := s.convStore.ListActiveMembers(ctx, conversationID)
	if err != nil {
		return msg, nil
	}

	for _, m := range members {
		if m.AgentID == from {
			continue
		}
		s.convStore.IncrementUnread(ctx, conversationID, m.AgentID)
	}

	// 增加好友交互计数
	if conv.Type == model.ConvTypeDirect {
		for _, m := range members {
			if m.AgentID != from {
				s.friendshipStore.IncrementInteraction(ctx, from, m.AgentID)
			}
		}
	}

	return msg, nil
}

// ListMessages 列出消息
func (s *MessageService) ListMessages(ctx context.Context, conversationID string, agentID string, afterMessageID string, limit int) ([]*model.Message, error) {
	// 校验权限
	isMember, err := s.convStore.IsMember(ctx, conversationID, agentID)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, ErrNotMember
	}

	return s.msgStore.ListByConversation(ctx, conversationID, afterMessageID, limit)
}

// GetByID 获取单条消息
func (s *MessageService) GetByID(ctx context.Context, messageID string) (*model.Message, error) {
	return s.msgStore.GetByID(ctx, messageID)
}

// EnqueueOffline 把消息加入离线队列
func (s *MessageService) EnqueueOffline(ctx context.Context, targetAgent string, messageID string, conversationID string) error {
	return s.offlineStore.Enqueue(ctx, targetAgent, messageID, conversationID)
}

// GetOfflinePending 获取待投递的离线消息
func (s *MessageService) GetOfflinePending(ctx context.Context, targetAgent string) ([]*model.OfflineQueueItem, error) {
	return s.offlineStore.GetPending(ctx, targetAgent)
}

// MarkOfflineDelivered 标记离线消息已投递
func (s *MessageService) MarkOfflineDelivered(ctx context.Context, id int) error {
	return s.offlineStore.MarkDelivered(ctx, id)
}

// MarkAllOfflineDelivered 标记所有离线消息已投递
func (s *MessageService) MarkAllOfflineDelivered(ctx context.Context, targetAgent string) error {
	return s.offlineStore.MarkAllDelivered(ctx, targetAgent)
}

func marshalContentFile(cf *model.ContentFile) ([]byte, error) {
	// 简单的 JSON 序列化
	if cf == nil {
		return nil, nil
	}
	data := []byte(`{"filename":"` + cf.Filename + `","size":` + itoa(int(cf.Size)) + `,"storage_key":"` + cf.StorageKey + `"}`)
	return data, nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var result []byte
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	for n > 0 {
		result = append([]byte{byte('0' + n%10)}, result...)
		n /= 10
	}
	if neg {
		result = append([]byte{'-'}, result...)
	}
	return string(result)
}
