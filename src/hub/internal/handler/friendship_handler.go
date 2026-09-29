package handler

import (
	"net/http"

	"github.com/pingo/hub/internal/auth"
	"github.com/pingo/hub/internal/service"
	"github.com/gin-gonic/gin"
)

type FriendshipHandler struct {
	friendshipService *service.FriendshipService
	agentService      *service.AgentService
}

func NewFriendshipHandler(friendshipService *service.FriendshipService, agentService *service.AgentService) *FriendshipHandler {
	return &FriendshipHandler{
		friendshipService: friendshipService,
		agentService:      agentService,
	}
}

// ListFriends 好友列表
func (h *FriendshipHandler) ListFriends(c *gin.Context) {
	agentID := auth.GetAgentID(c)
	group := c.Query("group")

	friendships, err := h.friendshipService.ListFriends(c.Request.Context(), agentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": err.Error()})
		return
	}

	// 按分组过滤
	result := make([]gin.H, 0)
	for _, f := range friendships {
		friendID, nickname, friendGroup, trustLevel, _ := f.GetFriendSide(agentID)
		if group != "" && friendGroup != group {
			continue
		}
		agent, _ := h.agentService.GetByID(c.Request.Context(), friendID)
		if agent == nil {
			continue
		}
		result = append(result, gin.H{
			"agent_id":    friendID,
			"name":        agent.Name,
			"avatar_url":  agent.AvatarURL,
			"status_text": agent.StatusText,
			"online":      agent.Online,
			"nickname":    nickname,
			"group":       friendGroup,
			"trust_level": trustLevel,
		})
	}

	c.JSON(http.StatusOK, gin.H{"ok": true, "data": result})
}

// ListRequests 好友请求列表
func (h *FriendshipHandler) ListRequests(c *gin.Context) {
	agentID := auth.GetAgentID(c)
	requests, err := h.friendshipService.ListReceivedRequests(c.Request.Context(), agentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": err.Error()})
		return
	}

	result := make([]gin.H, 0)
	for _, r := range requests {
		from := r.InitiatedBy
		agent, _ := h.agentService.GetByID(c.Request.Context(), from)
		name := from
		if agent != nil {
			name = agent.Name
		}
		result = append(result, gin.H{
			"from_agent": from,
			"name":       name,
			"message":    r.RequestMessage,
			"created_at": r.CreatedAt,
		})
	}

	c.JSON(http.StatusOK, gin.H{"ok": true, "data": result})
}

// SendRequest 发好友请求
func (h *FriendshipHandler) SendRequest(c *gin.Context) {
	agentID := auth.GetAgentID(c)
	var req struct {
		Target  string `json:"target" binding:"required"`
		Message string `json:"message"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": err.Error()})
		return
	}

	_, err := h.friendshipService.SendRequest(c.Request.Context(), agentID, req.Target, req.Message)
	if err != nil {
		switch err {
		case service.ErrAlreadyFriends:
			c.JSON(http.StatusConflict, gin.H{"ok": false, "error": "already friends", "code": "ALREADY_FRIENDS"})
		case service.ErrAgentNotFound:
			c.JSON(http.StatusNotFound, gin.H{"ok": false, "error": "agent not found", "code": "AGENT_NOT_FOUND"})
		case service.ErrBlocked:
			c.JSON(http.StatusForbidden, gin.H{"ok": false, "error": "blocked", "code": "BLOCKED"})
		case service.ErrPendingRequest:
			c.JSON(http.StatusConflict, gin.H{"ok": false, "error": "pending request exists", "code": "PENDING_EXISTS"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": err.Error()})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Accept 接受好友请求
func (h *FriendshipHandler) Accept(c *gin.Context) {
	agentID := auth.GetAgentID(c)
	target := c.Param("agent_id")

	err := h.friendshipService.Accept(c.Request.Context(), agentID, target)
	if err != nil {
		if err == service.ErrNoPendingRequest {
			c.JSON(http.StatusNotFound, gin.H{"ok": false, "error": "no pending request"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Reject 拒绝好友请求
func (h *FriendshipHandler) Reject(c *gin.Context) {
	agentID := auth.GetAgentID(c)
	target := c.Param("agent_id")

	err := h.friendshipService.Reject(c.Request.Context(), agentID, target)
	if err != nil {
		if err == service.ErrNoPendingRequest {
			c.JSON(http.StatusNotFound, gin.H{"ok": false, "error": "no pending request"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// SetNickname 设置备注
func (h *FriendshipHandler) SetNickname(c *gin.Context) {
	agentID := auth.GetAgentID(c)
	target := c.Param("agent_id")
	var req struct {
		Nickname string `json:"nickname"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": err.Error()})
		return
	}

	if err := h.friendshipService.SetNickname(c.Request.Context(), agentID, target, req.Nickname); err != nil {
		if err == service.ErrNotFriends {
			c.JSON(http.StatusNotFound, gin.H{"ok": false, "error": "not friends"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// SetGroup 设置分组
func (h *FriendshipHandler) SetGroup(c *gin.Context) {
	agentID := auth.GetAgentID(c)
	target := c.Param("agent_id")
	var req struct {
		Group string `json:"group"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": err.Error()})
		return
	}

	if err := h.friendshipService.SetGroup(c.Request.Context(), agentID, target, req.Group); err != nil {
		if err == service.ErrNotFriends {
			c.JSON(http.StatusNotFound, gin.H{"ok": false, "error": "not friends"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// SetTrust 设置信任等级
func (h *FriendshipHandler) SetTrust(c *gin.Context) {
	agentID := auth.GetAgentID(c)
	target := c.Param("agent_id")
	var req struct {
		Level string `json:"level" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": err.Error()})
		return
	}

	if err := h.friendshipService.SetTrust(c.Request.Context(), agentID, target, req.Level); err != nil {
		if err == service.ErrNotFriends {
			c.JSON(http.StatusNotFound, gin.H{"ok": false, "error": "not friends"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Remove 删除好友
func (h *FriendshipHandler) Remove(c *gin.Context) {
	agentID := auth.GetAgentID(c)
	target := c.Param("agent_id")

	err := h.friendshipService.Remove(c.Request.Context(), agentID, target)
	if err != nil {
		if err == service.ErrNotFriends {
			c.JSON(http.StatusNotFound, gin.H{"ok": false, "error": "not friends"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ViewProfile 查看名片
func (h *FriendshipHandler) ViewProfile(c *gin.Context) {
	agentID := auth.GetAgentID(c)
	target := c.Param("agent_id")

	agent, err := h.agentService.GetByID(c.Request.Context(), target)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": err.Error()})
		return
	}
	if agent == nil {
		c.JSON(http.StatusNotFound, gin.H{"ok": false, "error": "agent not found"})
		return
	}

	isFriend, _ := h.friendshipService.IsFriend(c.Request.Context(), agentID, target)

	public := agent.ToPublic()
	result := gin.H{
		"agent_id":     public.AgentID,
		"name":         public.Name,
		"avatar_url":   public.AvatarURL,
		"status_text":  public.StatusText,
		"online":       public.Online,
		"capabilities": public.Capabilities,
		"is_friend":    isFriend,
	}

	// 是好友的话返回更多信息
	if isFriend {
		friendship, _ := h.friendshipService.GetFriendship(c.Request.Context(), agentID, target)
		if friendship != nil {
			_, nickname, group, trustLevel, _ := friendship.GetFriendSide(agentID)
			result["nickname"] = nickname
			result["group"] = group
			result["trust_level"] = trustLevel
		}
	}

	c.JSON(http.StatusOK, gin.H{"ok": true, "data": result})
}

// Discover 搜索发现
func (h *FriendshipHandler) Discover(c *gin.Context) {
	agentID := auth.GetAgentID(c)
	keyword := c.Query("keyword")
	skill := c.Query("skill")
	tags := c.QueryArray("tags")
	onlineOnly := c.Query("online_only") == "true"
	limit := 20
	offset := 0
	if l := c.Query("limit"); l != "" {
		if n, err := atoi(l); err == nil {
			limit = n
		}
	}
	if o := c.Query("offset"); o != "" {
		if n, err := atoi(o); err == nil {
			offset = n
		}
	}

	agents, isFriendList, total, err := h.friendshipService.SearchWithFriendStatus(c.Request.Context(), agentID, keyword, skill, tags, onlineOnly, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": err.Error()})
		return
	}

	result := make([]gin.H, 0)
	for i, agent := range agents {
		if agent.AgentID == agentID {
			continue
		}
		caps, _ := agent.GetCapabilities()
		result = append(result, gin.H{
			"agent_id":     agent.AgentID,
			"name":         agent.Name,
			"avatar_url":   agent.AvatarURL,
			"status_text":  agent.StatusText,
			"online":       agent.Online,
			"is_friend":    isFriendList[i],
			"capabilities": caps,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"ok": true,
		"data": gin.H{
			"agents": result,
			"total":  total,
		},
	})
}

func atoi(s string) (int, error) {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, nil
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}
