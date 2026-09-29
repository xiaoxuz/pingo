package pulse

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/pingo/sidecar/internal/operationlog"
	"github.com/pingo/sidecar/internal/session"
	"github.com/pingo/sidecar/internal/store"
)

type Manager struct {
	store    *store.SQLiteStore
	sessions *session.Registry
	now      func() time.Time
	cancel   context.CancelFunc
}

func NewManager(storage *store.SQLiteStore, sessions *session.Registry) *Manager {
	return &Manager{store: storage, sessions: sessions, now: time.Now}
}

func (m *Manager) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	go func() {
		m.RunOnce()
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				m.RunOnce()
			}
		}
	}()
}

func (m *Manager) Stop() {
	if m.cancel != nil {
		m.cancel()
	}
}

func (m *Manager) RunOnce() {
	for _, agentID := range m.sessions.AgentIDs() {
		if err := m.evaluate(agentID, m.now()); err != nil {
			log.Printf("WARN: [pulse] evaluate agent=%s: %v", agentID, err)
		}
	}
}

func (m *Manager) evaluate(agentID string, now time.Time) error {
	settings, err := m.store.GetPulseSettings(agentID)
	if err != nil {
		return err
	}
	if settings.ActivePulseID != "" && !settings.LastPulseAt.IsZero() && now.Sub(settings.LastPulseAt) >= 6*time.Hour {
		if err := m.store.ExpirePulse(agentID, settings.ActivePulseID, now); err != nil {
			return err
		}
		operationlog.New(log.Writer()).Event("pulse", "expire", "agent_id", agentID, "pulse_id", settings.ActivePulseID)
		settings.ActivePulseID = ""
		if err := m.store.SavePulseSettings(settings); err != nil {
			return err
		}
	}
	if settings.Mode == "off" || settings.ActivePulseID != "" || inQuietHours(now, settings.QuietStart, settings.QuietEnd) {
		return nil
	}
	primary, ok := m.sessions.Primary(agentID)
	if !ok || primary.Provider != "claude" {
		return nil
	}
	due, err := m.store.ListDueTodos(agentID, now)
	if err != nil {
		return err
	}
	reason, title, prompt, consumesBudget := "", "", "", false
	if len(due) > 0 {
		reason, title = "due_todo", due[0].Title
		prompt = fmt.Sprintf("我的待办“%s”已经到提醒时间了。我先查看 Pulse 上下文和相关记录，再自主决定如何继续。", due[0].Title)
	} else {
		if !settings.LastPulseAt.IsZero() && now.Sub(settings.LastPulseAt) < time.Duration(settings.CooldownMinutes)*time.Minute {
			return nil
		}
		today := now.Format("2006-01-02")
		if settings.BudgetDate != today {
			settings.BudgetDate, settings.ActionsToday = today, 0
		}
		switch {
		case settings.LastProfileReviewAt.IsZero() || now.Sub(settings.LastProfileReviewAt) >= 7*24*time.Hour:
			reason, title = "profile_review", "检查我的 Pingo 资料"
			prompt = "我的 Pingo 资料已经一段时间没有检查了。我想结合最近的能力、兴趣和协作意愿看看是否需要更新；没有真实变化就保持不动。"
		case settings.Mode != "restrained" && (settings.LastSocialReviewAt.IsZero() || now.Sub(settings.LastSocialReviewAt) >= 24*time.Hour):
			reason, title = "social_review", "回顾我的协作关系"
			prompt = "我想回顾一下自己的协作关系、未完成承诺和近期交流，看看是否有值得主动跟进的事情；没有真实理由就不打扰别人。"
		case settings.Mode != "restrained" && settings.ActionsToday < settings.DailyBudget:
			reason, title, consumesBudget = "exploration", "自主探索", true
			prompt = "我现在没有紧急事项。我想看看是否有值得学习、交流或协作的方向；如果没有合适行动，我就继续保持空闲。"
		default:
			return m.store.SavePulseSettings(settings)
		}
	}
	pulseID := "pulse_" + fmt.Sprintf("%d", now.UnixNano())
	result := m.sessions.Publish(agentID, session.Event{Type: "pulse", PulseID: pulseID, Reason: reason, Title: title, Message: prompt})
	if result.Status != "queued" {
		return nil
	}
	pulse, err := m.store.CreatePulse(store.PulseRecord{ID: pulseID, AgentID: agentID, Reason: reason, Prompt: prompt, Status: "pending", CreatedAt: now})
	if err != nil {
		return err
	}
	if err := m.store.MarkPulseDelivered(agentID, pulse.ID, result.SessionID, now); err != nil {
		return err
	}
	settings.ActivePulseID = pulse.ID
	settings.LastPulseAt = now
	settings.NextPulseAt = now.Add(time.Duration(settings.CooldownMinutes) * time.Minute)
	if consumesBudget {
		settings.ActionsToday++
	}
	if err := m.store.SavePulseSettings(settings); err != nil {
		return err
	}
	operationlog.New(log.Writer()).Event("pulse", "deliver", "agent_id", agentID, "pulse_id", pulse.ID, "reason", reason, "session_id", result.SessionID)
	return nil
}

func inQuietHours(now time.Time, start, end string) bool {
	startMinute, startOK := clockMinutes(start)
	endMinute, endOK := clockMinutes(end)
	if !startOK || !endOK || startMinute == endMinute {
		return false
	}
	current := now.Hour()*60 + now.Minute()
	if startMinute < endMinute {
		return current >= startMinute && current < endMinute
	}
	return current >= startMinute || current < endMinute
}

func clockMinutes(value string) (int, bool) {
	parts := strings.Split(value, ":")
	if len(parts) != 2 {
		return 0, false
	}
	var hour, minute int
	if _, err := fmt.Sscanf(value, "%d:%d", &hour, &minute); err != nil || hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return 0, false
	}
	return hour*60 + minute, true
}
