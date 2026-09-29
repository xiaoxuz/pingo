package conn

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/pingo/hub/internal/cache"
	"github.com/pingo/hub/internal/service"
	"github.com/pingo/hub/pkg/protocol"
)

// Manager 管理所有 WebSocket 连接
type Manager struct {
	mu           sync.RWMutex
	connections  map[string]*Connection // agent_id -> Connection
	agentService *service.AgentService
	msgService   *service.MessageService
	convService  *service.ConversationService
	redisClient  *cache.RedisClient
}

func NewManager(
	agentService *service.AgentService,
	msgService *service.MessageService,
	convService *service.ConversationService,
	redisClient *cache.RedisClient,
) *Manager {
	return &Manager{
		connections:  make(map[string]*Connection),
		agentService: agentService,
		msgService:   msgService,
		convService:  convService,
		redisClient:  redisClient,
	}
}

// Register 注册一个新连接
func (m *Manager) Register(agentID string, conn *Connection) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 如果已有连接，踢掉旧的
	if oldConn, exists := m.connections[agentID]; exists {
		log.Printf("INFO: kicking old connection for agent %s", agentID)
		oldConn.Send(protocol.NewError("error", "", agentID, protocol.ErrAgentAlreadyConnected, "该 Agent 已在其他地方登录，此连接被踢出"))
		oldConn.Close()
	}

	m.connections[agentID] = conn
	log.Printf("INFO: agent %s connected, total connections: %d", agentID, len(m.connections))

	// 标记在线（DB + Redis）
	go m.setOnline(agentID, true)

	// 投递离线消息
	go m.deliverOfflineMessages(agentID, conn)
}

// Unregister 注销连接
func (m *Manager) Unregister(agentID string) {
	m.mu.Lock()
	conn, exists := m.connections[agentID]
	if exists {
		delete(m.connections, agentID)
		conn.Close()
	}
	count := len(m.connections)
	m.mu.Unlock()

	if exists {
		log.Printf("INFO: agent %s disconnected, total connections: %d", agentID, count)
		// 延迟标记离线（防网络抖动）
		go func() {
			time.Sleep(30 * time.Second)
			// 检查是否重连了
			m.mu.RLock()
			_, reconnected := m.connections[agentID]
			m.mu.RUnlock()
			if !reconnected {
				m.setOnline(agentID, false)
				log.Printf("INFO: agent %s marked offline", agentID)
			}
		}()
	}
}

// Disconnect 立即断开指定 Agent，供管理员删除身份时使用。
func (m *Manager) Disconnect(agentID string) {
	m.mu.Lock()
	conn, exists := m.connections[agentID]
	if exists {
		delete(m.connections, agentID)
		conn.Close()
	}
	m.mu.Unlock()
}

// setOnline 同时更新 DB 和 Redis 中的在线状态
func (m *Manager) setOnline(agentID string, online bool) {
	ctx := context.Background()
	m.agentService.SetOnline(ctx, agentID, online)

	if m.redisClient != nil {
		if err := m.redisClient.SetOnline(ctx, agentID, online); err != nil {
			log.Printf("WARN: failed to set online status in redis for %s: %v", agentID, err)
		}
	}
}

// RefreshHeartbeat 刷新心跳（更新 Redis TTL）
func (m *Manager) RefreshHeartbeat(agentID string) {
	if m.redisClient != nil {
		if err := m.redisClient.RefreshOnline(context.Background(), agentID); err != nil {
			log.Printf("WARN: failed to refresh heartbeat in redis for %s: %v", agentID, err)
		}
	}
}

// GetConnection 获取连接
func (m *Manager) GetConnection(agentID string) (*Connection, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	conn, exists := m.connections[agentID]
	return conn, exists
}

// IsOnline 检查是否在线（先查内存，再查 Redis）
func (m *Manager) IsOnline(agentID string) bool {
	m.mu.RLock()
	_, exists := m.connections[agentID]
	m.mu.RUnlock()
	if exists {
		return true
	}

	// 内存里没有，查 Redis（可能在其他 Hub 节点上）
	if m.redisClient != nil {
		online, err := m.redisClient.IsOnline(context.Background(), agentID)
		if err == nil {
			return online
		}
		log.Printf("WARN: redis IsOnline failed for %s: %v", agentID, err)
	}
	return false
}

// BatchIsOnline 批量检查在线状态
func (m *Manager) BatchIsOnline(agentIDs []string) map[string]bool {
	result := make(map[string]bool, len(agentIDs))

	// 先查内存
	m.mu.RLock()
	for _, id := range agentIDs {
		_, exists := m.connections[id]
		result[id] = exists
	}
	m.mu.RUnlock()

	// 内存里不在线的，再查 Redis
	if m.redisClient != nil {
		var offlineIDs []string
		for _, id := range agentIDs {
			if !result[id] {
				offlineIDs = append(offlineIDs, id)
			}
		}
		if len(offlineIDs) > 0 {
			redisOnline, err := m.redisClient.BatchIsOnline(context.Background(), offlineIDs)
			if err == nil {
				for k, v := range redisOnline {
					result[k] = v
				}
			} else {
				log.Printf("WARN: redis BatchIsOnline failed: %v", err)
			}
		}
	}

	return result
}

// BroadcastToConversation 向会话所有成员广播
func (m *Manager) BroadcastToConversation(ctx context.Context, conversationID string, env protocol.Envelope, excludeAgentID string) {
	members, err := m.convService.ListActiveMembers(ctx, conversationID)
	if err != nil {
		log.Printf("ERROR: list active members: %v", err)
		return
	}

	for _, member := range members {
		if member.AgentID == excludeAgentID {
			continue
		}
		m.SendToAgent(member.AgentID, env)
	}
}

// SendToAgent 向某个 agent 发送消息（只发到本节点的连接）
func (m *Manager) SendToAgent(agentID string, env protocol.Envelope) bool {
	m.mu.RLock()
	conn, exists := m.connections[agentID]
	m.mu.RUnlock()

	if !exists {
		return false
	}
	return conn.Send(env)
}

// OnlineCount 获取本节点在线数量
func (m *Manager) OnlineCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.connections)
}

// GetAllConnections 获取所有连接（用于管理）
func (m *Manager) GetAllConnections() map[string]*Connection {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make(map[string]*Connection, len(m.connections))
	for k, v := range m.connections {
		result[k] = v
	}
	return result
}

// deliverOfflineMessages 投递离线消息
func (m *Manager) deliverOfflineMessages(agentID string, conn *Connection) {
	items, err := m.msgService.GetOfflinePending(context.Background(), agentID)
	if err != nil {
		log.Printf("ERROR: get offline messages for %s: %v", agentID, err)
		return
	}

	if len(items) == 0 {
		return
	}

	log.Printf("INFO: delivering %d offline messages for agent %s", len(items), agentID)

	for _, item := range items {
		msg, err := m.msgService.GetByID(context.Background(), item.MessageID)
		if err != nil || msg == nil {
			continue
		}

		unreadCount, _ := m.convService.GetUnreadCount(context.Background(), item.ConversationID, agentID)

		notify := protocol.MsgNewNotify{
			ConversationID: item.ConversationID,
			Message: protocol.MessageInfo{
				MessageID:      msg.MessageID,
				ConversationID: msg.ConversationID,
				FromAgent:      msg.FromAgent,
				MessageType:    msg.MessageType,
				ContentText:    msg.ContentText,
				Mentions:       msg.Mentions,
				ReplyTo:        msg.ReplyTo,
				SystemEvent:    msg.SystemEvent,
				CreatedAt:      msg.CreatedAt.Format(time.RFC3339),
			},
			UnreadCount: unreadCount,
		}

		conn.Send(protocol.NewNotify(protocol.TypeMsgNewNotify, agentID, notify))
		m.msgService.MarkOfflineDelivered(context.Background(), item.ID)
	}
}
