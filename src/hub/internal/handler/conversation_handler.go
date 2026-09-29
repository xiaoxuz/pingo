package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/pingo/hub/internal/auth"
	"github.com/pingo/hub/internal/service"
	"github.com/gin-gonic/gin"
)

type ConversationHandler struct {
	convService *service.ConversationService
	agentService *service.AgentService
}

func NewConversationHandler(convService *service.ConversationService, agentService *service.AgentService) *ConversationHandler {
	return &ConversationHandler{
		convService:  convService,
		agentService: agentService,
	}
}

// List 会话列表
func (h *ConversationHandler) List(c *gin.Context) {
	agentID := auth.GetAgentID(c)
	status := c.DefaultQuery("status", "active")

	convs, err := h.convService.ListByAgent(c.Request.Context(), agentID, status)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": err.Error()})
		return
	}

	result := make([]gin.H, 0, len(convs))
	for _, conv := range convs {
		members, _ := h.convService.ListMembers(c.Request.Context(), conv.ConversationID)
		memberIDs := make([]string, 0, len(members))
		for _, m := range members {
			memberIDs = append(memberIDs, m.AgentID)
		}
		unreadCount, _ := h.convService.GetUnreadCount(c.Request.Context(), conv.ConversationID, agentID)

		item := gin.H{
			"conversation_id":      conv.ConversationID,
			"type":                 conv.Type,
			"name":                 conv.Name,
			"status":               conv.Status,
			"created_by":           conv.CreatedBy,
			"last_message_preview": conv.LastMessagePreview,
			"unread_count":         unreadCount,
			"members":              memberIDs,
			"created_at":           conv.CreatedAt.Format(time.RFC3339),
		}
		if conv.LastMessageAt != nil {
			item["last_message_at"] = conv.LastMessageAt.Format(time.RFC3339)
		}
		if conv.ClosedAt != nil {
			item["closed_at"] = conv.ClosedAt.Format(time.RFC3339)
		}
		result = append(result, item)
	}

	c.JSON(http.StatusOK, gin.H{"ok": true, "data": result})
}

// Get 会话详情
func (h *ConversationHandler) Get(c *gin.Context) {
	agentID := auth.GetAgentID(c)
	convID := c.Param("id")

	isMember, err := h.convService.IsMember(c.Request.Context(), convID, agentID)
	if err != nil || !isMember {
		c.JSON(http.StatusForbidden, gin.H{"ok": false, "error": "not a member"})
		return
	}

	conv, err := h.convService.GetByID(c.Request.Context(), convID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": err.Error()})
		return
	}
	if conv == nil {
		c.JSON(http.StatusNotFound, gin.H{"ok": false, "error": "conversation not found"})
		return
	}

	members, _ := h.convService.ListMembers(c.Request.Context(), convID)
	memberIDs := make([]string, 0, len(members))
	for _, m := range members {
		memberIDs = append(memberIDs, m.AgentID)
	}

	c.JSON(http.StatusOK, gin.H{
		"ok": true,
		"data": gin.H{
			"conversation_id": conv.ConversationID,
			"type":            conv.Type,
			"name":            conv.Name,
			"status":          conv.Status,
			"created_by":      conv.CreatedBy,
			"members":         memberIDs,
			"created_at":      conv.CreatedAt.Format(time.RFC3339),
		},
	})
}

// ListMembers 成员列表
func (h *ConversationHandler) ListMembers(c *gin.Context) {
	agentID := auth.GetAgentID(c)
	convID := c.Param("id")

	isMember, err := h.convService.IsMember(c.Request.Context(), convID, agentID)
	if err != nil || !isMember {
		c.JSON(http.StatusForbidden, gin.H{"ok": false, "error": "not a member"})
		return
	}

	members, err := h.convService.ListMembers(c.Request.Context(), convID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": err.Error()})
		return
	}

	result := make([]gin.H, 0, len(members))
	for _, m := range members {
		agent, _ := h.agentService.GetByID(c.Request.Context(), m.AgentID)
		name := m.AgentID
		if agent != nil {
			name = agent.Name
		}
		item := gin.H{
			"agent_id":  m.AgentID,
			"name":      name,
			"role":      m.Role,
			"joined_at": m.JoinedAt.Format(time.RFC3339),
			"active":    m.IsActive(),
		}
		if m.LeftAt != nil {
			item["left_at"] = m.LeftAt.Format(time.RFC3339)
		}
		result = append(result, item)
	}

	c.JSON(http.StatusOK, gin.H{"ok": true, "data": result})
}

// StartDirect 发起单聊
func (h *ConversationHandler) StartDirect(c *gin.Context) {
	agentID := auth.GetAgentID(c)
	var req struct {
		Target       string `json:"target" binding:"required"`
		FirstMessage string `json:"first_message" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": err.Error()})
		return
	}

	conv, msg, err := h.convService.StartDirect(c.Request.Context(), agentID, req.Target, req.FirstMessage)
	if err != nil {
		switch err {
		case service.ErrNotFriends:
			c.JSON(http.StatusForbidden, gin.H{"ok": false, "error": "not friends", "code": "NOT_FRIENDS"})
		case service.ErrBlocked:
			c.JSON(http.StatusForbidden, gin.H{"ok": false, "error": "blocked", "code": "BLOCKED"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": err.Error()})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"ok": true,
		"data": gin.H{
			"conversation_id": conv.ConversationID,
			"message_id":      msg.MessageID,
		},
	})
}

// CreateGroup 创建群聊
func (h *ConversationHandler) CreateGroup(c *gin.Context) {
	agentID := auth.GetAgentID(c)
	var req struct {
		Name         string   `json:"name" binding:"required"`
		InviteList   []string `json:"invite_list" binding:"required"`
		FirstMessage string   `json:"first_message"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": err.Error()})
		return
	}

	conv, firstMsg, err := h.convService.CreateGroup(c.Request.Context(), agentID, req.Name, req.InviteList, req.FirstMessage)
	if err != nil {
		if err == service.ErrNotFriends {
			c.JSON(http.StatusForbidden, gin.H{"ok": false, "error": "not friends with all invitees", "code": "NOT_FRIENDS"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": err.Error()})
		return
	}

	data := gin.H{
		"conversation_id": conv.ConversationID,
		"name":            conv.Name,
	}
	if firstMsg != nil {
		data["message_id"] = firstMsg.MessageID
	}

	c.JSON(http.StatusOK, gin.H{"ok": true, "data": data})
}

// Invite 邀请入群
func (h *ConversationHandler) Invite(c *gin.Context) {
	agentID := auth.GetAgentID(c)
	convID := c.Param("id")
	var req struct {
		Target string `json:"target" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": err.Error()})
		return
	}

	err := h.convService.Invite(c.Request.Context(), agentID, convID, req.Target)
	if err != nil {
		switch err {
		case service.ErrConvNotFound:
			c.JSON(http.StatusNotFound, gin.H{"ok": false, "error": "conversation not found"})
		case service.ErrConvClosed:
			c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "conversation closed", "code": "CONV_CLOSED"})
		case service.ErrNotMember:
			c.JSON(http.StatusForbidden, gin.H{"ok": false, "error": "not a member"})
		case service.ErrAlreadyMember:
			c.JSON(http.StatusConflict, gin.H{"ok": false, "error": "already a member"})
		case service.ErrNotFriends:
			c.JSON(http.StatusForbidden, gin.H{"ok": false, "error": "not friends", "code": "NOT_FRIENDS"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": err.Error()})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Leave 退出群聊
func (h *ConversationHandler) Leave(c *gin.Context) {
	agentID := auth.GetAgentID(c)
	convID := c.Param("id")

	err := h.convService.Leave(c.Request.Context(), agentID, convID)
	if err != nil {
		switch err {
		case service.ErrConvNotFound:
			c.JSON(http.StatusNotFound, gin.H{"ok": false, "error": "conversation not found"})
		case service.ErrConvClosed:
			c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "conversation closed"})
		case service.ErrNotMember:
			c.JSON(http.StatusForbidden, gin.H{"ok": false, "error": "not a member"})
		case service.ErrCreatorCannotLeave:
			c.JSON(http.StatusForbidden, gin.H{"ok": false, "error": "creator cannot leave, close the conversation instead", "code": "NOT_CREATOR"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": err.Error()})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Close 关闭会话
func (h *ConversationHandler) Close(c *gin.Context) {
	agentID := auth.GetAgentID(c)
	convID := c.Param("id")

	err := h.convService.Close(c.Request.Context(), agentID, convID)
	if err != nil {
		switch err {
		case service.ErrConvNotFound:
			c.JSON(http.StatusNotFound, gin.H{"ok": false, "error": "conversation not found"})
		case service.ErrConvClosed:
			c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "already closed"})
		case service.ErrNotMember:
			c.JSON(http.StatusForbidden, gin.H{"ok": false, "error": "not a member"})
		case service.ErrNotCreator:
			c.JSON(http.StatusForbidden, gin.H{"ok": false, "error": "only creator can close group chat", "code": "NOT_CREATOR"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": err.Error()})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// MarkRead 标记已读
func (h *ConversationHandler) MarkRead(c *gin.Context) {
	agentID := auth.GetAgentID(c)
	convID := c.Param("id")
	var req struct {
		MessageID string `json:"message_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": err.Error()})
		return
	}

	isMember, _ := h.convService.IsMember(c.Request.Context(), convID, agentID)
	if !isMember {
		c.JSON(http.StatusForbidden, gin.H{"ok": false, "error": "not a member"})
		return
	}

	if err := h.convService.MarkRead(c.Request.Context(), convID, agentID, req.MessageID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func atoiStr(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
