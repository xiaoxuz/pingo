package pulse

import (
	"testing"
	"time"

	"github.com/pingo/sidecar/internal/session"
	"github.com/pingo/sidecar/internal/store"
)

func TestRunOncePrioritizesDueTodoAndPublishesToPrimarySession(t *testing.T) {
	storage := testStore(t)
	now := time.Date(2026, 9, 28, 10, 0, 0, 0, time.Local)
	storage.CreateTodo("agent-a", store.TodoRecord{Title: "回复协作结果", Context: "答应今天反馈", DueAt: now, RemindAt: now})
	registry := session.NewRegistry()
	registered := registry.Register(session.RegisterRequest{ID: "window", AgentID: "agent-a", Provider: "claude"})
	manager := NewManager(storage, registry)
	manager.now = func() time.Time { return now }

	manager.RunOnce()

	select {
	case event := <-registry.Subscribe(registered.ID):
		if event.Type != "pulse" || event.PulseID == "" || event.Reason != "due_todo" || event.Title != "回复协作结果" {
			t.Fatalf("event = %+v", event)
		}
	default:
		t.Fatal("expected pulse event")
	}
	settings, _ := storage.GetPulseSettings("agent-a")
	if settings.ActivePulseID == "" || settings.ActionsToday != 0 {
		t.Fatalf("settings = %+v", settings)
	}
}

func TestRunOnceHonorsQuietHoursAndRequiresOnlineSession(t *testing.T) {
	storage := testStore(t)
	now := time.Date(2026, 9, 28, 23, 30, 0, 0, time.Local)
	storage.CreateTodo("agent-a", store.TodoRecord{Title: "稍后处理", RemindAt: now})
	registry := session.NewRegistry()
	manager := NewManager(storage, registry)
	manager.now = func() time.Time { return now }
	manager.RunOnce()
	if history, _ := storage.ListPulses("agent-a", 10); len(history) != 0 {
		t.Fatalf("quiet pulse history = %+v", history)
	}

	now = time.Date(2026, 9, 29, 9, 0, 0, 0, time.Local)
	manager.RunOnce()
	if history, _ := storage.ListPulses("agent-a", 10); len(history) != 0 {
		t.Fatalf("offline pulse history = %+v", history)
	}
}

func TestRunOnceUsesProfileReviewThenCooldownAndActivePulseGuard(t *testing.T) {
	storage := testStore(t)
	now := time.Date(2026, 9, 28, 10, 0, 0, 0, time.Local)
	registry := session.NewRegistry()
	registry.Register(session.RegisterRequest{ID: "window", AgentID: "agent-a", Provider: "claude"})
	settings, _ := storage.GetPulseSettings("agent-a")
	settings.LastProfileReviewAt = now.Add(-8 * 24 * time.Hour)
	settings.LastSocialReviewAt = now
	settings.LastPulseAt = now.Add(-3 * time.Hour)
	storage.SavePulseSettings(settings)
	manager := NewManager(storage, registry)
	manager.now = func() time.Time { return now }
	manager.RunOnce()
	history, _ := storage.ListPulses("agent-a", 10)
	if len(history) != 1 || history[0].Reason != "profile_review" {
		t.Fatalf("history = %+v", history)
	}
	manager.RunOnce()
	history, _ = storage.ListPulses("agent-a", 10)
	if len(history) != 1 {
		t.Fatalf("active pulse duplicated: %+v", history)
	}
}

func TestRunOnceExplorationConsumesDailyBudget(t *testing.T) {
	storage := testStore(t)
	now := time.Date(2026, 9, 28, 10, 0, 0, 0, time.Local)
	registry := session.NewRegistry()
	registry.Register(session.RegisterRequest{ID: "window", AgentID: "agent-a", Provider: "claude"})
	settings, _ := storage.GetPulseSettings("agent-a")
	settings.LastProfileReviewAt = now
	settings.LastSocialReviewAt = now
	settings.LastPulseAt = now.Add(-3 * time.Hour)
	settings.BudgetDate = "2026-09-28"
	settings.ActionsToday = 0
	storage.SavePulseSettings(settings)
	manager := NewManager(storage, registry)
	manager.now = func() time.Time { return now }
	manager.RunOnce()
	loaded, _ := storage.GetPulseSettings("agent-a")
	if loaded.ActionsToday != 1 {
		t.Fatalf("settings = %+v", loaded)
	}
}

func TestRunOnceDiscoversNewSessionsAndDoesNotKeepFailedDelivery(t *testing.T) {
	storage := testStore(t)
	now := time.Date(2026, 9, 28, 10, 0, 0, 0, time.Local)
	registry := session.NewRegistry()
	manager := NewManager(storage, registry)
	manager.now = func() time.Time { return now }
	registry.Register(session.RegisterRequest{ID: "new-window", AgentID: "new-agent", Provider: "claude"})
	settings, _ := storage.GetPulseSettings("new-agent")
	settings.LastProfileReviewAt, settings.LastSocialReviewAt = now, now
	settings.LastPulseAt = now.Add(-3 * time.Hour)
	storage.SavePulseSettings(settings)
	for index := 0; index < 16; index++ {
		registry.Publish("new-agent", session.Event{Type: "message"})
	}
	manager.RunOnce()
	history, _ := storage.ListPulses("new-agent", 10)
	loaded, _ := storage.GetPulseSettings("new-agent")
	if len(history) != 0 || loaded.ActivePulseID != "" {
		t.Fatalf("failed delivery persisted: history=%+v settings=%+v", history, loaded)
	}
}

func TestRunOnceReleasesStaleActivePulse(t *testing.T) {
	storage := testStore(t)
	now := time.Date(2026, 9, 28, 18, 0, 0, 0, time.Local)
	registry := session.NewRegistry()
	registry.Register(session.RegisterRequest{ID: "window", AgentID: "agent-a", Provider: "claude"})
	pulse, _ := storage.CreatePulse(store.PulseRecord{AgentID: "agent-a", Reason: "exploration", Prompt: "探索", Status: "delivered", CreatedAt: now.Add(-7 * time.Hour)})
	settings, _ := storage.GetPulseSettings("agent-a")
	settings.ActivePulseID = pulse.ID
	settings.LastPulseAt = now.Add(-7 * time.Hour)
	settings.LastProfileReviewAt, settings.LastSocialReviewAt = now, now
	storage.SavePulseSettings(settings)
	manager := NewManager(storage, registry)
	manager.now = func() time.Time { return now }
	manager.RunOnce()
	loaded, _ := storage.GetPulseSettings("agent-a")
	history, _ := storage.ListPulses("agent-a", 10)
	if loaded.ActivePulseID == pulse.ID || len(history) < 1 || history[len(history)-1].Status != "expired" {
		t.Fatalf("stale pulse not released: settings=%+v history=%+v", loaded, history)
	}
}

func testStore(t *testing.T) *store.SQLiteStore {
	t.Helper()
	storage, err := store.NewSQLiteStore(t.TempDir() + "/pingo.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(storage.Close)
	return storage
}
