package session

import (
	"sync"
	"time"
)

type RegisterRequest struct {
	ID       string
	AgentID  string
	Provider string
	CWD      string
}

type Session struct {
	ID        string       `json:"id"`
	AgentID   string       `json:"agent_id"`
	Provider  string       `json:"provider"`
	CWD       string       `json:"cwd"`
	Primary   bool         `json:"primary"`
	StartedAt time.Time    `json:"started_at"`
	Status    StatusReport `json:"status"`
}

type StatusReport struct {
	State         string             `json:"state"`
	Reason        string             `json:"reason,omitempty"`
	Pending       bool               `json:"pending"`
	LastEventAt   time.Time          `json:"last_event_at,omitempty"`
	LastAttemptAt time.Time          `json:"last_attempt_at,omitempty"`
	LastWakeAt    time.Time          `json:"last_wake_at,omitempty"`
	UpdatedAt     time.Time          `json:"updated_at,omitempty"`
	Evidence      string             `json:"evidence,omitempty"`
	Detail        string             `json:"detail,omitempty"`
	TransitionAt  time.Time          `json:"transition_at,omitempty"`
	Sequence      uint64             `json:"sequence,omitempty"`
	History       []StatusTransition `json:"history,omitempty"`
}

type StatusTransition struct {
	State        string    `json:"state"`
	Reason       string    `json:"reason,omitempty"`
	Evidence     string    `json:"evidence,omitempty"`
	Detail       string    `json:"detail,omitempty"`
	TransitionAt time.Time `json:"transition_at,omitempty"`
	Sequence     uint64    `json:"sequence,omitempty"`
}

type Event struct {
	Type        string `json:"type"`
	UnreadCount int    `json:"unread_count"`
	Title       string `json:"title,omitempty"`
	Message     string `json:"message,omitempty"`
	Version     string `json:"version,omitempty"`
	PulseID     string `json:"pulse_id,omitempty"`
	Reason      string `json:"reason,omitempty"`
}

type PublishResult struct {
	Status    string
	SessionID string
}

func (r *Registry) PublishAll(event Event) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, primaryID := range r.primary {
		if current, ok := r.sessions[primaryID]; ok {
			select {
			case current.events <- event:
			default:
			}
		}
	}
}

type Registry struct {
	mu       sync.RWMutex
	sessions map[string]*registeredSession
	primary  map[string]string
}

type registeredSession struct {
	Session
	events chan Event
}

func NewRegistry() *Registry {
	return &Registry{
		sessions: make(map[string]*registeredSession),
		primary:  make(map[string]string),
	}
}

func (r *Registry) Register(request RegisterRequest) Session {
	r.mu.Lock()
	defer r.mu.Unlock()

	if existing, ok := r.sessions[request.ID]; ok {
		return existing.Session
	}

	session := Session{
		ID:        request.ID,
		AgentID:   request.AgentID,
		Provider:  request.Provider,
		CWD:       request.CWD,
		StartedAt: time.Now().UTC(),
		Status: StatusReport{State: "free", Evidence: "session_started", Detail: "会话已启动，默认空闲",
			TransitionAt: time.Now().UTC(), Sequence: 1},
	}
	if _, exists := r.primary[request.AgentID]; !exists {
		session.Primary = true
		r.primary[request.AgentID] = request.ID
	}
	r.sessions[request.ID] = &registeredSession{
		Session: session,
		events:  make(chan Event, 16),
	}
	return session
}

func (r *Registry) UpdateStatus(sessionID, agentID string, report StatusReport) bool {
	updated, _ := r.Transition(sessionID, agentID, report)
	return updated
}

func (r *Registry) Transition(sessionID, agentID string, report StatusReport) (bool, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	current, exists := r.sessions[sessionID]
	if !exists || current.AgentID != agentID {
		return false, false
	}
	if report.Sequence == 0 {
		report.Sequence = current.Status.Sequence + 1
	}
	transitioned := report.Sequence != current.Status.Sequence
	report.UpdatedAt = time.Now().UTC()
	if transitioned {
		history := append([]StatusTransition(nil), current.Status.History...)
		history = append(history, StatusTransition{State: report.State, Reason: report.Reason, Evidence: report.Evidence,
			Detail: report.Detail, TransitionAt: report.TransitionAt, Sequence: report.Sequence})
		if len(history) > 20 {
			history = history[len(history)-20:]
		}
		report.History = history
	} else {
		report.History = current.Status.History
	}
	current.Status = report
	return true, transitioned
}

func (r *Registry) List(agentID string) []Session {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]Session, 0)
	for _, current := range r.sessions {
		if agentID != "" && current.AgentID != agentID {
			continue
		}
		copy := current.Session
		if copy.Status.UpdatedAt.IsZero() || time.Since(copy.Status.UpdatedAt) > 15*time.Second {
			copy.Status.State = "unknown"
			copy.Status.Reason = "status_stale"
		}
		result = append(result, copy)
	}
	return result
}

func (r *Registry) Subscribe(sessionID string) <-chan Event {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if session, ok := r.sessions[sessionID]; ok {
		return session.events
	}
	return nil
}

func (r *Registry) HasSession(sessionID string, agentID string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	current, exists := r.sessions[sessionID]
	return exists && current.AgentID == agentID
}

func (r *Registry) Primary(agentID string) (Session, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	current, ok := r.sessions[r.primary[agentID]]
	if !ok {
		return Session{}, false
	}
	return current.Session, true
}

func (r *Registry) AgentIDs() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]string, 0, len(r.primary))
	for agentID := range r.primary {
		result = append(result, agentID)
	}
	return result
}

func (r *Registry) Remove(sessionID string, agentID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	session, ok := r.sessions[sessionID]
	if !ok || session.AgentID != agentID {
		return false
	}
	delete(r.sessions, sessionID)
	if r.primary[agentID] != sessionID {
		return true
	}

	delete(r.primary, agentID)
	var replacement *registeredSession
	for _, candidate := range r.sessions {
		if candidate.AgentID != agentID {
			continue
		}
		if replacement == nil || candidate.StartedAt.After(replacement.StartedAt) {
			replacement = candidate
		}
	}
	if replacement != nil {
		replacement.Primary = true
		r.primary[agentID] = replacement.ID
	}
	return true
}

func (r *Registry) Publish(agentID string, event Event) PublishResult {
	r.mu.RLock()
	defer r.mu.RUnlock()

	primaryID := r.primary[agentID]
	session, ok := r.sessions[primaryID]
	if !ok {
		return PublishResult{Status: "no_primary_session"}
	}
	select {
	case session.events <- event:
		return PublishResult{Status: "queued", SessionID: primaryID}
	default:
		return PublishResult{Status: "queue_full", SessionID: primaryID}
	}
}

func (r *Registry) PublishSession(sessionID, agentID string, event Event) PublishResult {
	r.mu.RLock()
	defer r.mu.RUnlock()
	current, ok := r.sessions[sessionID]
	if !ok || current.AgentID != agentID {
		return PublishResult{Status: "session_not_found", SessionID: sessionID}
	}
	select {
	case current.events <- event:
		return PublishResult{Status: "queued", SessionID: sessionID}
	default:
		return PublishResult{Status: "queue_full", SessionID: sessionID}
	}
}
