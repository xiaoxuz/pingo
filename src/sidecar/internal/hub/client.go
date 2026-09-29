package hub

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pingo/sidecar/internal/operationlog"
)

type HubClient struct {
	agentID   string
	token     string
	endpoint  string
	conn      *websocket.Conn
	send      chan MessageEnvelope
	close     chan struct{}
	once      sync.Once
	mu        sync.RWMutex
	connected bool

	onMessage      func(env MessageEnvelope)
	onConnected    func()
	onDisconnected func()

	pendingRequests map[string]chan MessageEnvelope
	pendingMu       sync.Mutex
}

type MessageEnvelope struct {
	Type      string      `json:"type"`
	AgentID   string      `json:"agent_id,omitempty"`
	RequestID string      `json:"request_id,omitempty"`
	Data      interface{} `json:"data,omitempty"`
	Error     *ErrorInfo  `json:"error,omitempty"`
}

type ErrorInfo struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func NewHubClient(agentID string, token string, endpoint string) *HubClient {
	return &HubClient{
		agentID:         agentID,
		token:           token,
		endpoint:        endpoint,
		send:            make(chan MessageEnvelope, 256),
		close:           make(chan struct{}),
		pendingRequests: make(map[string]chan MessageEnvelope),
	}
}

func (c *HubClient) AgentID() string {
	return c.agentID
}

func (c *HubClient) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

func (c *HubClient) SetOnMessage(fn func(env MessageEnvelope)) {
	c.onMessage = fn
}

func (c *HubClient) SetOnConnected(fn func()) {
	c.onConnected = fn
}

func (c *HubClient) SetOnDisconnected(fn func()) {
	c.onDisconnected = fn
}

func (c *HubClient) Connect() error {
	c.mu.Lock()
	if c.connected {
		c.mu.Unlock()
		return nil
	}
	c.mu.Unlock()

	conn, _, err := websocket.DefaultDialer.Dial(c.endpoint, nil)
	if err != nil {
		return err
	}

	c.mu.Lock()
	c.conn = conn
	c.connected = true
	c.mu.Unlock()

	loginEnv := MessageEnvelope{
		Type:    "auth.login",
		AgentID: c.agentID,
		Data: map[string]interface{}{
			"agent_id": c.agentID,
			"token":    c.token,
		},
	}
	conn.WriteJSON(loginEnv)

	go c.writePump()
	go c.readPump()

	return nil
}

func (c *HubClient) Close() {
	c.once.Do(func() {
		close(c.close)
		c.mu.Lock()
		if c.conn != nil {
			c.conn.Close()
		}
		c.connected = false
		c.mu.Unlock()
	})
}

func (c *HubClient) Send(msgType string, data interface{}) {
	operationlog.New(log.Writer()).Event("hub", "queue", "agent_id", c.agentID, "event_type", msgType)
	env := MessageEnvelope{
		Type:    msgType,
		AgentID: c.agentID,
		Data:    data,
	}
	select {
	case c.send <- env:
	case <-c.close:
	}
}

func (c *HubClient) Request(msgType string, data interface{}, timeout time.Duration) (*MessageEnvelope, error) {
	requestID := generateRequestID()
	respCh := make(chan MessageEnvelope, 1)

	c.pendingMu.Lock()
	c.pendingRequests[requestID] = respCh
	c.pendingMu.Unlock()

	defer func() {
		c.pendingMu.Lock()
		delete(c.pendingRequests, requestID)
		c.pendingMu.Unlock()
	}()

	env := MessageEnvelope{
		Type:      msgType,
		AgentID:   c.agentID,
		RequestID: requestID,
		Data:      data,
	}

	select {
	case c.send <- env:
	case <-c.close:
		return nil, ErrDisconnected
	}

	select {
	case resp := <-respCh:
		return &resp, nil
	case <-time.After(timeout):
		return nil, ErrTimeout
	case <-c.close:
		return nil, ErrDisconnected
	}
}

func (c *HubClient) writePump() {
	defer c.Close()

	for {
		select {
		case env, ok := <-c.send:
			if !ok {
				return
			}
			c.mu.RLock()
			conn := c.conn
			c.mu.RUnlock()
			if conn == nil {
				return
			}
			err := conn.WriteJSON(env)
			if err != nil {
				log.Printf("WARN: write error for agent %s: %v", c.agentID, err)
				return
			}
			operationlog.New(log.Writer()).Event("hub", "send", "agent_id", c.agentID, "event_type", env.Type)
		case <-c.close:
			return
		}
	}
}

func (c *HubClient) readPump() {
	defer func() {
		c.mu.Lock()
		c.connected = false
		c.mu.Unlock()
		if c.onDisconnected != nil {
			c.onDisconnected()
		}
		c.Close()
	}()

	if c.onConnected != nil {
		c.onConnected()
	}

	for {
		c.mu.RLock()
		conn := c.conn
		c.mu.RUnlock()
		if conn == nil {
			return
		}

		conn.SetReadDeadline(time.Now().Add(120 * time.Second))
		_, msgData, err := conn.ReadMessage()
		if err != nil {
			log.Printf("WARN: read error for agent %s: %v", c.agentID, err)
			return
		}

		var env MessageEnvelope
		if err := json.Unmarshal(msgData, &env); err != nil {
			log.Printf("WARN: parse message error for agent %s: %v", c.agentID, err)
			continue
		}
		operationlog.New(log.Writer()).Event("hub", "receive", "agent_id", c.agentID, "event_type", env.Type)

		if env.RequestID != "" {
			c.pendingMu.Lock()
			ch, exists := c.pendingRequests[env.RequestID]
			c.pendingMu.Unlock()
			if exists {
				select {
				case ch <- env:
				default:
				}
				continue
			}
		}

		if env.Type == "heartbeat.ping" {
			c.Send("heartbeat.pong", nil)
			continue
		}

		if c.onMessage != nil {
			c.onMessage(env)
		}
	}
}

func generateRequestID() string {
	return "req-" + time.Now().Format("20060102150405") + "-" + randomString(6)
}

func randomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[time.Now().UnixNano()%int64(len(letters))]
		time.Sleep(time.Nanosecond)
	}
	return string(b)
}

var (
	ErrDisconnected = &ClientError{message: "disconnected"}
	ErrTimeout      = &ClientError{message: "request timeout"}
)

type ClientError struct {
	message string
}

func (e *ClientError) Error() string { return e.message }
