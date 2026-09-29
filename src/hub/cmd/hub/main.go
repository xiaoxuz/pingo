package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pingo/hub/internal/auth"
	"github.com/pingo/hub/internal/cache"
	"github.com/pingo/hub/internal/config"
	"github.com/pingo/hub/internal/conn"
	"github.com/pingo/hub/internal/filestore"
	"github.com/pingo/hub/internal/handler"
	"github.com/pingo/hub/internal/service"
	"github.com/pingo/hub/internal/store"
)

type agentStoreAuthAdapter struct {
	store *store.AgentStore
}

func (a *agentStoreAuthAdapter) GetAgentIDByToken(token string) (string, error) {
	return a.store.GetAgentIDByToken(context.Background(), token)
}

func main() {
	configPath := "configs/hub.yaml"
	if len(os.Args) > 1 {
		configPath = os.Args[1]
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	log.Printf("Pingo starting...")
	log.Printf("HTTP port: %d", cfg.Server.HTTPPort)

	// 初始化数据库连接
	dbpool, err := pgxpool.New(context.Background(), cfg.Database.DSN())
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer dbpool.Close()

	if err := dbpool.Ping(context.Background()); err != nil {
		log.Fatalf("Failed to ping database: %v", err)
	}
	log.Println("Database connected")

	// 初始化 Redis（可选，连不上就降级走内存）
	var redisClient *cache.RedisClient
	if cfg.Redis.Addr != "" {
		rc, err := cache.NewRedisClient(cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.DB)
		if err != nil {
			log.Printf("WARN: Redis connection failed, falling back to memory: %v", err)
		} else {
			redisClient = rc
			log.Println("Redis connected")
			defer rc.Close()
		}
	} else {
		log.Println("Redis not configured, using memory-only mode")
	}

	// 初始化 token store
	tokenStore := auth.NewTokenStore()

	// 初始化 stores
	agentStore := store.NewAgentStore(dbpool)
	friendshipStore := store.NewFriendshipStore(dbpool)
	convStore := store.NewConversationStore(dbpool)
	msgStore := store.NewMessageStore(dbpool)
	offlineStore := store.NewOfflineStore(dbpool)

	// 初始化 services
	agentService := service.NewAgentService(agentStore, tokenStore)
	friendshipService := service.NewFriendshipService(friendshipStore, agentStore)
	messageService := service.NewMessageService(msgStore, convStore, friendshipStore, offlineStore)
	convService := service.NewConversationService(convStore, friendshipStore, msgStore)

	// 初始化文件存储
	fileStore, err := filestore.NewLocalFileStore(cfg.FileStorage.LocalPath, cfg.FileStorage.MaxFileSize)
	if err != nil {
		log.Fatalf("Failed to init file store: %v", err)
	}

	// 初始化连接管理器和路由器
	connManager := conn.NewManager(agentService, messageService, convService, redisClient)
	wsRouter := conn.NewRouter(connManager, agentService, friendshipService, convService, messageService)

	// 初始化 handlers
	wsHandler := handler.NewWsHandler(connManager, wsRouter, agentService)
	agentHandler := handler.NewAgentHandler(agentService)
	friendshipHandler := handler.NewFriendshipHandler(friendshipService, agentService)
	convHandler := handler.NewConversationHandler(convService, agentService)
	msgHandler := handler.NewMessageHandler(messageService, convService)
	fileHandler := handler.NewFileHandler(fileStore, 50*1024*1024).RequireMembership(msgStore)
	adminHandler := handler.NewAdminHandler(dbpool, fileStore).WithDisconnect(connManager.Disconnect)
	if err := adminHandler.Bootstrap(context.Background()); err != nil {
		log.Fatalf("Admin tables unavailable; apply migrations/003_admin.sql: %v", err)
	}

	// 设置 Gin
	r := gin.Default()

	// 请求 ID 中间件
	r.Use(func(c *gin.Context) {
		c.Set("start_time", time.Now())
		c.Next()
	})

	// CORS 中间件
	r.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	// WebSocket 端点（不需要 HTTP auth 中间件，用 WS 内的 auth.login）
	r.GET(cfg.Server.WSPath, wsHandler.HandleWebSocket)
	r.StaticFile("/", "website/index.html")
	r.StaticFile("/agent.md", "docs/agent/pingo.md")
	r.StaticFile("/latest.json", "latest.json")
	r.StaticFile("/pingo-sidecar-package.latest.tar.gz", "dist/pingo-sidecar-package.latest.tar.gz")
	r.Static("/docs", "docs")
	r.StaticFile("/admin", "src/admin-web/dist/index.html")
	r.Static("/admin/assets", "src/admin-web/dist/assets")
	r.GET("/admin/", func(c *gin.Context) { c.File("src/admin-web/dist/index.html") })

	// 公开 API
	api := r.Group("/api")
	{
		// Agent 注册（公开，管理用）
		api.POST("/agents/register", agentHandler.Register)

		// 需要认证的 API
		authGroup := api.Group("")
		authGroup.Use(auth.HTTPAuthMiddleware(&agentStoreAuthAdapter{store: agentStore}))
		{
			// Agent
			authGroup.GET("/agents/me", agentHandler.GetMe)
			authGroup.PUT("/agents/me/status", agentHandler.UpdateStatus)
			authGroup.PUT("/agents/me/capabilities", agentHandler.UpdateCapabilities)
			authGroup.PUT("/agents/me/owner", agentHandler.UpdateOwner)

			// 发现
			authGroup.GET("/discover", friendshipHandler.Discover)

			// 好友
			friends := authGroup.Group("/friends")
			{
				friends.GET("", friendshipHandler.ListFriends)
				friends.GET("/requests", friendshipHandler.ListRequests)
				friends.POST("/request", friendshipHandler.SendRequest)
				friends.POST("/requests/:agent_id/accept", friendshipHandler.Accept)
				friends.POST("/requests/:agent_id/reject", friendshipHandler.Reject)
				friends.PUT("/:agent_id/nickname", friendshipHandler.SetNickname)
				friends.PUT("/:agent_id/group", friendshipHandler.SetGroup)
				friends.PUT("/:agent_id/trust", friendshipHandler.SetTrust)
				friends.DELETE("/:agent_id", friendshipHandler.Remove)
				friends.GET("/:agent_id/profile", friendshipHandler.ViewProfile)
			}

			// 会话
			convs := authGroup.Group("/conversations")
			{
				convs.GET("", convHandler.List)
				convs.POST("/start_direct", convHandler.StartDirect)
				convs.POST("/create_group", convHandler.CreateGroup)
				convs.GET("/:id", convHandler.Get)
				convs.GET("/:id/members", convHandler.ListMembers)
				convs.GET("/:id/messages", msgHandler.List)
				convs.POST("/:id/send", msgHandler.Send)
				convs.POST("/:id/invite", convHandler.Invite)
				convs.POST("/:id/leave", convHandler.Leave)
				convs.POST("/:id/close", convHandler.Close)
				convs.POST("/:id/read", convHandler.MarkRead)
			}

			// 文件
			files := authGroup.Group("/files")
			{
				files.POST("/upload", fileHandler.Upload)
				files.GET("/:key", fileHandler.Download)
			}
		}

		// 管理 API
		admin := api.Group("/admin")
		admin.POST("/login", adminHandler.Login)
		admin.Use(handler.AdminSessionMiddleware(adminHandler))
		{
			admin.GET("/me", adminHandler.Me)
			admin.POST("/logout", adminHandler.Logout)
			admin.POST("/change-password", adminHandler.ChangePassword)
			admin.GET("/stats", adminHandler.Overview)
			admin.GET("/agents", adminHandler.Agents)
			admin.DELETE("/agents/:id", adminHandler.DeleteAgent)
			admin.GET("/conversations", adminHandler.Conversations)
			admin.GET("/conversations/:id/messages", adminHandler.Messages)
			admin.DELETE("/conversations/:id", adminHandler.DeleteConversation)
			admin.GET("/relations", adminHandler.Relations)
			admin.DELETE("/relations/:a/:b", adminHandler.DeleteRelation)
			admin.GET("/queue", adminHandler.Queue)
			admin.GET("/files", adminHandler.Files)
			admin.GET("/files/:id/download", adminHandler.DownloadFile)
			admin.GET("/audit", adminHandler.Audit)
		}
	}

	// 启动 HTTP 服务器
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Server.HTTPPort),
		Handler: r,
	}

	// 优雅关闭
	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit
		log.Println("Shutting down server...")

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			log.Fatalf("Server forced to shutdown: %v", err)
		}
		log.Println("Server exited")
	}()

	log.Printf("Pingo listening on :%d", cfg.Server.HTTPPort)
	log.Printf("WebSocket endpoint: %s", cfg.Server.WSPath)

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Failed to start server: %v", err)
	}
}
