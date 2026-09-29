package operationlog

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestMiddlewareRedactsInputAndClassifies(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var output bytes.Buffer
	logger := New(&output)
	router := gin.New()
	router.Use(logger.Middleware())
	router.POST("/conversations/:id/send", func(ctx *gin.Context) { ctx.Status(http.StatusAccepted) })
	request := httptest.NewRequest(http.MethodPost, "/conversations/private/send?token=secret", strings.NewReader(`{"body":"private message"}`))
	request.Header.Set("X-Agent-ID", "agent-a")
	request.Header.Set("Authorization", "Bearer secret")
	router.ServeHTTP(httptest.NewRecorder(), request)
	if strings.Contains(output.String(), "secret") || strings.Contains(output.String(), "private") {
		t.Fatal("sensitive input leaked")
	}
	var entry map[string]any
	if err := json.Unmarshal(output.Bytes(), &entry); err != nil {
		t.Fatal(err)
	}
	if entry["category"] != "message" || entry["action"] != "POST /conversations/:id/send" || entry["status"] != float64(202) || entry["agent_id"] != "agent-a" {
		t.Fatalf("wrong entry: %v", entry)
	}
}

func TestCategoryClassifiesAutonomousActivity(t *testing.T) {
	for _, path := range []string{"/pulse/context", "/pulse/settings", "/todos", "/todos/:id/complete"} {
		if got := Category(path); got != "pulse" {
			t.Fatalf("Category(%q) = %q", path, got)
		}
	}
}
