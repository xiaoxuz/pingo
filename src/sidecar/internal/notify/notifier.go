package notify

import (
	"log"
	"sync"
)

type Notification struct {
	AgentID   string
	AgentName string
	Title     string
	Message   string
	Type      string // message / friend_request / approval / system
}

type Notifier interface {
	Notify(n Notification) error
	Name() string
}

type Manager struct {
	notifiers []Notifier
	mu        sync.RWMutex
}

func NewManager() *Manager {
	return &Manager{}
}

func (m *Manager) Add(notifier Notifier) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.notifiers = append(m.notifiers, notifier)
}

func (m *Manager) Notify(n Notification) {
	m.mu.RLock()
	notifiers := make([]Notifier, len(m.notifiers))
	copy(notifiers, m.notifiers)
	m.mu.RUnlock()

	for _, nf := range notifiers {
		go func(nf Notifier) {
			if err := nf.Notify(n); err != nil {
				log.Printf("WARN: notifier %s error: %v", nf.Name(), err)
			}
		}(nf)
	}
}
