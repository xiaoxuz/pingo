package hub

import (
	"log"
	"sync"
	"time"

	"github.com/pingo/sidecar/internal/config"
)

// Pool 管理多个 Agent 的 Hub 连接
type Pool struct {
	config    *config.Config
	clients   map[string]*HubClient
	mu        sync.RWMutex
	onMessage func(agentID string, env MessageEnvelope)
}

func NewPool(cfg *config.Config) *Pool {
	return &Pool{
		config:  cfg,
		clients: make(map[string]*HubClient),
	}
}

func (p *Pool) SetOnMessage(fn func(agentID string, env MessageEnvelope)) {
	p.onMessage = fn
}

// Start 启动所有 Agent 的连接
func (p *Pool) Start() {
	for _, agentCfg := range p.config.Agents {
		p.addClient(agentCfg)
	}
}

// Stop 停止所有连接
func (p *Pool) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, client := range p.clients {
		client.Close()
	}
	p.clients = make(map[string]*HubClient)
}

// GetClient 获取某个 Agent 的客户端
func (p *Pool) GetClient(agentID string) (*HubClient, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	client, exists := p.clients[agentID]
	return client, exists
}

// HasAgent 检查是否管理某个 Agent
func (p *Pool) HasAgent(agentID string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	_, exists := p.clients[agentID]
	return exists
}

// ListAgents 列出所有 Agent
func (p *Pool) ListAgents() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	ids := make([]string, 0, len(p.clients))
	for id := range p.clients {
		ids = append(ids, id)
	}
	return ids
}

// AgentStatus 获取 Agent 连接状态
func (p *Pool) AgentStatus(agentID string) (connected bool, exists bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	client, ok := p.clients[agentID]
	if !ok {
		return false, false
	}
	return client.IsConnected(), true
}

// AddAgent 动态添加 Agent
func (p *Pool) AddAgent(cfg config.AgentConfig) {
	p.mu.Lock()
	if _, exists := p.clients[cfg.ID]; exists {
		p.mu.Unlock()
		return
	}
	p.mu.Unlock()
	p.addClient(cfg)
}

// RemoveAgent 移除 Agent
func (p *Pool) RemoveAgent(agentID string) {
	p.mu.Lock()
	client, exists := p.clients[agentID]
	if exists {
		delete(p.clients, agentID)
	}
	p.mu.Unlock()
	if exists {
		client.Close()
	}
}

func (p *Pool) addClient(agentCfg config.AgentConfig) {
	client := NewHubClient(agentCfg.ID, agentCfg.Token, p.config.Hub.Endpoint)

	client.SetOnMessage(func(env MessageEnvelope) {
		if p.onMessage != nil {
			p.onMessage(agentCfg.ID, env)
		}
	})

	client.SetOnConnected(func() {
		log.Printf("INFO: agent %s connected to hub", agentCfg.ID)
		// 连接成功后同步数据
		go p.syncInitialData(agentCfg.ID, client)
	})

	client.SetOnDisconnected(func() {
		log.Printf("INFO: agent %s disconnected from hub", agentCfg.ID)
		// 启动重连
		go p.reconnect(agentCfg, client)
	})

	p.mu.Lock()
	p.clients[agentCfg.ID] = client
	p.mu.Unlock()

	// 启动连接
	go func() {
		backoff := 1 * time.Second
		maxBackoff := 30 * time.Second
		for {
			err := client.Connect()
			if err == nil {
				return
			}
			log.Printf("WARN: failed to connect agent %s: %v, retrying in %v", agentCfg.ID, err, backoff)
			time.Sleep(backoff)
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}

			// 检查是否已被移除
			p.mu.RLock()
			_, exists := p.clients[agentCfg.ID]
			p.mu.RUnlock()
			if !exists {
				return
			}
		}
	}()
}

func (p *Pool) reconnect(agentCfg config.AgentConfig, oldClient *HubClient) {
	backoff := 1 * time.Second
	maxBackoff := 30 * time.Second

	for {
		// 检查是否还在池子里
		p.mu.RLock()
		client, exists := p.clients[agentCfg.ID]
		p.mu.RUnlock()
		if !exists || client != oldClient {
			return
		}

		log.Printf("INFO: reconnecting agent %s in %v...", agentCfg.ID, backoff)
		time.Sleep(backoff)

		newClient := NewHubClient(agentCfg.ID, agentCfg.Token, p.config.Hub.Endpoint)
		newClient.SetOnMessage(func(env MessageEnvelope) {
			if p.onMessage != nil {
				p.onMessage(agentCfg.ID, env)
			}
		})
		newClient.SetOnConnected(func() {
			log.Printf("INFO: agent %s reconnected", agentCfg.ID)
			go p.syncInitialData(agentCfg.ID, newClient)
		})
		newClient.SetOnDisconnected(func() {
			log.Printf("INFO: agent %s disconnected again", agentCfg.ID)
			go p.reconnect(agentCfg, newClient)
		})

		err := newClient.Connect()
		if err == nil {
			p.mu.Lock()
			if p.clients[agentCfg.ID] == oldClient {
				p.clients[agentCfg.ID] = newClient
			}
			p.mu.Unlock()
			oldClient.Close()
			return
		}

		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

func (p *Pool) syncInitialData(agentID string, client *HubClient) {
	// 同步好友列表
	client.Send("sync.friends", nil)
	// 同步好友请求
	client.Send("sync.friend_requests", nil)
	// 同步会话列表
	client.Send("sync.conversations", nil)
}
