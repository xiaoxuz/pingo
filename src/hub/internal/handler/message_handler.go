package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/pingo/hub/internal/auth"
	"github.com/pingo/hub/internal/model"
	"github.com/pingo/hub/internal/service"
	"github.com/gin-gonic/gin"
)

type MessageHandler struct {
	msgService  *service.MessageService
	convService *service.ConversationService
}

func NewMessageHandler(msgService *service.MessageService, convService *service.ConversationService) *MessageHandler {
	return &MessageHandler{
		msgService:  msgService,
		convService: convService,
	}
}

// List 消息列表
func (h *MessageHandler) List(c *gin.Context) {
	agentID := auth.GetAgentID(c)
	convID := c.Param("id")

	isMember, err := h.convService.IsMember(c.Request.Context(), convID, agentID)
	if err != nil || !isMember {
		c.JSON(http.StatusForbidden, gin.H{"ok": false, "error": "not a member"})
		return
	}

	afterMessageID := c.Query("after_message_id")
	limit := 50
	if l := c.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}

	messages, err := h.msgService.ListMessages(c.Request.Context(), convID, agentID, afterMessageID, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": err.Error()})
		return
	}

	result := make([]gin.H, 0, len(messages))
	for _, msg := range messages {
		item := gin.H{
			"message_id":      msg.MessageID,
			"conversation_id": msg.ConversationID,
			"from_agent":      msg.FromAgent,
			"message_type":    msg.MessageType,
			"content_text":    msg.ContentText,
			"mentions":        msg.Mentions,
			"reply_to":        msg.ReplyTo,
			"system_event":    msg.SystemEvent,
			"created_at":      msg.CreatedAt.Format(time.RFC3339),
		}
		if msg.ContentFile != nil && len(msg.ContentFile) > 0 {
			item["content_file"] = msg.ContentFile
		}
		result = append(result, item)
	}

	c.JSON(http.StatusOK, gin.H{"ok": true, "data": result})
}

// Send 发送消息
func (h *MessageHandler) Send(c *gin.Context) {
	agentID := auth.GetAgentID(c)
	convID := c.Param("id")

	var req struct {
		MessageType string                  `json:"message_type"`
		ContentText string                  `json:"content_text"`
		ContentFile *model.ContentFile      `json:"content_file"`
		Mentions    []string                `json:"mentions"`
		ReplyTo     string                  `json:"reply_to"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": err.Error()})
		return
	}

	msgType := req.MessageType
	if msgType == "" {
		msgType = model.MsgTypeText
	}

	msg, err := h.msgService.Send(c.Request.Context(), agentID, convID, msgType, req.ContentText, req.ContentFile, req.Mentions, req.ReplyTo)
	if err != nil {
		switch err {
		case service.ErrConvNotFound:
			c.JSON(http.StatusNotFound, gin.H{"ok": false, "error": "conversation not found"})
		case service.ErrConvClosed:
			c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "conversation closed", "code": "CONV_CLOSED"})
		case service.ErrNotMember:
			c.JSON(http.StatusForbidden, gin.H{"ok": false, "error": "not a member"})
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
			"message_id": msg.MessageID,
			"created_at": msg.CreatedAt.Format(time.RFC3339),
		},
	})
}
