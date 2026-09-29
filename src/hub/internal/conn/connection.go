package conn

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/pingo/hub/pkg/protocol"
	"github.com/gorilla/websocket"
)

// Connection 封装一个 WebSocket 连接
type Connection struct {
	agentID string
	conn    *websocket.Conn
	send    chan protocol.Envelope
	close   chan struct{}
	once    sync.Once
	mu      sync.Mutex
}

func NewConnection(agentID string, ws *websocket.Conn) *Connection {
	c := &Connection{
		agentID: agentID,
		conn:    ws,
		send:    make(chan protocol.Envelope, 256),
		close:   make(chan struct{}),
	}
	return c
}

func (c *Connection) AgentID() string {
	return c.agentID
}

// Send 发送消息到客户端（非阻塞）
func (c *Connection) Send(env protocol.Envelope) bool {
	select {
	case c.send <- env:
		return true
	case <-c.close:
		return false
	default:
		// 发送队列满了，丢弃
		log.Printf("WARN: send queue full for agent %s, dropping message", c.agentID)
		return false
	}
}

// Close 关闭连接
func (c *Connection) Close() {
	c.once.Do(func() {
		close(c.close)
		c.conn.Close()
	})
}

// IsClosed 检查是否已关闭
func (c *Connection) IsClosed() bool {
	select {
	case <-c.close:
		return true
	default:
		return false
	}
}

// writePump 写协程
func (c *Connection) WritePump() {
	defer c.Close()

	for {
		select {
		case env, ok := <-c.send:
			if !ok {
				return
			}
			c.mu.Lock()
			err := c.conn.WriteJSON(env)
			c.mu.Unlock()
			if err != nil {
				log.Printf("WARN: write error for agent %s: %v", c.agentID, err)
				return
			}
		case <-c.close:
			return
		}
	}
}

// ReadMessage 读取一条消息
func (c *Connection) ReadMessage() (protocol.Envelope, error) {
	var env protocol.Envelope
	// 设置读超时
	c.conn.SetReadDeadline(time.Now().Add(120 * time.Second))
	_, msgData, err := c.conn.ReadMessage()
	if err != nil {
		return env, err
	}
	err = json.Unmarshal(msgData, &env)
	if err != nil {
		return env, err
	}
	return env, nil
}
