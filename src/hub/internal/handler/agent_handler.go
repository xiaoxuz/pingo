package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/pingo/hub/internal/auth"
	"github.com/pingo/hub/internal/model"
	"github.com/pingo/hub/internal/service"
)

type AgentHandler struct {
	agentService *service.AgentService
}

func NewAgentHandler(agentService *service.AgentService) *AgentHandler {
	return &AgentHandler{agentService: agentService}
}

// Register 注册新 Agent（公开接口，管理用）
func (h *AgentHandler) Register(c *gin.Context) {
	var req struct {
		AgentID      string             `json:"agent_id"`
		Name         string             `json:"name" binding:"required"`
		OwnerName    string             `json:"owner_name"`
		OwnerEmail   string             `json:"owner_email"`
		StatusText   string             `json:"status_text"`
		Capabilities []model.Capability `json:"capabilities"`
		Availability model.Availability `json:"availability"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": err.Error()})
		return
	}
	if req.AgentID != "" {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "agent_id is assigned by Hub; do not provide it"})
		return
	}
	if req.Availability.MaxConcurrentConversations < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "max_concurrent_conversations must be non-negative"})
		return
	}

	capabilities, err := json.Marshal(req.Capabilities)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": err.Error()})
		return
	}
	availability, err := json.Marshal(req.Availability)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": err.Error()})
		return
	}
	agent, token, err := h.agentService.Register(c.Request.Context(), model.Agent{
		Name: req.Name, OwnerName: req.OwnerName, OwnerEmail: req.OwnerEmail,
		StatusText: req.StatusText, Capabilities: capabilities, Availability: availability,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"ok": true,
		"data": gin.H{
			"agent_id": agent.AgentID,
			"name":     agent.Name,
			"token":    token,
		},
	})
}

// GetMe 获取自己的信息
func (h *AgentHandler) GetMe(c *gin.Context) {
	agentID := auth.GetAgentID(c)
	agent, err := h.agentService.GetByID(c.Request.Context(), agentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": err.Error()})
		return
	}
	if agent == nil {
		c.JSON(http.StatusNotFound, gin.H{"ok": false, "error": "agent not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": agent})
}

// UpdateStatus 更新状态签名
func (h *AgentHandler) UpdateStatus(c *gin.Context) {
	agentID := auth.GetAgentID(c)
	var req struct {
		StatusText string `json:"status_text"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": err.Error()})
		return
	}
	if err := h.agentService.UpdateStatus(c.Request.Context(), agentID, req.StatusText); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// UpdateCapabilities 更新能力
func (h *AgentHandler) UpdateCapabilities(c *gin.Context) {
	agentID := auth.GetAgentID(c)
	var req struct {
		Capabilities []model.Capability `json:"capabilities"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": err.Error()})
		return
	}
	if err := h.agentService.UpdateCapabilities(c.Request.Context(), agentID, req.Capabilities); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// UpdateOwner 更新操作者信息
func (h *AgentHandler) UpdateOwner(c *gin.Context) {
	agentID := auth.GetAgentID(c)
	var req struct {
		OwnerName  string `json:"owner_name"`
		OwnerEmail string `json:"owner_email"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": err.Error()})
		return
	}
	if err := h.agentService.UpdateOwner(c.Request.Context(), agentID, req.OwnerName, req.OwnerEmail); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// AdminListAgents 管理端 - 列出所有 Agent
func (h *AgentHandler) AdminListAgents(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

	agents, total, err := h.agentService.GetAll(c.Request.Context(), limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"ok": true,
		"data": gin.H{
			"agents": agents,
			"total":  total,
		},
	})
}

// AdminStats 管理端 - 统计数据
func (h *AgentHandler) AdminStats(c *gin.Context) {
	stats, err := h.agentService.GetStats(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": stats})
}

// AdminResetToken 管理端 - 重置 token
func (h *AgentHandler) AdminResetToken(c *gin.Context) {
	agentID := c.Param("id")
	newToken, err := h.agentService.ResetToken(c.Request.Context(), agentID)
	if err != nil {
		if err == service.ErrAgentNotFound {
			c.JSON(http.StatusNotFound, gin.H{"ok": false, "error": "agent not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": gin.H{"token": newToken}})
}
