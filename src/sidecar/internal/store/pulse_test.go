package store

import (
	"testing"
	"time"
)

func TestTodoLifecycleIsIsolatedByAgent(t *testing.T) {
	storage, err := NewSQLiteStore(t.TempDir() + "/pingo.db")
	if err != nil {
		t.Fatal(err)
	}
	defer storage.Close()
	now := time.Date(2026, 9, 28, 10, 0, 0, 0, time.UTC)
	todo, err := storage.CreateTodo("agent-a", TodoRecord{Title: "跟进协作", Context: "等待回复", Priority: "high", DueAt: now.Add(time.Hour), RemindAt: now})
	if err != nil {
		t.Fatal(err)
	}
	if todo.ID == "" || todo.Status != "pending" {
		t.Fatalf("todo = %+v", todo)
	}
	if got, _ := storage.ListTodos("agent-b", ""); len(got) != 0 {
		t.Fatalf("cross-agent leak: %+v", got)
	}
	if err := storage.SnoozeTodo("agent-a", todo.ID, now.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	due, err := storage.ListDueTodos("agent-a", now.Add(time.Hour))
	if err != nil || len(due) != 0 {
		t.Fatalf("snoozed todo due too early: %+v %v", due, err)
	}
	if err := storage.CompleteTodo("agent-a", todo.ID, now.Add(3*time.Hour)); err != nil {
		t.Fatal(err)
	}
	items, _ := storage.ListTodos("agent-a", "completed")
	if len(items) != 1 || items[0].CompletedAt.IsZero() {
		t.Fatalf("completed todos = %+v", items)
	}
}

func TestPulseSettingsAndHistoryLifecycle(t *testing.T) {
	storage, err := NewSQLiteStore(t.TempDir() + "/pingo.db")
	if err != nil {
		t.Fatal(err)
	}
	defer storage.Close()
	settings, err := storage.GetPulseSettings("agent-a")
	if err != nil {
		t.Fatal(err)
	}
	if settings.Mode != "balanced" || settings.QuietStart != "23:00" || settings.QuietEnd != "08:00" || settings.DailyBudget != 1 {
		t.Fatalf("defaults = %+v", settings)
	}
	settings.Mode, settings.DailyBudget = "active", 3
	if err := storage.SavePulseSettings(settings); err != nil {
		t.Fatal(err)
	}
	loaded, _ := storage.GetPulseSettings("agent-a")
	if loaded.Mode != "active" || loaded.DailyBudget != 3 {
		t.Fatalf("saved = %+v", loaded)
	}

	now := time.Date(2026, 9, 28, 10, 0, 0, 0, time.UTC)
	pulse, err := storage.CreatePulse(PulseRecord{AgentID: "agent-a", Reason: "profile_review", Prompt: "检查资料", Status: "pending", CreatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	if err := storage.MarkPulseDelivered("agent-a", pulse.ID, "window-1", now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := storage.CompletePulse("agent-a", pulse.ID, "资料无需修改", false, now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	history, _ := storage.ListPulses("agent-a", 10)
	if len(history) != 1 || history[0].Status != "completed" || history[0].Result != "资料无需修改" || history[0].ActionTaken {
		t.Fatalf("history = %+v", history)
	}
}
