package local

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/pingo/sidecar/internal/store"
)

func TestTodoCreateAndPulseCompleteUseCurrentAgent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	storage, err := store.NewSQLiteStore(t.TempDir() + "/pingo.db")
	if err != nil {
		t.Fatal(err)
	}
	defer storage.Close()
	server := &APIServer{store: storage}

	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Set("agent_id", "agent-a")
	context.Request = httptest.NewRequest(http.MethodPost, "/todos", strings.NewReader(`{"title":"跟进回复","remind_at":"2026-09-28T12:00:00Z"}`))
	context.Request.Header.Set("Content-Type", "application/json")
	server.handleCreateTodo(context)
	if response.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", response.Code, response.Body.String())
	}
	if items, _ := storage.ListTodos("agent-b", ""); len(items) != 0 {
		t.Fatalf("cross-agent todos: %+v", items)
	}

	now := time.Now().UTC()
	pulse, _ := storage.CreatePulse(store.PulseRecord{AgentID: "agent-a", Reason: "profile_review", Prompt: "review", CreatedAt: now})
	settings, _ := storage.GetPulseSettings("agent-a")
	settings.ActivePulseID = pulse.ID
	storage.SavePulseSettings(settings)
	response = httptest.NewRecorder()
	context, _ = gin.CreateTestContext(response)
	context.Set("agent_id", "agent-a")
	context.Request = httptest.NewRequest(http.MethodPost, "/pulse/complete", strings.NewReader(`{"pulse_id":"`+pulse.ID+`","result":"无需修改","action_taken":false}`))
	context.Request.Header.Set("Content-Type", "application/json")
	server.handleCompletePulse(context)
	loaded, _ := storage.GetPulseSettings("agent-a")
	if response.Code != http.StatusOK || loaded.ActivePulseID != "" || loaded.LastProfileReviewAt.IsZero() {
		t.Fatalf("complete: %d %s settings=%+v", response.Code, response.Body.String(), loaded)
	}
}
