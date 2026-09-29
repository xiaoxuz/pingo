package model

import (
	"encoding/json"
	"time"
)

type Capability struct {
	Skill string   `json:"skill"`
	Tags  []string `json:"tags"`
}

type Availability struct {
	Mode                    string `json:"mode"`
	OnlineHours             string `json:"online_hours,omitempty"`
	Timezone                string `json:"timezone,omitempty"`
	MaxConcurrentConversations int `json:"max_concurrent_conversations,omitempty"`
}

type Agent struct {
	AgentID       string          `db:"agent_id" json:"agent_id"`
	Name          string          `db:"name" json:"name"`
	Token         string          `db:"token" json:"-"`
	OwnerName     string          `db:"owner_name" json:"owner_name,omitempty"`
	OwnerEmail    string          `db:"owner_email" json:"owner_email,omitempty"`
	AvatarURL     string          `db:"avatar_url" json:"avatar_url,omitempty"`
	StatusText    string          `db:"status_text" json:"status_text,omitempty"`
	Capabilities  json.RawMessage `db:"capabilities" json:"capabilities"`
	Availability  json.RawMessage `db:"availability" json:"availability"`
	Online        bool            `db:"online" json:"online"`
	LastHeartbeat *time.Time      `db:"last_heartbeat" json:"last_heartbeat,omitempty"`
	CreatedAt     time.Time       `db:"created_at" json:"created_at"`
	UpdatedAt     time.Time       `db:"updated_at" json:"updated_at"`
}

func (a *Agent) GetCapabilities() ([]Capability, error) {
	var caps []Capability
	if len(a.Capabilities) == 0 {
		return caps, nil
	}
	err := json.Unmarshal(a.Capabilities, &caps)
	return caps, err
}

func (a *Agent) SetCapabilities(caps []Capability) error {
	data, err := json.Marshal(caps)
	if err != nil {
		return err
	}
	a.Capabilities = data
	return nil
}

type AgentPublic struct {
	AgentID      string       `json:"agent_id"`
	Name         string       `json:"name"`
	AvatarURL    string       `json:"avatar_url,omitempty"`
	StatusText   string       `json:"status_text,omitempty"`
	Online       bool         `json:"online"`
	Capabilities []Capability `json:"capabilities,omitempty"`
}

func (a *Agent) ToPublic() *AgentPublic {
	caps, _ := a.GetCapabilities()
	return &AgentPublic{
		AgentID:      a.AgentID,
		Name:         a.Name,
		AvatarURL:    a.AvatarURL,
		StatusText:   a.StatusText,
		Online:       a.Online,
		Capabilities: caps,
	}
}

type CapabilityIndex struct {
	AgentID string `db:"agent_id"`
	Skill   string `db:"skill"`
	Tag     string `db:"tag"`
}
