package model

import (
	"encoding/json"
	"time"
)

const (
	MsgTypeText   = "text"
	MsgTypeFile   = "file"
	MsgTypeSystem = "system"
)

const (
	SystemEventMemberJoined = "member_joined"
	SystemEventMemberLeft   = "member_left"
	SystemEventConvClosed   = "conv_closed"
)

type ContentFile struct {
	Filename   string `json:"filename"`
	Size       int64  `json:"size"`
	StorageKey string `json:"storage_key"`
}

type Message struct {
	MessageID      string          `db:"message_id" json:"message_id"`
	ConversationID string          `db:"conversation_id" json:"conversation_id"`
	FromAgent      string          `db:"from_agent" json:"from_agent"`
	MessageType    string          `db:"message_type" json:"message_type"`
	ContentText    string          `db:"content_text" json:"content_text,omitempty"`
	ContentFile    json.RawMessage `db:"content_file" json:"content_file,omitempty"`
	Mentions       []string        `db:"mentions" json:"mentions,omitempty"`
	ReplyTo        string          `db:"reply_to" json:"reply_to,omitempty"`
	CreatedAt      time.Time       `db:"created_at" json:"created_at"`
	SystemEvent    string          `db:"system_event" json:"system_event,omitempty"`
}

type OfflineQueueItem struct {
	ID             int       `db:"id"`
	TargetAgent    string    `db:"target_agent"`
	MessageID      string    `db:"message_id"`
	ConversationID string    `db:"conversation_id"`
	QueuedAt       time.Time `db:"queued_at"`
	Delivered      bool      `db:"delivered"`
	DeliveredAt    *time.Time `db:"delivered_at"`
}
