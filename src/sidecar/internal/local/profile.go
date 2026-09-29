package local

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

func (s *APIServer) handleGetMe(c *gin.Context) {
	agentID := getAgentID(c)
	token := s.getAgentToken(agentID)
	if token == "" {
		errResponse(c, http.StatusNotFound, "agent not managed")
		return
	}
	request, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, s.config.Hub.HTTPEndpoint()+"/api/agents/me", nil)
	if err != nil {
		errResponse(c, http.StatusInternalServerError, err.Error())
		return
	}
	request.Header.Set("Authorization", "Bearer "+token)
	client := &http.Client{Timeout: 5 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		errResponse(c, http.StatusBadGateway, "Hub profile unavailable: "+err.Error())
		return
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		errResponse(c, http.StatusBadGateway, "Hub profile unavailable")
		return
	}
	var payload struct {
		OK   bool `json:"ok"`
		Data struct {
			AgentID       string          `json:"agent_id"`
			Name          string          `json:"name"`
			OwnerName     string          `json:"owner_name"`
			OwnerEmail    string          `json:"owner_email"`
			AvatarURL     string          `json:"avatar_url"`
			StatusText    string          `json:"status_text"`
			Capabilities  json.RawMessage `json:"capabilities"`
			Availability  json.RawMessage `json:"availability"`
			Online        bool            `json:"online"`
			LastHeartbeat *time.Time      `json:"last_heartbeat"`
			CreatedAt     time.Time       `json:"created_at"`
			UpdatedAt     time.Time       `json:"updated_at"`
		} `json:"data"`
	}
	if json.NewDecoder(response.Body).Decode(&payload) != nil || !payload.OK || payload.Data.AgentID != agentID {
		errResponse(c, http.StatusBadGateway, "invalid Hub profile")
		return
	}
	okResponse(c, payload.Data)
}
