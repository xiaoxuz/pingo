package session

import (
	"testing"
	"time"
)

func TestRegistryPublishesEventsOnlyToPrimarySessionForAgent(t *testing.T) {
	registry := NewRegistry()
	primary := registry.Register(RegisterRequest{
		ID:       "session-primary",
		AgentID:  "agent-b",
		Provider: "claude",
	})
	secondary := registry.Register(RegisterRequest{
		ID:       "session-secondary",
		AgentID:  "agent-b",
		Provider: "codex",
	})
	other := registry.Register(RegisterRequest{
		ID:       "session-other",
		AgentID:  "agent-c",
		Provider: "claude",
	})

	primaryEvents := registry.Subscribe(primary.ID)
	secondaryEvents := registry.Subscribe(secondary.ID)
	otherEvents := registry.Subscribe(other.ID)

	result := registry.Publish("agent-b", Event{Type: "message", UnreadCount: 1})
	if result.Status != "queued" || result.SessionID != primary.ID {
		t.Fatalf("publish result = %+v", result)
	}

	assertEvent(t, primaryEvents, Event{Type: "message", UnreadCount: 1})
	assertNoEvent(t, secondaryEvents)
	assertNoEvent(t, otherEvents)
}

func TestRegistryPromotesNewestSessionWhenPrimarySessionIsRemoved(t *testing.T) {
	registry := NewRegistry()
	first := registry.Register(RegisterRequest{ID: "session-1", AgentID: "agent-b", Provider: "claude"})
	second := registry.Register(RegisterRequest{ID: "session-2", AgentID: "agent-b", Provider: "codex"})
	firstEvents := registry.Subscribe(first.ID)
	secondEvents := registry.Subscribe(second.ID)

	if !registry.Remove(first.ID, "agent-b") {
		t.Fatal("expected primary session to be removed")
	}
	registry.Publish("agent-b", Event{Type: "message", UnreadCount: 2})

	assertNoEvent(t, firstEvents)
	assertEvent(t, secondEvents, Event{Type: "message", UnreadCount: 2})
}

func TestRegistryReportsWhyEventCannotBeQueued(t *testing.T) {
	registry := NewRegistry()
	if result := registry.Publish("missing-agent", Event{Type: "message"}); result.Status != "no_primary_session" {
		t.Fatalf("missing session result = %+v", result)
	}

	registered := registry.Register(RegisterRequest{ID: "window", AgentID: "agent-b", Provider: "claude"})
	for index := 0; index < cap(registry.sessions[registered.ID].events); index++ {
		if result := registry.Publish("agent-b", Event{Type: "message"}); result.Status != "queued" {
			t.Fatalf("fill queue result %d = %+v", index, result)
		}
	}
	if result := registry.Publish("agent-b", Event{Type: "message"}); result.Status != "queue_full" || result.SessionID != registered.ID {
		t.Fatalf("full queue result = %+v", result)
	}
}

func TestRegistryPublishesRuntimeEventToExactSession(t *testing.T) {
	registry := NewRegistry()
	first := registry.Register(RegisterRequest{ID: "window-1", AgentID: "agent-b", Provider: "claude"})
	second := registry.Register(RegisterRequest{ID: "window-2", AgentID: "agent-b", Provider: "claude"})

	result := registry.PublishSession(second.ID, "agent-b", Event{Type: "runtime.free", Reason: "hook_stop"})
	if result.Status != "queued" || result.SessionID != second.ID {
		t.Fatalf("publish result = %+v", result)
	}
	assertNoEvent(t, registry.Subscribe(first.ID))
	assertEvent(t, registry.Subscribe(second.ID), Event{Type: "runtime.free", Reason: "hook_stop"})
	if result := registry.PublishSession(second.ID, "agent-a", Event{Type: "runtime.free"}); result.Status != "session_not_found" {
		t.Fatalf("cross-agent publish result = %+v", result)
	}
}

func TestRegistryTracksWakeStateAndExpiresStaleReports(t *testing.T) {
	registry := NewRegistry()
	registered := registry.Register(RegisterRequest{ID: "window", AgentID: "agent-b", Provider: "claude", CWD: "/work"})
	if registered.Status.State != "free" || registered.Status.Evidence != "session_started" || registered.Status.Detail != "会话已启动，默认空闲" {
		t.Fatalf("initial status = %+v", registered.Status)
	}
	if !registry.UpdateStatus("window", "agent-a", StatusReport{State: "busy"}) {
		// ownership must be rejected
	} else {
		t.Fatal("wrong agent updated session")
	}
	if !registry.UpdateStatus("window", "agent-b", StatusReport{State: "busy", Reason: "interaction_required", Pending: true, LastEventAt: time.Now()}) {
		t.Fatal("owner update rejected")
	}
	list := registry.List("agent-b")
	if len(list) != 1 || list[0].Status.State != "busy" || !list[0].Status.Pending || list[0].Status.Reason != "interaction_required" {
		t.Fatalf("wrong status: %+v", list)
	}
	if len(registry.List("agent-a")) != 0 {
		t.Fatal("cross-agent session leaked")
	}
	registry.mu.Lock()
	registry.sessions["window"].Status.UpdatedAt = time.Now().Add(-time.Minute)
	registry.mu.Unlock()
	if got := registry.List("agent-b")[0].Status.State; got != "unknown" {
		t.Fatalf("stale status = %q", got)
	}
}

func TestRegistryKeepsRecentStateTransitions(t *testing.T) {
	registry := NewRegistry()
	registry.Register(RegisterRequest{ID: "window", AgentID: "agent-b", Provider: "claude"})
	registry.UpdateStatus("window", "agent-b", StatusReport{State: "busy", Evidence: "prompt_submitted", Detail: "输入“1+1”后提交", Sequence: 2})
	registry.UpdateStatus("window", "agent-b", StatusReport{State: "free", Evidence: "pty_empty_prompt", Detail: "PTY 检测到稳定的空输入框", Sequence: 3})
	status := registry.List("agent-b")[0].Status
	if len(status.History) != 2 || status.History[0].State != "busy" || status.History[1].State != "free" {
		t.Fatalf("history = %+v", status.History)
	}
	registry.UpdateStatus("window", "agent-b", StatusReport{State: "free", Evidence: "pty_empty_prompt", Detail: "PTY 检测到稳定的空输入框", Sequence: 3})
	if got := len(registry.List("agent-b")[0].Status.History); got != 2 {
		t.Fatalf("duplicate heartbeat added transition: %d", got)
	}
}

func assertEvent(t *testing.T, events <-chan Event, want Event) {
	t.Helper()
	select {
	case got := <-events:
		if got != want {
			t.Fatalf("event = %#v, want %#v", got, want)
		}
	case <-time.After(time.Second):
		t.Fatalf("expected event %#v", want)
	}
}

func assertNoEvent(t *testing.T, events <-chan Event) {
	t.Helper()
	select {
	case got := <-events:
		t.Fatalf("unexpected event %#v", got)
	case <-time.After(30 * time.Millisecond):
	}
}
