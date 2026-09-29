package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pingo/sidecar/internal/approval"
	"github.com/pingo/sidecar/internal/config"
	"github.com/pingo/sidecar/internal/dashboard"
	"github.com/pingo/sidecar/internal/hub"
	"github.com/pingo/sidecar/internal/local"
	"github.com/pingo/sidecar/internal/notify"
	"github.com/pingo/sidecar/internal/operationlog"
	"github.com/pingo/sidecar/internal/pulse"
	"github.com/pingo/sidecar/internal/session"
	"github.com/pingo/sidecar/internal/store"
	"github.com/pingo/sidecar/internal/update"
)

func main() {
	configPath := ""
	if len(os.Args) > 1 {
		if os.Args[1] == "--print-hub-endpoint" {
			fmt.Println(config.DefaultHubEndpoint())
			return
		}
		configPath = os.Args[1]
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	log.Println("Pingo Sidecar starting...")
	operations := operationlog.New(log.Writer())
	operations.Event("lifecycle", "start", "managed_agents", len(cfg.Agents))
	log.Printf("Managed agents: %d", len(cfg.Agents))

	// 初始化本地存储
	db, err := store.NewSQLiteStore(cfg.Local.DBPath)
	if err != nil {
		log.Fatalf("Failed to init SQLite: %v", err)
	}
	defer db.Close()
	log.Println("Local store initialized")

	// 初始化通知管理器
	notifier := notify.NewManager()
	if cfg.Notify.Terminal {
		notifier.Add(notify.NewTerminalNotifier(true))
		log.Println("Terminal notifier enabled")
	}
	if cfg.Notify.Desktop {
		notifier.Add(notify.NewDesktopNotifier(true))
		log.Println("Desktop notifier enabled")
	}
	if cfg.Notify.Webhook.Enabled {
		notifier.Add(notify.NewWebhookNotifier(true, cfg.Notify.Webhook.URL))
		log.Println("Webhook notifier enabled")
	}

	// 初始化审批管理器
	approvalMgr := approval.NewManager(db, &cfg.Approval, notifier)
	approvalMgr.Start()
	log.Println("Approval manager started")

	// 构建 agent name 映射
	agentNames := make(map[string]string)
	for _, a := range cfg.Agents {
		agentNames[a.ID] = a.Name
	}

	// 初始化消息接收器
	sessions := session.NewRegistry()
	pulseManager := pulse.NewManager(db, sessions)
	pulseManager.Start()
	defer pulseManager.Stop()
	log.Println("Agent pulse manager started")
	updates := update.NewChecker(config.Version, config.Channel, config.UpdateManifestURL, func(state update.State) {
		operations.Event("update", "available", "version", state.LatestVersion)
		log.Printf("INFO: [update] new version available: %s", state.LatestVersion)
		sessions.PublishAll(session.Event{Type: "update.available", Version: state.LatestVersion})
	})
	interval, err := time.ParseDuration(config.UpdateCheckInterval)
	if err != nil || interval <= 0 {
		log.Fatalf("Invalid update check interval: %s", config.UpdateCheckInterval)
	}
	updateContext, stopUpdates := context.WithCancel(context.Background())
	defer stopUpdates()
	go func() {
		check := func() {
			if err := updates.Check(updateContext); err != nil {
				operations.Event("update", "check", "result", "failed")
				log.Printf("WARN: [update] check failed: %v", err)
			} else {
				operations.Event("update", "check", "result", "ok", "available", updates.State().Available)
			}
		}
		check()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-updateContext.Done():
				return
			case <-ticker.C:
				check()
			}
		}
	}()
	receiver := hub.NewMessageReceiver(db, notifier, approvalMgr, sessions, agentNames)

	// 初始化 Hub 连接池
	pool := hub.NewPool(cfg)
	pool.SetOnMessage(func(agentID string, env hub.MessageEnvelope) {
		receiver.HandleMessage(agentID, env)
	})
	pool.Start()
	log.Println("Hub connection pool started")

	// 启动本地 API 服务器
	apiServer := local.NewAPIServer(cfg, pool, db, approvalMgr, notifier, sessions, updates)
	if err := apiServer.Start(); err != nil {
		log.Fatalf("Failed to start API server: %v", err)
	}

	// 启动 Dashboard
	dashServer := dashboard.NewServer(cfg.Local.DashboardPort, "http://127.0.0.1:"+itoa(cfg.Local.APIPort))
	if err := dashServer.Start(); err != nil {
		log.Printf("WARN: Failed to start dashboard: %v", err)
	} else {
		log.Printf("Dashboard available at http://127.0.0.1:%d", cfg.Local.DashboardPort)
	}

	log.Println("Pingo Sidecar started successfully")

	// 等待退出信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down...")
	dashServer.Stop()
	apiServer.Stop()
	approvalMgr.Stop()
	pool.Stop()
	log.Println("Pingo Sidecar stopped")
	operations.Event("lifecycle", "stop")
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var result []byte
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	for n > 0 {
		result = append([]byte{byte('0' + n%10)}, result...)
		n /= 10
	}
	if neg {
		result = append([]byte{'-'}, result...)
	}
	return string(result)
}
