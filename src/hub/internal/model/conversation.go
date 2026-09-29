package model

import "time"

const (
	ConvTypeDirect = "direct"
	ConvTypeGroup  = "group"
)

const (
	ConvStatusActive = "active"
	ConvStatusClosed = "closed"
)

const (
	MemberRoleCreator = "creator"
	MemberRoleMember  = "member"
)

type Conversation struct {
	ConversationID     string     `db:"conversation_id" json:"conversation_id"`
	Type               string     `db:"type" json:"type"`
	Name               string     `db:"name" json:"name,omitempty"`
	CreatedBy          string     `db:"created_by" json:"created_by"`
	Status             string     `db:"status" json:"status"`
	CreatedAt          time.Time  `db:"created_at" json:"created_at"`
	ClosedAt           *time.Time `db:"closed_at" json:"closed_at,omitempty"`
	LastMessageAt      *time.Time `db:"last_message_at" json:"last_message_at,omitempty"`
	LastMessagePreview string     `db:"last_message_preview" json:"last_message_preview,omitempty"`
}

type ConversationMember struct {
	ConversationID     string     `db:"conversation_id" json:"conversation_id"`
	AgentID            string     `db:"agent_id" json:"agent_id"`
	Role               string     `db:"role" json:"role"`
	JoinedAt           time.Time  `db:"joined_at" json:"joined_at"`
	LeftAt             *time.Time `db:"left_at" json:"left_at,omitempty"`
	LastReadMessageID  string     `db:"last_read_message_id" json:"last_read_message_id,omitempty"`
	UnreadCount        int        `db:"unread_count" json:"unread_count"`
}

func (m *ConversationMember) IsActive() bool {
	return m.LeftAt == nil
}
