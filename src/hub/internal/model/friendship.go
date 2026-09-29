package model

import (
	"encoding/json"
	"time"
)

const (
	FriendStatusPending  = "pending"
	FriendStatusAccepted = "accepted"
	FriendStatusBlocked  = "blocked"
)

const (
	TrustLevelNormal  = "normal"
	TrustLevelTrusted = "trusted"
	TrustLevelBlocked = "blocked"
)

type TrustRules struct {
	AutoAcceptChat  bool `json:"auto_accept_chat"`
	AutoJoinGroup   bool `json:"auto_join_group"`
}

type Friendship struct {
	ID              int             `db:"id" json:"id"`
	AgentA          string          `db:"agent_a" json:"agent_a"`
	AgentB          string          `db:"agent_b" json:"agent_b"`
	Status          string          `db:"status" json:"status"`
	InitiatedBy     string          `db:"initiated_by" json:"initiated_by"`
	RequestMessage  string          `db:"request_message" json:"request_message,omitempty"`

	ANicknameForB   string          `db:"a_nickname_for_b" json:"a_nickname_for_b,omitempty"`
	AGroupForB      string          `db:"a_group_for_b" json:"a_group_for_b,omitempty"`
	ATrustLevel     string          `db:"a_trust_level" json:"a_trust_level"`
	ATrustRules     json.RawMessage `db:"a_trust_rules" json:"a_trust_rules,omitempty"`

	BNicknameForA   string          `db:"b_nickname_for_a" json:"b_nickname_for_a,omitempty"`
	BGroupForA      string          `db:"b_group_for_a" json:"b_group_for_a,omitempty"`
	BTrustLevel     string          `db:"b_trust_level" json:"b_trust_level"`
	BTrustRules     json.RawMessage `db:"b_trust_rules" json:"b_trust_rules,omitempty"`

	InteractionCount int            `db:"interaction_count" json:"interaction_count"`
	LastInteraction  *time.Time     `db:"last_interaction" json:"last_interaction,omitempty"`

	CreatedAt        time.Time      `db:"created_at" json:"created_at"`
	UpdatedAt        time.Time      `db:"updated_at" json:"updated_at"`
}

// GetFriendSide 返回 viewer 视角的好友信息
func (f *Friendship) GetFriendSide(viewer string) (friendID string, nickname string, group string, trustLevel string, trustRules json.RawMessage) {
	if f.AgentA == viewer {
		return f.AgentB, f.ANicknameForB, f.AGroupForB, f.ATrustLevel, f.ATrustRules
	}
	return f.AgentA, f.BNicknameForA, f.BGroupForA, f.BTrustLevel, f.BTrustRules
}

// SetFriendSide 设置 viewer 对 friend 的备注/分组/信任
func (f *Friendship) SetNickname(viewer string, nickname string) {
	if f.AgentA == viewer {
		f.ANicknameForB = nickname
	} else {
		f.BNicknameForA = nickname
	}
}

func (f *Friendship) SetGroup(viewer string, group string) {
	if f.AgentA == viewer {
		f.AGroupForB = group
	} else {
		f.BGroupForA = group
	}
}

func (f *Friendship) SetTrustLevel(viewer string, level string) {
	if f.AgentA == viewer {
		f.ATrustLevel = level
	} else {
		f.BTrustLevel = level
	}
}

func (f *Friendship) GetTrustLevelOf(target string) string {
	// target 是被查看的人，返回 viewer 对 target 的信任等级
	if f.AgentA == target {
		return f.BTrustLevel
	}
	return f.ATrustLevel
}

func (f *Friendship) IsBlockedBy(viewer string) bool {
	// viewer 是否把对方拉黑了
	if f.AgentA == viewer {
		return f.ATrustLevel == TrustLevelBlocked
	}
	return f.BTrustLevel == TrustLevelBlocked
}

type FriendGroup struct {
	AgentID   string `db:"agent_id" json:"agent_id"`
	GroupName string `db:"group_name" json:"group_name"`
	SortOrder int    `db:"sort_order" json:"sort_order"`
}
