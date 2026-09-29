package hub

import (
	"encoding/json"
	"log"
	"time"

	appr "github.com/pingo/sidecar/internal/approval"
	notif "github.com/pingo/sidecar/internal/notify"
	"github.com/pingo/sidecar/internal/operationlog"
	"github.com/pingo/sidecar/internal/session"
	st "github.com/pingo/sidecar/internal/store"
)

type MessageReceiver struct {
	store       *st.SQLiteStore
	notifier    *notif.Manager
	approvalMgr *appr.Manager
	sessions    *session.Registry
	agentNames  map[string]string
}

func NewMessageReceiver(
	s *st.SQLiteStore,
	n *notif.Manager,
	a *appr.Manager,
	sessions *session.Registry,
	agentNames map[string]string,
) *MessageReceiver {
	return &MessageReceiver{
		store:       s,
		notifier:    n,
		approvalMgr: a,
		sessions:    sessions,
		agentNames:  agentNames,
	}
}

func (r *MessageReceiver) HandleMessage(agentID string, env MessageEnvelope) {
	agentName := r.agentNames[agentID]
	if agentName == "" {
		agentName = agentID
	}

	switch env.Type {
	case "msg.new.notify":
		r.handleMsgNew(agentID, agentName, env)
	case "friend.request.notify":
		r.handleFriendRequest(agentID, agentName, env)
	case "friend.accepted.notify":
		r.handleFriendAccepted(agentID, agentName, env)
	case "friend.rejected.notify":
		r.handleFriendRejected(agentID, agentName, env)
	case "friend.removed.notify":
		r.handleFriendRemoved(agentID, env)
	case "conv.invited.notify":
		r.handleConvInvited(agentID, agentName, env)
	case "conv.member_joined.notify":
		r.handleConvMemberJoined(agentID, env)
	case "conv.member_left.notify":
		r.handleConvMemberLeft(agentID, env)
	case "conv.closed.notify":
		r.handleConvClosed(agentID, env)
	case "sync.friends.resp":
		r.handleSyncFriends(agentID, env)
	case "sync.friend_requests.resp":
		r.handleSyncFriendRequests(agentID, env)
	case "sync.conversations.resp":
		r.handleSyncConversations(agentID, env)
	case "sync.messages.resp":
		r.handleSyncMessages(agentID, env)
	case "auth.login.resp":
		log.Printf("INFO: agent %s login confirmed", agentID)
	case "heartbeat.pong":
	default:
		log.Printf("INFO: received message type: %s for agent %s", env.Type, agentID)
	}
}

func (r *MessageReceiver) handleMsgNew(agentID string, agentName string, env MessageEnvelope) {
	dataBytes, _ := json.Marshal(env.Data)
	var data struct {
		ConversationID string `json:"conversation_id"`
		Message        struct {
			MessageID      string   `json:"message_id"`
			ConversationID string   `json:"conversation_id"`
			FromAgent      string   `json:"from_agent"`
			MessageType    string   `json:"message_type"`
			ContentText    string   `json:"content_text"`
			Mentions       []string `json:"mentions"`
			ReplyTo        string   `json:"reply_to"`
			SystemEvent    string   `json:"system_event"`
			CreatedAt      string   `json:"created_at"`
		} `json:"message"`
		UnreadCount int `json:"unread_count"`
	}
	json.Unmarshal(dataBytes, &data)

	msg := st.MessageRecord{
		MessageID:      data.Message.MessageID,
		ConversationID: data.ConversationID,
		FromAgent:      data.Message.FromAgent,
		MessageType:    data.Message.MessageType,
		ContentText:    data.Message.ContentText,
		Mentions:       joinStrings(data.Message.Mentions, ","),
		ReplyTo:        data.Message.ReplyTo,
		SystemEvent:    data.Message.SystemEvent,
		CreatedAt:      data.Message.CreatedAt,
	}
	r.store.SaveMessage(agentID, msg)

	r.store.IncrementUnread(agentID, data.ConversationID)

	if data.Message.MessageType == "system" {
		return
	}

	fromName := data.Message.FromAgent
	friend, _ := r.store.GetFriend(agentID, data.Message.FromAgent)
	if friend != nil {
		if friend.Nickname != "" {
			fromName = friend.Nickname
		} else if friend.Name != "" {
			fromName = friend.Name
		}
	}

	preview := messagePreview(data.Message.ContentText)
	conversationType := ""
	conversationName := ""
	if conversation, err := r.store.GetConversation(agentID, data.ConversationID); err == nil && conversation != nil {
		conversationType = conversation.Type
		conversationName = conversation.Name
	}

	r.notifier.Notify(notif.Notification{
		AgentID:   agentID,
		AgentName: agentName,
		Title:     fromName + " 发来消息",
		Message:   preview,
		Type:      "message",
	})

	if r.sessions == nil {
		return
	}
	unreadCount, err := r.store.GetTotalUnread(agentID)
	if err != nil {
		log.Printf("WARN: failed to read unread count for agent %s: %v", agentID, err)
		return
	}
	result := r.sessions.Publish(agentID, session.Event{
		Type:             "message",
		UnreadCount:      unreadCount,
		Title:            fromName + " 发来消息",
		Message:          preview,
		ConversationType: conversationType,
		ConversationName: conversationName,
		SenderName:       fromName,
	})
	operationlog.New(log.Writer()).Event("session", "deliver_event", "agent_id", agentID,
		"event_type", "message", "session_id", result.SessionID, "result", result.Status, "unread_count", unreadCount)
}

func messagePreview(content string) string {
	runes := []rune(content)
	if len(runes) <= 1000 {
		return content
	}
	return string(runes[:1000]) + "..."
}

func (r *MessageReceiver) handleFriendRequest(agentID string, agentName string, env MessageEnvelope) {
	dataBytes, _ := json.Marshal(env.Data)
	var data struct {
		FromAgent string `json:"from_agent"`
		Name      string `json:"name"`
		Message   string `json:"message"`
		CreatedAt string `json:"created_at"`
	}
	json.Unmarshal(dataBytes, &data)

	r.store.UpsertFriendRequest(agentID, st.FriendRequestRecord{
		FromAgent: data.FromAgent,
		Name:      data.Name,
		Message:   data.Message,
		CreatedAt: data.CreatedAt,
	})

	r.notifier.Notify(notif.Notification{
		AgentID:   agentID,
		AgentName: agentName,
		Title:     "收到好友请求",
		Message:   data.Name + ": " + data.Message,
		Type:      "friend_request",
	})
	if r.sessions != nil {
		result := r.sessions.Publish(agentID, session.Event{Type: "friend_request", Title: "收到好友请求", Message: data.Name + ": " + data.Message})
		operationlog.New(log.Writer()).Event("session", "deliver_event", "agent_id", agentID,
			"event_type", "friend_request", "session_id", result.SessionID, "result", result.Status)
	}
}

func (r *MessageReceiver) handleFriendAccepted(agentID string, agentName string, env MessageEnvelope) {
	dataBytes, _ := json.Marshal(env.Data)
	var data struct {
		FromAgent string `json:"from_agent"`
		Name      string `json:"name"`
	}
	json.Unmarshal(dataBytes, &data)

	r.notifier.Notify(notif.Notification{
		AgentID:   agentID,
		AgentName: agentName,
		Title:     "好友请求已通过",
		Message:   data.Name + " 通过了你的好友请求",
		Type:      "friend_request",
	})
}

func (r *MessageReceiver) handleFriendRejected(agentID string, agentName string, env MessageEnvelope) {
	dataBytes, _ := json.Marshal(env.Data)
	var data struct {
		FromAgent string `json:"from_agent"`
	}
	json.Unmarshal(dataBytes, &data)

	r.notifier.Notify(notif.Notification{
		AgentID:   agentID,
		AgentName: agentName,
		Title:     "好友请求被拒绝",
		Message:   data.FromAgent + " 拒绝了你的好友请求",
		Type:      "friend_request",
	})
}

func (r *MessageReceiver) handleFriendRemoved(agentID string, env MessageEnvelope) {
	dataBytes, _ := json.Marshal(env.Data)
	var data struct {
		FromAgent string `json:"from_agent"`
	}
	json.Unmarshal(dataBytes, &data)
	log.Printf("INFO: agent %s was removed as friend by %s", agentID, data.FromAgent)
}

func (r *MessageReceiver) handleConvInvited(agentID string, agentName string, env MessageEnvelope) {
	dataBytes, _ := json.Marshal(env.Data)
	var data struct {
		ConversationID string   `json:"conversation_id"`
		Name           string   `json:"name"`
		InvitedBy      string   `json:"invited_by"`
		InvitedByName  string   `json:"invited_by_name"`
		Members        []string `json:"members"`
		CreatedAt      string   `json:"created_at"`
	}
	json.Unmarshal(dataBytes, &data)

	r.store.UpsertConversation(agentID, st.ConversationRecord{
		ConversationID: data.ConversationID,
		Type:           "group",
		Name:           data.Name,
		Status:         "active",
		CreatedBy:      data.InvitedBy,
		CreatedAt:      data.CreatedAt,
	})

	for _, m := range data.Members {
		r.store.UpsertMember(agentID, data.ConversationID, st.MemberRecord{
			MemberAgentID: m,
			Role:          "member",
			JoinedAt:      data.CreatedAt,
		})
	}

	r.notifier.Notify(notif.Notification{
		AgentID:   agentID,
		AgentName: agentName,
		Title:     "被邀请加入群聊",
		Message:   data.InvitedByName + " 邀请你加入 " + data.Name,
		Type:      "message",
	})
}

func (r *MessageReceiver) handleConvMemberJoined(agentID string, env MessageEnvelope) {
	dataBytes, _ := json.Marshal(env.Data)
	var data struct {
		ConversationID string `json:"conversation_id"`
		MemberAgentID  string `json:"member_agent_id"`
		MemberName     string `json:"member_name"`
	}
	json.Unmarshal(dataBytes, &data)

	r.store.UpsertMember(agentID, data.ConversationID, st.MemberRecord{
		MemberAgentID: data.MemberAgentID,
		Role:          "member",
		JoinedAt:      time.Now().Format(time.RFC3339),
	})

	msgID := "msg-sys-" + generateLocalID()
	r.store.SaveMessage(agentID, st.MessageRecord{
		MessageID:      msgID,
		ConversationID: data.ConversationID,
		FromAgent:      "system",
		MessageType:    "system",
		ContentText:    data.MemberName + " 加入了群聊",
		SystemEvent:    "member_joined",
		CreatedAt:      time.Now().Format(time.RFC3339),
	})
}

func (r *MessageReceiver) handleConvMemberLeft(agentID string, env MessageEnvelope) {
	dataBytes, _ := json.Marshal(env.Data)
	var data struct {
		ConversationID string `json:"conversation_id"`
		MemberAgentID  string `json:"member_agent_id"`
		MemberName     string `json:"member_name"`
	}
	json.Unmarshal(dataBytes, &data)

	r.store.UpsertMember(agentID, data.ConversationID, st.MemberRecord{
		MemberAgentID: data.MemberAgentID,
		LeftAt:        time.Now().Format(time.RFC3339),
	})

	msgID := "msg-sys-" + generateLocalID()
	r.store.SaveMessage(agentID, st.MessageRecord{
		MessageID:      msgID,
		ConversationID: data.ConversationID,
		FromAgent:      "system",
		MessageType:    "system",
		ContentText:    data.MemberName + " 退出了群聊",
		SystemEvent:    "member_left",
		CreatedAt:      time.Now().Format(time.RFC3339),
	})
}

func (r *MessageReceiver) handleConvClosed(agentID string, env MessageEnvelope) {
	dataBytes, _ := json.Marshal(env.Data)
	var data struct {
		ConversationID string `json:"conversation_id"`
		ClosedBy       string `json:"closed_by"`
		ClosedByName   string `json:"closed_by_name"`
		ClosedAt       string `json:"closed_at"`
	}
	json.Unmarshal(dataBytes, &data)

	conv, _ := r.store.GetConversation(agentID, data.ConversationID)
	if conv != nil {
		conv.Status = "closed"
		conv.ClosedAt = data.ClosedAt
		r.store.UpsertConversation(agentID, *conv)
	}

	msgID := "msg-sys-" + generateLocalID()
	r.store.SaveMessage(agentID, st.MessageRecord{
		MessageID:      msgID,
		ConversationID: data.ConversationID,
		FromAgent:      "system",
		MessageType:    "system",
		ContentText:    data.ClosedByName + " 关闭了会话",
		SystemEvent:    "conv_closed",
		CreatedAt:      data.ClosedAt,
	})
}

func (r *MessageReceiver) handleSyncFriends(agentID string, env MessageEnvelope) {
	dataBytes, _ := json.Marshal(env.Data)
	var data struct {
		Friends []struct {
			AgentID      string `json:"agent_id"`
			Name         string `json:"name"`
			AvatarURL    string `json:"avatar_url"`
			StatusText   string `json:"status_text"`
			Online       bool   `json:"online"`
			Nickname     string `json:"nickname"`
			Group        string `json:"group"`
			TrustLevel   string `json:"trust_level"`
			Capabilities []struct {
				Skill string   `json:"skill"`
				Tags  []string `json:"tags"`
			} `json:"capabilities"`
		} `json:"friends"`
	}
	json.Unmarshal(dataBytes, &data)

	for _, f := range data.Friends {
		capsJSON, _ := json.Marshal(f.Capabilities)
		r.store.UpsertFriend(agentID, st.FriendRecord{
			FriendID:     f.AgentID,
			Name:         f.Name,
			Nickname:     f.Nickname,
			Group:        f.Group,
			TrustLevel:   f.TrustLevel,
			StatusText:   f.StatusText,
			Online:       f.Online,
			Capabilities: string(capsJSON),
		})
	}

	log.Printf("INFO: synced %d friends for agent %s", len(data.Friends), agentID)
}

func (r *MessageReceiver) handleSyncFriendRequests(agentID string, env MessageEnvelope) {
	dataBytes, _ := json.Marshal(env.Data)
	var data struct {
		Requests []struct {
			FromAgent string `json:"from_agent"`
			Name      string `json:"name"`
			Message   string `json:"message"`
			CreatedAt string `json:"created_at"`
		} `json:"requests"`
	}
	json.Unmarshal(dataBytes, &data)

	for _, req := range data.Requests {
		r.store.UpsertFriendRequest(agentID, st.FriendRequestRecord{
			FromAgent: req.FromAgent,
			Name:      req.Name,
			Message:   req.Message,
			CreatedAt: req.CreatedAt,
		})
	}

	log.Printf("INFO: synced %d friend requests for agent %s", len(data.Requests), agentID)
}

func (r *MessageReceiver) handleSyncConversations(agentID string, env MessageEnvelope) {
	dataBytes, _ := json.Marshal(env.Data)
	var data struct {
		Conversations []struct {
			ConversationID     string   `json:"conversation_id"`
			Type               string   `json:"type"`
			Name               string   `json:"name"`
			Status             string   `json:"status"`
			CreatedBy          string   `json:"created_by"`
			LastMessagePreview string   `json:"last_message_preview"`
			LastMessageAt      string   `json:"last_message_at"`
			UnreadCount        int      `json:"unread_count"`
			Members            []string `json:"members"`
			CreatedAt          string   `json:"created_at"`
			ClosedAt           string   `json:"closed_at"`
		} `json:"conversations"`
	}
	json.Unmarshal(dataBytes, &data)

	for _, conv := range data.Conversations {
		r.store.UpsertConversation(agentID, st.ConversationRecord{
			ConversationID:     conv.ConversationID,
			Type:               conv.Type,
			Name:               conv.Name,
			Status:             conv.Status,
			CreatedBy:          conv.CreatedBy,
			LastMessagePreview: conv.LastMessagePreview,
			LastMessageAt:      conv.LastMessageAt,
			UnreadCount:        conv.UnreadCount,
			CreatedAt:          conv.CreatedAt,
			ClosedAt:           conv.ClosedAt,
		})

		for _, m := range conv.Members {
			r.store.UpsertMember(agentID, conv.ConversationID, st.MemberRecord{
				MemberAgentID: m,
				Role:          "member",
			})
		}
	}

	log.Printf("INFO: synced %d conversations for agent %s", len(data.Conversations), agentID)
}

func (r *MessageReceiver) handleSyncMessages(agentID string, env MessageEnvelope) {
	dataBytes, _ := json.Marshal(env.Data)
	var data struct {
		ConversationID string `json:"conversation_id"`
		Messages       []struct {
			MessageID      string   `json:"message_id"`
			ConversationID string   `json:"conversation_id"`
			FromAgent      string   `json:"from_agent"`
			MessageType    string   `json:"message_type"`
			ContentText    string   `json:"content_text"`
			Mentions       []string `json:"mentions"`
			ReplyTo        string   `json:"reply_to"`
			SystemEvent    string   `json:"system_event"`
			CreatedAt      string   `json:"created_at"`
		} `json:"messages"`
	}
	json.Unmarshal(dataBytes, &data)

	for _, msg := range data.Messages {
		r.store.SaveMessage(agentID, st.MessageRecord{
			MessageID:      msg.MessageID,
			ConversationID: data.ConversationID,
			FromAgent:      msg.FromAgent,
			MessageType:    msg.MessageType,
			ContentText:    msg.ContentText,
			Mentions:       joinStrings(msg.Mentions, ","),
			ReplyTo:        msg.ReplyTo,
			SystemEvent:    msg.SystemEvent,
			CreatedAt:      msg.CreatedAt,
		})
	}
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

func generateLocalID() string {
	return time.Now().Format("20060102150405") + "-" + randomString(6)
}
