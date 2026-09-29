package local

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/pingo/sidecar/internal/config"
	"github.com/pingo/sidecar/internal/session"
	"github.com/pingo/sidecar/internal/store"
)

func TestPendingWakeRequiresMatchingSessionAndAgent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	storage, err := store.NewSQLiteStore(t.TempDir() + "/pingo.db")
	if err != nil {
		t.Fatal(err)
	}
	defer storage.Close()
	if err := storage.UpsertFriendRequest("agent-b", store.FriendRequestRecord{FromAgent: "agent-a", Name: "A", Message: "hi", CreatedAt: "now"}); err != nil {
		t.Fatal(err)
	}
	registry := session.NewRegistry()
	registry.Register(session.RegisterRequest{ID: "session-b", AgentID: "agent-b", Provider: "claude"})
	server := &APIServer{store: storage, sessions: registry}
	for _, test := range []struct {
		name, agentID string
		status        int
		pending       bool
	}{
		{"correct", "agent-b", 200, true},
		{"wrong agent", "agent-a", 404, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(response)
			context.Params = gin.Params{{Key: "id", Value: "session-b"}}
			context.Request = httptest.NewRequest(http.MethodGet, "/pingo/sessions/session-b/pending?agent_id="+test.agentID, nil)
			server.handlePingoPending(context)
			if response.Code != test.status {
				t.Fatalf("status = %d: %s", response.Code, response.Body.String())
			}
			if test.status == 200 {
				var result struct {
					Data struct {
						Pending bool `json:"pending"`
					} `json:"data"`
				}
				if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || result.Data.Pending != test.pending {
					t.Fatalf("response = %s, %v", response.Body.String(), err)
				}
			}
		})
	}
}

func TestSessionStatusEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	registry := session.NewRegistry()
	registry.Register(session.RegisterRequest{ID: "window", AgentID: "agent-b", Provider: "claude"})
	server := &APIServer{sessions: registry}
	for _, test := range []struct {
		identity string
		want     int
	}{
		{"agent-b", http.StatusOK},
		{"agent-a", http.StatusNotFound},
	} {
		response := httptest.NewRecorder()
		context, _ := gin.CreateTestContext(response)
		context.Params = gin.Params{{Key: "id", Value: "window"}}
		context.Request = httptest.NewRequest(http.MethodPut, "/pingo/sessions/window/status?agent_id="+test.identity, strings.NewReader(`{"state":"busy","reason":"interaction_required","pending":true}`))
		server.handlePingoSessionStatus(context)
		if response.Code != test.want {
			t.Fatalf("status = %d, want %d: %s", response.Code, test.want, response.Body.String())
		}
	}
	if got := registry.List("agent-b")[0].Status.State; got != "busy" {
		t.Fatalf("stored status = %q", got)
	}
}

func TestHookStatusPublishesRuntimeEventToSameSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	registry := session.NewRegistry()
	registry.Register(session.RegisterRequest{ID: "window", AgentID: "agent-b", Provider: "claude"})
	server := &APIServer{sessions: registry}
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Params = gin.Params{{Key: "id", Value: "window"}}
	context.Request = httptest.NewRequest(http.MethodPut, "/pingo/sessions/window/status?agent_id=agent-b&source=hook", strings.NewReader(`{"state":"busy","evidence":"hook_user_prompt_submit","detail":"Claude UserPromptSubmit Hook"}`))
	server.handlePingoSessionStatus(context)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", response.Code, response.Body.String())
	}
	select {
	case event := <-registry.Subscribe("window"):
		if event.Type != "runtime.busy" || event.Reason != "hook_user_prompt_submit" || event.Message != "Claude UserPromptSubmit Hook" {
			t.Fatalf("runtime event = %+v", event)
		}
	default:
		t.Fatal("expected runtime event")
	}
}

func TestFreeSessionStateAccepted(t *testing.T) {
	if !validSessionState("free") || !validSessionState("busy") || !validSessionState("unknown") || validSessionState("waiting") || validSessionState("typing") {
		t.Fatal("only free, busy and unknown must be accepted")
	}
}

func TestAgentProfileProxyStripsToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	hub := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/agents/me" || request.Header.Get("Authorization") != "Bearer secret" {
			t.Errorf("unexpected hub request: %s", request.URL.String())
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.Write([]byte(`{"ok":true,"data":{"agent_id":"agent-b","name":"B","token":"secret","capabilities":[],"availability":{}}}`))
	}))
	defer hub.Close()
	server := &APIServer{config: &config.Config{Hub: config.HubConfig{Endpoint: strings.Replace(hub.URL, "http://", "ws://", 1) + "/ws"}, Agents: []config.AgentConfig{{ID: "agent-b", Token: "secret"}}}, sessions: session.NewRegistry()}
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Set("agent_id", "agent-b")
	context.Request = httptest.NewRequest(http.MethodGet, "/me", nil)
	server.handleGetMe(context)
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "secret") {
		t.Fatalf("profile leaked token or failed: %d %s", response.Code, response.Body.String())
	}
}
