package local

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/pingo/sidecar/internal/approval"
	"github.com/pingo/sidecar/internal/config"
	"github.com/pingo/sidecar/internal/hub"
	"github.com/pingo/sidecar/internal/notify"
	"github.com/pingo/sidecar/internal/operationlog"
	"github.com/pingo/sidecar/internal/session"
	"github.com/pingo/sidecar/internal/store"
	"github.com/pingo/sidecar/internal/update"
)

type APIServer struct {
	config       *config.Config
	pool         *hub.Pool
	store        *store.SQLiteStore
	approvalMgr  *approval.Manager
	notifier     *notify.Manager
	sessions     *session.Registry
	updates      *update.Checker
	server       *http.Server
	agentNameMap map[string]string
}

func NewAPIServer(
	cfg *config.Config,
	pool *hub.Pool,
	store *store.SQLiteStore,
	approvalMgr *approval.Manager,
	notifier *notify.Manager,
	sessions *session.Registry,
	updates *update.Checker,
) *APIServer {
	agentNameMap := make(map[string]string)
	for _, a := range cfg.Agents {
		agentNameMap[a.ID] = a.Name
	}

	return &APIServer{
		config:       cfg,
		pool:         pool,
		store:        store,
		approvalMgr:  approvalMgr,
		notifier:     notifier,
		sessions:     sessions,
		updates:      updates,
		agentNameMap: agentNameMap,
	}
}

func (s *APIServer) Start() error {
	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()
	r.Use(operationlog.New(log.Writer()).Middleware())

	// CORS（本地用，宽松点）
	r.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Agent-ID")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	// 公共接口
	r.GET("/status", s.handleStatus)
	r.GET("/agents", s.handleListAgents)
	r.GET("/update/status", s.handleReleaseStatus)
	r.POST("/update/check", s.handleReleaseCheck)
	r.POST("/agents/register", s.handleRegisterAgent)
	r.POST("/pingo/sessions", s.handlePingoRegisterSession)
	r.GET("/pingo/sessions/:id/events", s.handlePingoSessionEvents)
	r.GET("/pingo/sessions/:id/pending", s.handlePingoPending)
	r.PUT("/pingo/sessions/:id/status", s.handlePingoSessionStatus)
	r.GET("/pingo/sessions", s.handlePingoSessions)
	r.DELETE("/pingo/sessions/:id", s.handlePingoRemoveSession)
	r.GET("/approvals/pending", s.handlePendingApprovals)
	r.POST("/approvals/:id/decide", s.handleDecideApproval)

	// 需要 Agent 身份的接口
	api := r.Group("")
	api.Use(s.agentAuthMiddleware())
	{
		// 收件箱
		api.GET("/inbox", s.handleInbox)

		// 会话
		api.GET("/conversations", s.handleListConversations)
		api.GET("/conversations/:id", s.handleGetConversation)
		api.GET("/conversations/:id/messages", s.handleListMessages)
		api.POST("/conversations/start_direct", s.handleStartDirect)
		api.POST("/conversations/create_group", s.handleCreateGroup)
		api.POST("/conversations/:id/send", s.handleSendMessage)
		api.POST("/conversations/:id/invite", s.handleInvite)
		api.POST("/conversations/:id/leave", s.handleLeave)
		api.POST("/conversations/:id/close", s.handleClose)
		api.POST("/conversations/:id/read", s.handleMarkRead)

		// 好友
		api.GET("/friends", s.handleListFriends)
		api.GET("/friends/requests", s.handleFriendRequests)
		api.POST("/friends/request", s.handleSendFriendRequest)
		api.POST("/friends/requests/:agent_id/accept", s.handleAcceptFriend)
		api.POST("/friends/requests/:agent_id/reject", s.handleRejectFriend)
		api.PUT("/friends/:agent_id/nickname", s.handleSetNickname)
		api.PUT("/friends/:agent_id/group", s.handleSetGroup)
		api.PUT("/friends/:agent_id/trust", s.handleSetTrust)
		api.POST("/friends/:agent_id/block", s.handleBlockFriend)
		api.DELETE("/friends/:agent_id", s.handleRemoveFriend)
		api.GET("/friends/:agent_id/profile", s.handleFriendProfile)

		// 发现
		api.POST("/discover", s.handleDiscover)

		// 文件
		files := api.Group("/files")
		{
			files.POST("/upload", s.handleFileUpload)
			files.GET("/:key", s.handleFileDownload)
		}

		// 自身
		api.GET("/me", s.handleGetMe)
		api.PUT("/me/status", s.handleUpdateStatus)
		api.PUT("/me/capabilities", s.handleUpdateCapabilities)
		api.PUT("/me/profile", s.handleUpdateProfile)
		api.PUT("/me/availability", s.handleUpdateAvailability)
		api.PUT("/me/owner", s.handleUpdateOwner)

		// Agent 自主脉冲与私有待办
		api.GET("/todos", s.handleListTodos)
		api.POST("/todos", s.handleCreateTodo)
		api.POST("/todos/:id/complete", s.handleCompleteTodo)
		api.POST("/todos/:id/snooze", s.handleSnoozeTodo)
		api.DELETE("/todos/:id", s.handleCancelTodo)
		api.GET("/pulse/context", s.handlePulseContext)
		api.POST("/pulse/complete", s.handleCompletePulse)
		api.GET("/pulse/settings", s.handleGetPulseSettings)
		api.PUT("/pulse/settings", s.handleUpdatePulseSettings)
		api.GET("/pulse/history", s.handlePulseHistory)

		// 人工决策
		api.POST("/approvals/request", s.handleRequestApproval)
		api.GET("/approvals/:id", s.handleGetApproval)
	}

	s.server = &http.Server{
		Addr:    fmt.Sprintf("127.0.0.1:%d", s.config.Local.APIPort),
		Handler: r,
	}

	go func() {
		log.Printf("INFO: Local API server listening on 127.0.0.1:%d", s.config.Local.APIPort)
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("ERROR: Local API server error: %v", err)
		}
	}()

	return nil
}

type pingoRegisterRequest struct {
	SessionID string `json:"session_id"`
	AgentID   string `json:"agent_id"`
	Provider  string `json:"provider"`
	CWD       string `json:"cwd"`
}

func (s *APIServer) handlePingoRegisterSession(c *gin.Context) {
	var request pingoRegisterRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "invalid JSON body"})
		return
	}
	if request.SessionID == "" || request.AgentID == "" || request.Provider == "" {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "session_id, agent_id and provider are required"})
		return
	}
	if request.Provider != "claude" && request.Provider != "codex" {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "provider must be claude or codex"})
		return
	}
	if !s.pool.HasAgent(request.AgentID) {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "agent is not managed by this sidecar"})
		return
	}

	registered := s.sessions.Register(session.RegisterRequest{
		ID:       request.SessionID,
		AgentID:  request.AgentID,
		Provider: request.Provider,
		CWD:      request.CWD,
	})
	if s.updates != nil {
		state := s.updates.State()
		if state.Available && registered.Primary {
			s.sessions.Publish(request.AgentID, session.Event{Type: "update.available", Version: state.LatestVersion})
		}
	}
	c.JSON(http.StatusCreated, gin.H{"ok": true, "data": registered})
}

func (s *APIServer) handleReleaseStatus(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": s.updates.State()})
}

func (s *APIServer) handleReleaseCheck(c *gin.Context) {
	if err := s.updates.Check(c.Request.Context()); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"ok": false, "error": "update check failed", "data": s.updates.State()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": s.updates.State()})
}

func (s *APIServer) handlePingoSessionEvents(c *gin.Context) {
	agentID := c.Query("agent_id")
	if agentID == "" || !s.pool.HasAgent(agentID) {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "agent is not managed by this sidecar"})
		return
	}
	events := s.sessions.Subscribe(c.Param("id"))
	if events == nil {
		c.JSON(http.StatusNotFound, gin.H{"ok": false, "error": "pingo session not found"})
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Stream(func(writer io.Writer) bool {
		select {
		case event := <-events:
			c.SSEvent(event.Type, event)
			return true
		case <-c.Request.Context().Done():
			return false
		case <-time.After(20 * time.Second):
			c.SSEvent("ping", gin.H{"ok": true})
			return true
		}
	})
}

func (s *APIServer) handlePingoRemoveSession(c *gin.Context) {
	agentID := strings.TrimSpace(c.Query("agent_id"))
	if agentID == "" || !s.sessions.Remove(c.Param("id"), agentID) {
		c.JSON(http.StatusNotFound, gin.H{"ok": false, "error": "pingo session not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *APIServer) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.server.Shutdown(ctx)
}

// agentAuthMiddleware 验证 X-Agent-ID Header
func (s *APIServer) agentAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		agentID := c.GetHeader("X-Agent-ID")
		if agentID == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"ok":    false,
				"error": "missing X-Agent-ID header",
			})
			c.Abort()
			return
		}

		if !s.pool.HasAgent(agentID) {
			c.JSON(http.StatusBadRequest, gin.H{
				"ok":    false,
				"error": fmt.Sprintf("agent %s not managed by this sidecar", agentID),
			})
			c.Abort()
			return
		}

		c.Set("agent_id", agentID)
		c.Next()
	}
}

func getAgentID(c *gin.Context) string {
	v, _ := c.Get("agent_id")
	id, _ := v.(string)
	return id
}

func okResponse(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": data})
}

func errResponse(c *gin.Context, code int, message string) {
	c.JSON(code, gin.H{"ok": false, "error": message})
}
