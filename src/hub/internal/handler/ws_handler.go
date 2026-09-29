package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/pingo/hub/internal/conn"
	"github.com/pingo/hub/internal/service"
	"github.com/pingo/hub/pkg/protocol"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type WsHandler struct {
	manager  *conn.Manager
	router   *conn.Router
	agentSvc *service.AgentService
}

func NewWsHandler(
	manager *conn.Manager,
	router *conn.Router,
	agentSvc *service.AgentService,
) *WsHandler {
	return &WsHandler{
		manager:  manager,
		router:   router,
		agentSvc: agentSvc,
	}
}

func (h *WsHandler) HandleWebSocket(c *gin.Context) {
	ws, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("ERROR: websocket upgrade failed: %v", err)
		return
	}

	// 设置读超时
	ws.SetReadDeadline(time.Now().Add(10 * time.Second))

	// 读取第一条消息，必须是 auth.login
	_, msgData, err := ws.ReadMessage()
	if err != nil {
		log.Printf("WARN: read auth message failed: %v", err)
		ws.Close()
		return
	}

	var env protocol.Envelope
	if err := json.Unmarshal(msgData, &env); err != nil {
		log.Printf("WARN: parse auth message failed: %v", err)
		ws.Close()
		return
	}

	if env.Type != protocol.TypeAuthLogin {
		resp := protocol.NewError(protocol.TypeError, env.RequestID, "", protocol.ErrUnauthorized, "first message must be auth.login")
		ws.WriteJSON(resp)
		ws.Close()
		return
	}

	// 解析登录请求
	var req protocol.AuthLoginRequest
	dataBytes, _ := json.Marshal(env.Data)
	json.Unmarshal(dataBytes, &req)

	if req.AgentID == "" || req.Token == "" {
		resp := protocol.NewError(protocol.TypeAuthLoginResp, env.RequestID, req.AgentID, protocol.ErrBadRequest, "agent_id and token are required")
		ws.WriteJSON(resp)
		ws.Close()
		return
	}

	// 验证 token
	agentID, err := h.agentSvc.ValidateToken(c.Request.Context(), req.Token)
	if err != nil || agentID == "" || agentID != req.AgentID {
		resp := protocol.NewError(protocol.TypeAuthLoginResp, env.RequestID, req.AgentID, protocol.ErrInvalidToken, "invalid token")
		ws.WriteJSON(resp)
		ws.Close()
		return
	}

	// 登录成功，创建连接对象
	connection := conn.NewConnection(agentID, ws)
	h.manager.Register(agentID, connection)

	// 发送登录响应
	connection.Send(protocol.NewResponse(protocol.TypeAuthLoginResp, env.RequestID, agentID, protocol.AuthLoginResponse{
		Success: true,
		AgentID: agentID,
	}))

	// 启动写协程
	go connection.WritePump()

	// 读循环
	h.readLoop(connection)
}

func (h *WsHandler) readLoop(c *conn.Connection) {
	defer func() {
		h.manager.Unregister(c.AgentID())
	}()

	for {
		if c.IsClosed() {
			return
		}
		env, err := c.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure, websocket.CloseNoStatusReceived) {
				log.Printf("WARN: websocket error for agent %s: %v", c.AgentID(), err)
			}
			return
		}
		h.router.HandleMessage(c, env)
	}
}
