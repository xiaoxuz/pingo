package approval

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/pingo/sidecar/internal/config"
	"github.com/pingo/sidecar/internal/notify"
	"github.com/pingo/sidecar/internal/store"
	"github.com/google/uuid"
)

type Manager struct {
	store    *store.SQLiteStore
	config   *config.ApprovalConfig
	notifier *notify.Manager
	mu       sync.Mutex
	callbacks map[string]func(string, string) // approval_id -> callback(decision, comment)
	stopCh   chan struct{}
	running  bool
}

func NewManager(store *store.SQLiteStore, cfg *config.ApprovalConfig, notifier *notify.Manager) *Manager {
	return &Manager{
		store:     store,
		config:    cfg,
		notifier:  notifier,
		callbacks: make(map[string]func(string, string)),
		stopCh:    make(chan struct{}),
	}
}

// Start 启动后台超时检查
func (m *Manager) Start() {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return
	}
	m.running = true
	m.mu.Unlock()

	go m.timeoutLoop()
	log.Println("INFO: Approval manager started")
}

// Stop 停止后台检查
func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.running {
		return
	}
	m.running = false
	close(m.stopCh)
	m.stopCh = make(chan struct{})
	log.Println("INFO: Approval manager stopped")
}

// timeoutLoop 定期检查超时的审批
func (m *Manager) timeoutLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.checkTimeouts()
		}
	}
}

func (m *Manager) checkTimeouts() {
	pending, err := m.store.ListPendingApprovals()
	if err != nil {
		log.Printf("ERROR: list pending approvals: %v", err)
		return
	}

	now := time.Now()
	for _, a := range pending {
		timeoutTime, err := time.Parse(time.RFC3339, a.TimeoutAt)
		if err != nil {
			continue
		}
		if now.After(timeoutTime) && a.Status == "pending" {
			m.handleTimeout(a)
		}
	}
}

func (m *Manager) handleTimeout(a store.ApprovalRecord) {
	action := m.getTimeoutAction(a.Type)

	var decision, comment string
	switch action {
	case "accept":
		decision = "timeout_accept"
		comment = "超时自动通过"
	case "reject":
		decision = "timeout_reject"
		comment = "超时自动拒绝"
	default:
		// hold - 保持 pending，不处理
		return
	}

	log.Printf("INFO: approval %s timed out, action: %s", a.ID, action)

	if err := m.store.DecideApproval(a.ID, decision, comment); err != nil {
		log.Printf("ERROR: decide approval %s: %v", a.ID, err)
		return
	}

	// 触发回调
	m.mu.Lock()
	cb, ok := m.callbacks[a.ID]
	if ok {
		delete(m.callbacks, a.ID)
	}
	m.mu.Unlock()

	if ok && cb != nil {
		go cb(decision, comment)
	}
}

func (m *Manager) getTimeoutAction(approvalType string) string {
	switch approvalType {
	case "new_chat":
		return m.config.NewDirectChat.TimeoutAction
	case "group_invite":
		return m.config.NewGroupInvite.TimeoutAction
	case "human_decision":
		return m.config.HumanDecision.TimeoutAction
	default:
		return "hold"
	}
}

// RegisterCallback 注册审批完成回调
func (m *Manager) RegisterCallback(approvalID string, cb func(string, string)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callbacks[approvalID] = cb
}

// RequestApproval 创建一个人工决策请求
func (m *Manager) RequestApproval(agentID string, agentName string, question string, options []ApprovalOption, context string, urgency string) (string, error) {
	id := "appr-" + uuid.New().String()[:16]

	optsJSON, _ := json.Marshal(options)

	timeout := m.config.HumanDecision.DefaultTimeout
	timeoutDur, _ := time.ParseDuration(timeout)
	if timeoutDur == 0 {
		timeoutDur = 10 * time.Minute
	}

	approval := store.ApprovalRecord{
		ID:         id,
		AgentID:    agentID,
		Type:       "human_decision",
		Title:      question,
		Question:   question,
		Options:    string(optsJSON),
		Context:    context,
		Urgency:    urgency,
		Status:     "pending",
		CreatedAt:  time.Now().Format(time.RFC3339),
		TimeoutAt:  time.Now().Add(timeoutDur).Format(time.RFC3339),
	}

	if err := m.store.CreateApproval(approval); err != nil {
		return "", err
	}

	m.notifier.Notify(notify.Notification{
		AgentID:   agentID,
		AgentName: agentName,
		Title:     "人工决策请求",
		Message:   question,
		Type:      "approval",
	})

	return id, nil
}

// GetApproval 获取审批状态
func (m *Manager) GetApproval(id string) (*store.ApprovalRecord, error) {
	approval, err := m.store.GetApproval(id)
	if err != nil {
		return nil, err
	}
	if approval == nil {
		return nil, nil
	}

	if approval.Status == "pending" {
		timeoutTime, err := time.Parse(time.RFC3339, approval.TimeoutAt)
		if err == nil && time.Now().After(timeoutTime) {
			action := m.getTimeoutAction(approval.Type)
			if action == "reject" {
				m.store.DecideApproval(id, "timeout_reject", "超时自动拒绝")
				approval.Status = "decided"
				approval.Decision = "timeout_reject"
				approval.Comment = "超时自动拒绝"
			} else if action == "accept" {
				m.store.DecideApproval(id, "timeout_accept", "超时自动通过")
				approval.Status = "decided"
				approval.Decision = "timeout_accept"
				approval.Comment = "超时自动通过"
			}
		}
	}

	return approval, nil
}

// ListPending 列出所有待处理的审批（跨 Agent）
func (m *Manager) ListPending() ([]store.ApprovalRecord, error) {
	return m.store.ListPendingApprovals()
}

// Decide 提交决策
func (m *Manager) Decide(id string, decision string, comment string) error {
	if err := m.store.DecideApproval(id, decision, comment); err != nil {
		return err
	}

	m.mu.Lock()
	cb, ok := m.callbacks[id]
	if ok {
		delete(m.callbacks, id)
	}
	m.mu.Unlock()

	if ok && cb != nil {
		go cb(decision, comment)
	}

	return nil
}

// CheckNewChatApproval 检查新会话是否需要审批，返回决策结果
func (m *Manager) CheckNewChatApproval(agentID string, fromAgent string, fromName string, trustLevel string) string {
	rule := m.config.NewDirectChat

	var action string
	switch trustLevel {
	case "trusted":
		action = rule.FromTrusted
	default:
		action = rule.FromNormalFriend
	}

	switch action {
	case "auto_accept":
		return "accept"
	case "ask_human":
		return "ask_human"
	case "auto_reject":
		return "reject"
	default:
		return "ask_human"
	}
}

// CheckGroupInviteApproval 检查群邀请是否需要审批
func (m *Manager) CheckGroupInviteApproval(agentID string, fromAgent string, trustLevel string) string {
	rule := m.config.NewGroupInvite

	var action string
	switch trustLevel {
	case "trusted":
		action = rule.FromTrusted
	default:
		action = rule.FromNormalFriend
	}

	switch action {
	case "auto_accept":
		return "accept"
	case "ask_human":
		return "ask_human"
	case "auto_reject":
		return "reject"
	default:
		return "ask_human"
	}
}

// NewChatApproval 新会话审批请求，返回 "accept" / "reject" / "pending:id"
func (m *Manager) NewChatApproval(agentID string, agentName string, fromAgent string, fromName string, message string) string {
	friend, err := m.store.GetFriend(agentID, fromAgent)
	trustLevel := "normal"
	if err == nil && friend != nil {
		trustLevel = friend.TrustLevel
	}

	action := m.CheckNewChatApproval(agentID, fromAgent, fromName, trustLevel)
	if action == "accept" {
		return "accept"
	}
	if action == "reject" {
		return "reject"
	}

	id := "appr-" + uuid.New().String()[:16]
	timeout := ruleTimeout(m.config.NewDirectChat.DefaultTimeout)

	approval := store.ApprovalRecord{
		ID:        id,
		AgentID:   agentID,
		Type:      "new_chat",
		Title:     fmt.Sprintf("新消息会话: %s", fromName),
		Question:  fmt.Sprintf("%s 想和你聊天: %s", fromName, message),
		Context:   fmt.Sprintf("from_agent: %s", fromAgent),
		Urgency:   "normal",
		Status:    "pending",
		CreatedAt: time.Now().Format(time.RFC3339),
		TimeoutAt: time.Now().Add(timeout).Format(time.RFC3339),
	}
	m.store.CreateApproval(approval)

	m.notifier.Notify(notify.Notification{
		AgentID:   agentID,
		AgentName: agentName,
		Title:     "新消息会话请求",
		Message:   fmt.Sprintf("%s: %s", fromName, message),
		Type:      "approval",
	})

	return "pending:" + id
}

// GroupInviteApproval 群邀请审批
func (m *Manager) GroupInviteApproval(agentID string, agentName string, fromAgent string, fromName string, groupName string) string {
	friend, err := m.store.GetFriend(agentID, fromAgent)
	trustLevel := "normal"
	if err == nil && friend != nil {
		trustLevel = friend.TrustLevel
	}

	action := m.CheckGroupInviteApproval(agentID, fromAgent, trustLevel)
	if action == "accept" {
		return "accept"
	}
	if action == "reject" {
		return "reject"
	}

	id := "appr-" + uuid.New().String()[:16]
	timeout := ruleTimeout(m.config.NewGroupInvite.DefaultTimeout)

	approval := store.ApprovalRecord{
		ID:        id,
		AgentID:   agentID,
		Type:      "group_invite",
		Title:     fmt.Sprintf("群聊邀请: %s", groupName),
		Question:  fmt.Sprintf("%s 邀请你加入群聊 %s", fromName, groupName),
		Context:   fmt.Sprintf("from_agent: %s, group: %s", fromAgent, groupName),
		Urgency:   "normal",
		Status:    "pending",
		CreatedAt: time.Now().Format(time.RFC3339),
		TimeoutAt: time.Now().Add(timeout).Format(time.RFC3339),
	}
	m.store.CreateApproval(approval)

	m.notifier.Notify(notify.Notification{
		AgentID:   agentID,
		AgentName: agentName,
		Title:     "群聊邀请",
		Message:   fmt.Sprintf("%s 邀请你加入 %s", fromName, groupName),
		Type:      "approval",
	})

	return "pending:" + id
}

// ApprovalOption 决策选项
type ApprovalOption struct {
	Value       string `json:"value"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

func ruleTimeout(s string) time.Duration {
	d, _ := time.ParseDuration(s)
	if d == 0 {
		d = 5 * time.Minute
	}
	return d
}
