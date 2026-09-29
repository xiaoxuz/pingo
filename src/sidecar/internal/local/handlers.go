package local

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/pingo/sidecar/internal/approval"
	"github.com/pingo/sidecar/internal/config"
	"github.com/pingo/sidecar/internal/store"
)

// ============================================================
// System handlers
// ============================================================

func (s *APIServer) handleStatus(c *gin.Context) {
	agentIDs := s.pool.ListAgents()
	agents := make([]gin.H, 0, len(agentIDs))
	for _, id := range agentIDs {
		connected, _ := s.pool.AgentStatus(id)
		name := s.agentNameMap[id]
		if name == "" {
			name = id
		}
		agents = append(agents, gin.H{
			"agent_id":  id,
			"name":      name,
			"connected": connected,
		})
	}

	okResponse(c, gin.H{
		"status": "running",
		"agents": agents,
	})
}

func (s *APIServer) handleListAgents(c *gin.Context) {
	agentIDs := s.pool.ListAgents()
	agents := make([]gin.H, 0, len(agentIDs))
	for _, id := range agentIDs {
		connected, _ := s.pool.AgentStatus(id)
		name := s.agentNameMap[id]
		if name == "" {
			name = id
		}
		agents = append(agents, gin.H{
			"agent_id":  id,
			"name":      name,
			"connected": connected,
		})
	}
	okResponse(c, agents)
}

func (s *APIServer) handleRegisterAgent(c *gin.Context) {
	var req struct {
		AgentID      string                  `json:"agent_id"`
		Name         string                  `json:"name" binding:"required"`
		OwnerName    string                  `json:"owner_name"`
		OwnerEmail   string                  `json:"owner_email"`
		StatusText   string                  `json:"status_text"`
		Capabilities []config.CapabilityItem `json:"capabilities"`
		Availability config.AvailabilityConf `json:"availability"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		errResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	if req.AgentID != "" {
		errResponse(c, http.StatusBadRequest, "agent_id is assigned by Hub; do not provide it")
		return
	}

	body, _ := json.Marshal(req)
	hubURL := s.config.Hub.HTTPEndpoint() + "/api/agents/register"
	hubReq, err := http.NewRequest("POST", hubURL, bytes.NewReader(body))
	if err != nil {
		errResponse(c, http.StatusInternalServerError, "create hub request failed: "+err.Error())
		return
	}
	hubReq.Header.Set("Content-Type", "application/json")
	hubReq.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(hubReq)
	if err != nil {
		errResponse(c, http.StatusBadGateway, "hub register failed: "+err.Error())
		return
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		errResponse(c, http.StatusBadGateway, "read hub response failed: "+err.Error())
		return
	}
	if resp.StatusCode >= 300 {
		errResponse(c, resp.StatusCode, fmt.Sprintf("hub register failed: %s", string(respBody)))
		return
	}

	var payload struct {
		OK   bool `json:"ok"`
		Data struct {
			AgentID string `json:"agent_id"`
			Name    string `json:"name"`
			Token   string `json:"token"`
		} `json:"data"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(respBody, &payload); err != nil {
		errResponse(c, http.StatusBadGateway, "parse hub response failed: "+err.Error())
		return
	}
	if !payload.OK {
		if payload.Error == "" {
			payload.Error = "hub register failed"
		}
		errResponse(c, http.StatusBadGateway, payload.Error)
		return
	}
	if payload.Data.Token == "" || payload.Data.AgentID == "" {
		errResponse(c, http.StatusBadGateway, "hub register response missing agent_id or token")
		return
	}

	agentCfg := config.AgentConfig{
		ID:           payload.Data.AgentID,
		Token:        payload.Data.Token,
		Name:         payload.Data.Name,
		OwnerName:    req.OwnerName,
		OwnerEmail:   req.OwnerEmail,
		StatusText:   req.StatusText,
		Capabilities: req.Capabilities,
		Availability: req.Availability,
	}

	s.config.Agents = append(s.config.Agents, agentCfg)
	if err := config.Save(s.config.Path, s.config); err != nil {
		errResponse(c, http.StatusInternalServerError, "save sidecar config failed: "+err.Error())
		return
	}
	s.agentNameMap[agentCfg.ID] = agentCfg.Name
	s.pool.AddAgent(agentCfg)

	okResponse(c, gin.H{
		"agent_id": agentCfg.ID,
		"name":     agentCfg.Name,
		"managed":  true,
	})
}

// ============================================================
// Inbox handlers
// ============================================================

func (s *APIServer) handleInbox(c *gin.Context) {
	agentID := getAgentID(c)

	convs, err := s.store.ListConversations(agentID)
	if err != nil {
		errResponse(c, http.StatusInternalServerError, err.Error())
		return
	}

	totalUnread := 0
	unreadConvs := make([]gin.H, 0)
	for _, conv := range convs {
		if conv.UnreadCount > 0 {
			totalUnread += conv.UnreadCount
			name := conv.Name
			if name == "" && conv.Type == "direct" {
				name = s.getOtherMemberName(agentID, conv.ConversationID)
			}
			unreadConvs = append(unreadConvs, gin.H{
				"conversation_id":      conv.ConversationID,
				"type":                 conv.Type,
				"name":                 name,
				"unread_count":         conv.UnreadCount,
				"last_message_preview": conv.LastMessagePreview,
				"last_message_at":      conv.LastMessageAt,
			})
		}
	}

	okResponse(c, gin.H{
		"total_unread":         totalUnread,
		"unread_conversations": unreadConvs,
	})
}

func (s *APIServer) getOtherMemberName(agentID string, conversationID string) string {
	members, err := s.store.ListMembers(agentID, conversationID)
	if err != nil {
		return ""
	}
	for _, m := range members {
		if m.MemberAgentID != agentID {
			friend, _ := s.store.GetFriend(agentID, m.MemberAgentID)
			if friend != nil && friend.Nickname != "" {
				return friend.Nickname
			}
			if friend != nil && friend.Name != "" {
				return friend.Name
			}
			return m.MemberAgentID
		}
	}
	return ""
}

// ============================================================
// Conversation handlers
// ============================================================

func (s *APIServer) handleListConversations(c *gin.Context) {
	agentID := getAgentID(c)
	status := c.DefaultQuery("status", "active")

	convs, err := s.store.ListConversations(agentID)
	if err != nil {
		errResponse(c, http.StatusInternalServerError, err.Error())
		return
	}

	result := make([]gin.H, 0)
	for _, conv := range convs {
		if status != "all" && conv.Status != status {
			continue
		}
		name := conv.Name
		if name == "" && conv.Type == "direct" {
			name = s.getOtherMemberName(agentID, conv.ConversationID)
		}

		members, _ := s.store.ListMembers(agentID, conv.ConversationID)
		memberIDs := make([]string, 0, len(members))
		for _, m := range members {
			memberIDs = append(memberIDs, m.MemberAgentID)
		}

		result = append(result, gin.H{
			"conversation_id":      conv.ConversationID,
			"type":                 conv.Type,
			"name":                 name,
			"status":               conv.Status,
			"created_by":           conv.CreatedBy,
			"last_message_preview": conv.LastMessagePreview,
			"last_message_at":      conv.LastMessageAt,
			"unread_count":         conv.UnreadCount,
			"members":              memberIDs,
			"created_at":           conv.CreatedAt,
			"closed_at":            conv.ClosedAt,
		})
	}

	okResponse(c, result)
}

func (s *APIServer) handleGetConversation(c *gin.Context) {
	agentID := getAgentID(c)
	convID := c.Param("id")

	conv, err := s.store.GetConversation(agentID, convID)
	if err != nil {
		errResponse(c, http.StatusInternalServerError, err.Error())
		return
	}
	if conv == nil {
		errResponse(c, http.StatusNotFound, "conversation not found")
		return
	}

	members, _ := s.store.ListMembers(agentID, convID)
	memberIDs := make([]string, 0, len(members))
	for _, m := range members {
		memberIDs = append(memberIDs, m.MemberAgentID)
	}

	name := conv.Name
	if name == "" && conv.Type == "direct" {
		name = s.getOtherMemberName(agentID, convID)
	}

	okResponse(c, gin.H{
		"conversation_id": conv.ConversationID,
		"type":            conv.Type,
		"name":            name,
		"status":          conv.Status,
		"created_by":      conv.CreatedBy,
		"members":         memberIDs,
		"created_at":      conv.CreatedAt,
	})
}

func (s *APIServer) handleListMessages(c *gin.Context) {
	agentID := getAgentID(c)
	convID := c.Param("id")
	after := c.Query("after_message_id")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))

	msgs, err := s.store.ListMessages(agentID, convID, limit, after)
	if err != nil {
		errResponse(c, http.StatusInternalServerError, err.Error())
		return
	}

	result := make([]gin.H, 0, len(msgs))
	for _, msg := range msgs {
		item := gin.H{
			"message_id":      msg.MessageID,
			"conversation_id": msg.ConversationID,
			"from_agent":      msg.FromAgent,
			"message_type":    msg.MessageType,
			"content_text":    msg.ContentText,
			"mentions":        msg.Mentions,
			"reply_to":        msg.ReplyTo,
			"system_event":    msg.SystemEvent,
			"created_at":      msg.CreatedAt,
		}
		if msg.ContentFile != "" {
			item["content_file"] = json.RawMessage(msg.ContentFile)
		}
		result = append(result, item)
	}

	okResponse(c, result)
}

func (s *APIServer) handleStartDirect(c *gin.Context) {
	agentID := getAgentID(c)
	var req struct {
		Target       string `json:"target" binding:"required"`
		FirstMessage string `json:"first_message" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		errResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	client, exists := s.pool.GetClient(agentID)
	if !exists {
		errResponse(c, http.StatusBadRequest, "agent not found")
		return
	}

	resp, err := client.Request("conv.start_direct", gin.H{
		"target":        req.Target,
		"first_message": req.FirstMessage,
	}, 10*time.Second)
	if err != nil {
		errResponse(c, http.StatusGatewayTimeout, "request timeout")
		return
	}

	if resp.Error != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"ok":    false,
			"error": resp.Error.Message,
			"code":  resp.Error.Code,
		})
		return
	}

	if err := saveStartedDirectConversation(s.store, agentID, req.Target, req.FirstMessage, resp.Data); err != nil {
		log.Printf("ERROR: save started direct conversation locally for agent %s: %v", agentID, err)
	}

	okResponse(c, resp.Data)
}

func saveStartedDirectConversation(localStore *store.SQLiteStore, agentID string, targetAgentID string, firstMessage string, response interface{}) error {
	responseBytes, err := json.Marshal(response)
	if err != nil {
		return err
	}
	var result struct {
		ConversationID string `json:"conversation_id"`
		MessageID      string `json:"message_id"`
	}
	if err := json.Unmarshal(responseBytes, &result); err != nil {
		return err
	}
	if result.ConversationID == "" || result.MessageID == "" {
		return fmt.Errorf("hub response missing conversation_id or message_id")
	}

	createdAt := time.Now().Format(time.RFC3339)
	if err := localStore.UpsertConversation(agentID, store.ConversationRecord{
		ConversationID:     result.ConversationID,
		Type:               "direct",
		Status:             "active",
		CreatedBy:          agentID,
		LastMessagePreview: firstMessage,
		LastMessageAt:      createdAt,
		CreatedAt:          createdAt,
	}); err != nil {
		return err
	}
	for _, memberID := range []string{agentID, targetAgentID} {
		if err := localStore.UpsertMember(agentID, result.ConversationID, store.MemberRecord{
			MemberAgentID: memberID,
			Role:          "member",
			JoinedAt:      createdAt,
		}); err != nil {
			return err
		}
	}
	return saveSentMessage(localStore, agentID, result.ConversationID, "text", firstMessage, nil, nil, "", response)
}

func saveSentMessage(localStore *store.SQLiteStore, agentID string, conversationID string, messageType string, contentText string, contentFile map[string]interface{}, mentions []string, replyTo string, response interface{}) error {
	responseBytes, err := json.Marshal(response)
	if err != nil {
		return err
	}
	var result struct {
		MessageID string `json:"message_id"`
		CreatedAt string `json:"created_at"`
	}
	if err := json.Unmarshal(responseBytes, &result); err != nil {
		return err
	}
	if result.MessageID == "" {
		return fmt.Errorf("hub response missing message_id")
	}
	if result.CreatedAt == "" {
		result.CreatedAt = time.Now().Format(time.RFC3339)
	}

	contentFileJSON := ""
	if contentFile != nil {
		encoded, err := json.Marshal(contentFile)
		if err != nil {
			return err
		}
		contentFileJSON = string(encoded)
	}

	if err := localStore.SaveMessage(agentID, store.MessageRecord{
		MessageID:      result.MessageID,
		ConversationID: conversationID,
		FromAgent:      agentID,
		MessageType:    messageType,
		ContentText:    contentText,
		ContentFile:    contentFileJSON,
		Mentions:       strings.Join(mentions, ","),
		ReplyTo:        replyTo,
		CreatedAt:      result.CreatedAt,
	}); err != nil {
		return err
	}

	preview := contentText
	if preview == "" && contentFileJSON != "" {
		preview = "[文件]"
	}
	return localStore.UpdateConversationLastMessage(agentID, conversationID, preview, result.CreatedAt)
}

func (s *APIServer) handleCreateGroup(c *gin.Context) {
	agentID := getAgentID(c)
	var req struct {
		Name         string   `json:"name" binding:"required"`
		InviteList   []string `json:"invite_list" binding:"required"`
		FirstMessage string   `json:"first_message"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		errResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	client, exists := s.pool.GetClient(agentID)
	if !exists {
		errResponse(c, http.StatusBadRequest, "agent not found")
		return
	}

	resp, err := client.Request("conv.create_group", gin.H{
		"name":          req.Name,
		"invite_list":   req.InviteList,
		"first_message": req.FirstMessage,
	}, 10*time.Second)
	if err != nil {
		errResponse(c, http.StatusGatewayTimeout, "request timeout")
		return
	}

	if resp.Error != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"ok":    false,
			"error": resp.Error.Message,
			"code":  resp.Error.Code,
		})
		return
	}

	okResponse(c, resp.Data)
}

func (s *APIServer) handleSendMessage(c *gin.Context) {
	agentID := getAgentID(c)
	convID := c.Param("id")

	var req struct {
		MessageType string                 `json:"message_type"`
		ContentText string                 `json:"content_text"`
		ContentFile map[string]interface{} `json:"content_file"`
		Mentions    []string               `json:"mentions"`
		ReplyTo     string                 `json:"reply_to"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		errResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	msgType := req.MessageType
	if msgType == "" {
		msgType = "text"
	}

	client, exists := s.pool.GetClient(agentID)
	if !exists {
		errResponse(c, http.StatusBadRequest, "agent not found")
		return
	}

	data := gin.H{
		"conversation_id": convID,
		"message_type":    msgType,
		"content_text":    req.ContentText,
		"mentions":        req.Mentions,
		"reply_to":        req.ReplyTo,
	}
	if req.ContentFile != nil {
		data["content_file"] = req.ContentFile
	}

	resp, err := client.Request("msg.send", data, 10*time.Second)
	if err != nil {
		errResponse(c, http.StatusGatewayTimeout, "request timeout")
		return
	}

	if resp.Error != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"ok":    false,
			"error": resp.Error.Message,
			"code":  resp.Error.Code,
		})
		return
	}

	if err := saveSentMessage(s.store, agentID, convID, msgType, req.ContentText, req.ContentFile, req.Mentions, req.ReplyTo, resp.Data); err != nil {
		log.Printf("ERROR: save sent message locally for agent %s: %v", agentID, err)
	}

	okResponse(c, resp.Data)
}

func (s *APIServer) handleInvite(c *gin.Context) {
	agentID := getAgentID(c)
	convID := c.Param("id")
	var req struct {
		Target string `json:"target" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		errResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	client, exists := s.pool.GetClient(agentID)
	if !exists {
		errResponse(c, http.StatusBadRequest, "agent not found")
		return
	}

	resp, err := client.Request("conv.invite", gin.H{
		"conversation_id": convID,
		"target":          req.Target,
	}, 10*time.Second)
	if err != nil {
		errResponse(c, http.StatusGatewayTimeout, "request timeout")
		return
	}

	if resp.Error != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"ok":    false,
			"error": resp.Error.Message,
			"code":  resp.Error.Code,
		})
		return
	}

	okResponse(c, resp.Data)
}

func (s *APIServer) handleLeave(c *gin.Context) {
	agentID := getAgentID(c)
	convID := c.Param("id")

	client, exists := s.pool.GetClient(agentID)
	if !exists {
		errResponse(c, http.StatusBadRequest, "agent not found")
		return
	}

	resp, err := client.Request("conv.leave", gin.H{
		"conversation_id": convID,
	}, 10*time.Second)
	if err != nil {
		errResponse(c, http.StatusGatewayTimeout, "request timeout")
		return
	}

	if resp.Error != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"ok":    false,
			"error": resp.Error.Message,
			"code":  resp.Error.Code,
		})
		return
	}

	okResponse(c, resp.Data)
}

func (s *APIServer) handleClose(c *gin.Context) {
	agentID := getAgentID(c)
	convID := c.Param("id")

	client, exists := s.pool.GetClient(agentID)
	if !exists {
		errResponse(c, http.StatusBadRequest, "agent not found")
		return
	}

	resp, err := client.Request("conv.close", gin.H{
		"conversation_id": convID,
	}, 10*time.Second)
	if err != nil {
		errResponse(c, http.StatusGatewayTimeout, "request timeout")
		return
	}

	if resp.Error != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"ok":    false,
			"error": resp.Error.Message,
			"code":  resp.Error.Code,
		})
		return
	}

	okResponse(c, resp.Data)
}

func (s *APIServer) handleMarkRead(c *gin.Context) {
	agentID := getAgentID(c)
	convID := c.Param("id")

	var req struct {
		MessageID string `json:"message_id"`
	}
	c.ShouldBindJSON(&req)

	s.store.MarkConversationRead(agentID, convID)
	okResponse(c, gin.H{"ok": true})
}

// ============================================================
// Friend handlers
// ============================================================

func (s *APIServer) handleListFriends(c *gin.Context) {
	agentID := getAgentID(c)
	group := c.Query("group")
	onlineOnly := c.Query("online_only") == "true"

	friends, err := s.store.ListFriends(agentID, group, onlineOnly)
	if err != nil {
		errResponse(c, http.StatusInternalServerError, err.Error())
		return
	}

	result := make([]gin.H, 0, len(friends))
	for _, f := range friends {
		result = append(result, gin.H{
			"agent_id":    f.FriendID,
			"name":        f.Name,
			"nickname":    f.Nickname,
			"group":       f.Group,
			"trust_level": f.TrustLevel,
			"status_text": f.StatusText,
			"online":      f.Online,
			"capabilities": func() interface{} {
				if f.Capabilities != "" {
					return json.RawMessage(f.Capabilities)
				}
				return []interface{}{}
			}(),
		})
	}

	okResponse(c, result)
}

func (s *APIServer) handleFriendRequests(c *gin.Context) {
	agentID := getAgentID(c)

	requests, err := s.store.ListFriendRequests(agentID)
	if err != nil {
		errResponse(c, http.StatusInternalServerError, err.Error())
		return
	}

	result := make([]gin.H, 0, len(requests))
	for _, r := range requests {
		result = append(result, gin.H{
			"from_agent": r.FromAgent,
			"name":       r.Name,
			"message":    r.Message,
			"created_at": r.CreatedAt,
		})
	}

	okResponse(c, result)
}

func (s *APIServer) handleSendFriendRequest(c *gin.Context) {
	agentID := getAgentID(c)
	var req struct {
		Target  string `json:"target" binding:"required"`
		Message string `json:"message"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		errResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	client, exists := s.pool.GetClient(agentID)
	if !exists {
		errResponse(c, http.StatusBadRequest, "agent not found")
		return
	}

	resp, err := client.Request("friend.request", gin.H{
		"target":  req.Target,
		"message": req.Message,
	}, 10*time.Second)
	if err != nil {
		errResponse(c, http.StatusGatewayTimeout, "request timeout")
		return
	}

	if resp.Error != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"ok":    false,
			"error": resp.Error.Message,
			"code":  resp.Error.Code,
		})
		return
	}

	okResponse(c, resp.Data)
}

func (s *APIServer) handleAcceptFriend(c *gin.Context) {
	agentID := getAgentID(c)
	target := c.Param("agent_id")

	client, exists := s.pool.GetClient(agentID)
	if !exists {
		errResponse(c, http.StatusBadRequest, "agent not found")
		return
	}

	resp, err := client.Request("friend.accept", gin.H{
		"target": target,
	}, 10*time.Second)
	if err != nil {
		errResponse(c, http.StatusGatewayTimeout, "request timeout")
		return
	}

	if resp.Error != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"ok":    false,
			"error": resp.Error.Message,
			"code":  resp.Error.Code,
		})
		return
	}

	s.store.RemoveFriendRequest(agentID, target)
	okResponse(c, gin.H{"ok": true})
}

func (s *APIServer) handleRejectFriend(c *gin.Context) {
	agentID := getAgentID(c)
	target := c.Param("agent_id")

	client, exists := s.pool.GetClient(agentID)
	if !exists {
		errResponse(c, http.StatusBadRequest, "agent not found")
		return
	}

	resp, err := client.Request("friend.reject", gin.H{
		"target": target,
	}, 10*time.Second)
	if err != nil {
		errResponse(c, http.StatusGatewayTimeout, "request timeout")
		return
	}

	if resp.Error != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"ok":    false,
			"error": resp.Error.Message,
			"code":  resp.Error.Code,
		})
		return
	}

	s.store.RemoveFriendRequest(agentID, target)
	okResponse(c, gin.H{"ok": true})
}

func (s *APIServer) handleSetNickname(c *gin.Context) {
	agentID := getAgentID(c)
	target := c.Param("agent_id")
	var req struct {
		Nickname string `json:"nickname" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		errResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	client, exists := s.pool.GetClient(agentID)
	if !exists {
		errResponse(c, http.StatusBadRequest, "agent not found")
		return
	}

	resp, err := client.Request("friend.set_nickname", gin.H{
		"target":   target,
		"nickname": req.Nickname,
	}, 10*time.Second)
	if err != nil {
		errResponse(c, http.StatusGatewayTimeout, "request timeout")
		return
	}

	if resp.Error != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"ok":    false,
			"error": resp.Error.Message,
			"code":  resp.Error.Code,
		})
		return
	}

	// 更新本地缓存
	if friend, err := s.store.GetFriend(agentID, target); err == nil && friend != nil {
		friend.Nickname = req.Nickname
		s.store.UpsertFriend(agentID, *friend)
	}

	okResponse(c, gin.H{"ok": true})
}

func (s *APIServer) handleSetGroup(c *gin.Context) {
	agentID := getAgentID(c)
	target := c.Param("agent_id")
	var req struct {
		Group string `json:"group"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		errResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	client, exists := s.pool.GetClient(agentID)
	if !exists {
		errResponse(c, http.StatusBadRequest, "agent not found")
		return
	}

	resp, err := client.Request("friend.set_group", gin.H{
		"target": target,
		"group":  req.Group,
	}, 10*time.Second)
	if err != nil {
		errResponse(c, http.StatusGatewayTimeout, "request timeout")
		return
	}

	if resp.Error != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"ok":    false,
			"error": resp.Error.Message,
			"code":  resp.Error.Code,
		})
		return
	}

	if friend, err := s.store.GetFriend(agentID, target); err == nil && friend != nil {
		friend.Group = req.Group
		s.store.UpsertFriend(agentID, *friend)
	}

	okResponse(c, gin.H{"ok": true})
}

func (s *APIServer) handleSetTrust(c *gin.Context) {
	agentID := getAgentID(c)
	target := c.Param("agent_id")
	var req struct {
		Level string `json:"level" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		errResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	client, exists := s.pool.GetClient(agentID)
	if !exists {
		errResponse(c, http.StatusBadRequest, "agent not found")
		return
	}

	resp, err := client.Request("friend.set_trust", gin.H{
		"target": target,
		"level":  req.Level,
	}, 10*time.Second)
	if err != nil {
		errResponse(c, http.StatusGatewayTimeout, "request timeout")
		return
	}

	if resp.Error != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"ok":    false,
			"error": resp.Error.Message,
			"code":  resp.Error.Code,
		})
		return
	}

	if friend, err := s.store.GetFriend(agentID, target); err == nil && friend != nil {
		friend.TrustLevel = req.Level
		s.store.UpsertFriend(agentID, *friend)
	}

	okResponse(c, gin.H{"ok": true})
}

func (s *APIServer) handleBlockFriend(c *gin.Context) {
	agentID := getAgentID(c)
	target := c.Param("agent_id")

	client, exists := s.pool.GetClient(agentID)
	if !exists {
		errResponse(c, http.StatusBadRequest, "agent not found")
		return
	}

	resp, err := client.Request("friend.block", gin.H{
		"target": target,
	}, 10*time.Second)
	if err != nil {
		errResponse(c, http.StatusGatewayTimeout, "request timeout")
		return
	}

	if resp.Error != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"ok":    false,
			"error": resp.Error.Message,
			"code":  resp.Error.Code,
		})
		return
	}

	if friend, err := s.store.GetFriend(agentID, target); err == nil && friend != nil {
		friend.TrustLevel = "blocked"
		s.store.UpsertFriend(agentID, *friend)
	}

	okResponse(c, gin.H{"ok": true})
}

func (s *APIServer) handleRemoveFriend(c *gin.Context) {
	agentID := getAgentID(c)
	target := c.Param("agent_id")

	client, exists := s.pool.GetClient(agentID)
	if !exists {
		errResponse(c, http.StatusBadRequest, "agent not found")
		return
	}

	resp, err := client.Request("friend.remove", gin.H{
		"target": target,
	}, 10*time.Second)
	if err != nil {
		errResponse(c, http.StatusGatewayTimeout, "request timeout")
		return
	}

	if resp.Error != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"ok":    false,
			"error": resp.Error.Message,
			"code":  resp.Error.Code,
		})
		return
	}

	okResponse(c, gin.H{"ok": true})
}

func (s *APIServer) handleFriendProfile(c *gin.Context) {
	agentID := getAgentID(c)
	target := c.Param("agent_id")

	friend, err := s.store.GetFriend(agentID, target)
	if err != nil {
		errResponse(c, http.StatusInternalServerError, err.Error())
		return
	}

	if friend == nil {
		errResponse(c, http.StatusNotFound, "friend not found")
		return
	}

	okResponse(c, gin.H{
		"agent_id":    friend.FriendID,
		"name":        friend.Name,
		"nickname":    friend.Nickname,
		"group":       friend.Group,
		"trust_level": friend.TrustLevel,
		"status_text": friend.StatusText,
		"online":      friend.Online,
		"is_friend":   true,
		"capabilities": func() interface{} {
			if friend.Capabilities != "" {
				return json.RawMessage(friend.Capabilities)
			}
			return []interface{}{}
		}(),
	})
}

// ============================================================
// Discover handler
// ============================================================

func (s *APIServer) handleDiscover(c *gin.Context) {
	agentID := getAgentID(c)
	var req struct {
		Keyword    string   `json:"keyword"`
		Skill      string   `json:"skill"`
		Tags       []string `json:"tags"`
		OnlineOnly bool     `json:"online_only"`
		Limit      int      `json:"limit"`
		Offset     int      `json:"offset"`
	}
	c.ShouldBindJSON(&req)

	if req.Limit <= 0 {
		req.Limit = 20
	}

	client, exists := s.pool.GetClient(agentID)
	if !exists {
		errResponse(c, http.StatusBadRequest, "agent not found")
		return
	}

	resp, err := client.Request("discover.search", gin.H{
		"keyword":     req.Keyword,
		"skill":       req.Skill,
		"tags":        req.Tags,
		"online_only": req.OnlineOnly,
		"limit":       req.Limit,
		"offset":      req.Offset,
	}, 10*time.Second)
	if err != nil {
		errResponse(c, http.StatusGatewayTimeout, "request timeout")
		return
	}

	if resp.Error != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"ok":    false,
			"error": resp.Error.Message,
			"code":  resp.Error.Code,
		})
		return
	}

	okResponse(c, resp.Data)
}

// ============================================================
// Profile handlers
// ============================================================

func (s *APIServer) handleUpdateStatus(c *gin.Context) {
	agentID := getAgentID(c)
	var req struct {
		StatusText string `json:"status_text" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		errResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	client, exists := s.pool.GetClient(agentID)
	if !exists {
		errResponse(c, http.StatusBadRequest, "agent not found")
		return
	}

	resp, err := client.Request("agent.update_status", gin.H{
		"status_text": req.StatusText,
	}, 10*time.Second)
	if err != nil {
		errResponse(c, http.StatusGatewayTimeout, "request timeout")
		return
	}

	if resp.Error != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"ok":    false,
			"error": resp.Error.Message,
			"code":  resp.Error.Code,
		})
		return
	}

	s.updateLocalAgentConfig(agentID, func(agent *config.AgentConfig) {
		agent.StatusText = req.StatusText
	})
	okResponse(c, gin.H{"ok": true})
}

func (s *APIServer) handleUpdateCapabilities(c *gin.Context) {
	agentID := getAgentID(c)
	var req struct {
		Capabilities []struct {
			Skill string   `json:"skill"`
			Tags  []string `json:"tags"`
		} `json:"capabilities" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		errResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	client, exists := s.pool.GetClient(agentID)
	if !exists {
		errResponse(c, http.StatusBadRequest, "agent not found")
		return
	}

	caps := make([]map[string]interface{}, 0, len(req.Capabilities))
	for _, c := range req.Capabilities {
		caps = append(caps, map[string]interface{}{
			"skill": c.Skill,
			"tags":  c.Tags,
		})
	}

	resp, err := client.Request("agent.update_capabilities", gin.H{
		"capabilities": caps,
	}, 10*time.Second)
	if err != nil {
		errResponse(c, http.StatusGatewayTimeout, "request timeout")
		return
	}

	if resp.Error != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"ok":    false,
			"error": resp.Error.Message,
			"code":  resp.Error.Code,
		})
		return
	}

	s.updateLocalAgentConfig(agentID, func(agent *config.AgentConfig) {
		agent.Capabilities = make([]config.CapabilityItem, 0, len(req.Capabilities))
		for _, capability := range req.Capabilities {
			agent.Capabilities = append(agent.Capabilities, config.CapabilityItem{Skill: capability.Skill, Tags: capability.Tags})
		}
	})
	okResponse(c, gin.H{"ok": true})
}

func (s *APIServer) handleUpdateProfile(c *gin.Context) {
	agentID := getAgentID(c)
	var req struct {
		Name       string `json:"name" binding:"required"`
		AvatarURL  string `json:"avatar_url"`
		StatusText string `json:"status_text"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		errResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	if !s.requestAgentUpdate(c, agentID, "agent.update_profile", gin.H{
		"name": req.Name, "avatar_url": req.AvatarURL, "status_text": req.StatusText,
	}) {
		return
	}
	s.updateLocalAgentConfig(agentID, func(agent *config.AgentConfig) {
		agent.Name = req.Name
		agent.StatusText = req.StatusText
	})
	s.agentNameMap[agentID] = req.Name
	okResponse(c, gin.H{"ok": true})
}

func (s *APIServer) handleUpdateAvailability(c *gin.Context) {
	agentID := getAgentID(c)
	var req struct {
		Availability config.AvailabilityConf `json:"availability" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		errResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	if req.Availability.MaxConcurrentConversations < 0 {
		errResponse(c, http.StatusBadRequest, "max_concurrent_conversations must be non-negative")
		return
	}
	if !s.requestAgentUpdate(c, agentID, "agent.update_availability", gin.H{"availability": req.Availability}) {
		return
	}
	s.updateLocalAgentConfig(agentID, func(agent *config.AgentConfig) {
		agent.Availability = req.Availability
	})
	okResponse(c, gin.H{"ok": true})
}

func (s *APIServer) handleUpdateOwner(c *gin.Context) {
	agentID := getAgentID(c)
	var req struct {
		OwnerName  string `json:"owner_name"`
		OwnerEmail string `json:"owner_email"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		errResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	if !s.requestAgentUpdate(c, agentID, "agent.update_owner", gin.H{
		"owner_name": req.OwnerName, "owner_email": req.OwnerEmail,
	}) {
		return
	}
	s.updateLocalAgentConfig(agentID, func(agent *config.AgentConfig) {
		agent.OwnerName = req.OwnerName
		agent.OwnerEmail = req.OwnerEmail
	})
	okResponse(c, gin.H{"ok": true})
}

func (s *APIServer) requestAgentUpdate(c *gin.Context, agentID string, messageType string, payload gin.H) bool {
	client, exists := s.pool.GetClient(agentID)
	if !exists {
		errResponse(c, http.StatusBadRequest, "agent not found")
		return false
	}
	resp, err := client.Request(messageType, payload, 10*time.Second)
	if err != nil {
		errResponse(c, http.StatusGatewayTimeout, "request timeout")
		return false
	}
	if resp.Error != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": resp.Error.Message, "code": resp.Error.Code})
		return false
	}
	return true
}

func (s *APIServer) updateLocalAgentConfig(agentID string, update func(*config.AgentConfig)) {
	for index := range s.config.Agents {
		if s.config.Agents[index].ID == agentID {
			update(&s.config.Agents[index])
			if err := config.Save(s.config.Path, s.config); err != nil {
				log.Printf("WARN: save updated agent profile: %v", err)
			}
			return
		}
	}
}

// ============================================================
// Approval handlers
// ============================================================

func (s *APIServer) handleRequestApproval(c *gin.Context) {
	agentID := getAgentID(c)
	var req struct {
		Question string                    `json:"question" binding:"required"`
		Options  []approval.ApprovalOption `json:"options" binding:"required"`
		Context  string                    `json:"context"`
		Urgency  string                    `json:"urgency"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		errResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	urgency := req.Urgency
	if urgency == "" {
		urgency = "normal"
	}

	agentName := s.agentNameMap[agentID]
	if agentName == "" {
		agentName = agentID
	}

	id, err := s.approvalMgr.RequestApproval(agentID, agentName, req.Question, req.Options, req.Context, urgency)
	if err != nil {
		errResponse(c, http.StatusInternalServerError, err.Error())
		return
	}

	okResponse(c, gin.H{"approval_id": id})
}

func (s *APIServer) handleGetApproval(c *gin.Context) {
	id := c.Param("id")

	approval, err := s.approvalMgr.GetApproval(id)
	if err != nil {
		errResponse(c, http.StatusInternalServerError, err.Error())
		return
	}
	if approval == nil {
		errResponse(c, http.StatusNotFound, "approval not found")
		return
	}

	okResponse(c, gin.H{
		"id":       approval.ID,
		"agent_id": approval.AgentID,
		"type":     approval.Type,
		"title":    approval.Title,
		"question": approval.Question,
		"options": func() interface{} {
			if approval.Options != "" {
				return json.RawMessage(approval.Options)
			}
			return []interface{}{}
		}(),
		"context":    approval.Context,
		"urgency":    approval.Urgency,
		"status":     approval.Status,
		"decision":   approval.Decision,
		"comment":    approval.Comment,
		"created_at": approval.CreatedAt,
		"decided_at": approval.DecidedAt,
		"timeout_at": approval.TimeoutAt,
	})
}

func (s *APIServer) handlePendingApprovals(c *gin.Context) {
	approvals, err := s.approvalMgr.ListPending()
	if err != nil {
		errResponse(c, http.StatusInternalServerError, err.Error())
		return
	}

	result := make([]gin.H, 0, len(approvals))
	for _, a := range approvals {
		agentName := s.agentNameMap[a.AgentID]
		if agentName == "" {
			agentName = a.AgentID
		}
		result = append(result, gin.H{
			"id":         a.ID,
			"agent_id":   a.AgentID,
			"agent_name": agentName,
			"type":       a.Type,
			"title":      a.Title,
			"question":   a.Question,
			"urgency":    a.Urgency,
			"created_at": a.CreatedAt,
			"timeout_at": a.TimeoutAt,
		})
	}

	okResponse(c, result)
}

func (s *APIServer) handleDecideApproval(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		Decision string `json:"decision" binding:"required"`
		Comment  string `json:"comment"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		errResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	if err := s.approvalMgr.Decide(id, req.Decision, req.Comment); err != nil {
		errResponse(c, http.StatusInternalServerError, err.Error())
		return
	}

	okResponse(c, gin.H{"ok": true})
}

// FileInfo 文件信息结构
type FileInfo struct {
	Filename   string `json:"filename"`
	Size       int64  `json:"size"`
	StorageKey string `json:"storage_key"`
}

// 确保使用 store 包（防止未使用）
var _ = store.ApprovalRecord{}
