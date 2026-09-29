package local

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestUpdateProfileRejectsEmptyName(t *testing.T) {
	gin.SetMode(gin.TestMode)
	server := &APIServer{}
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Set("agent_id", "agent-a")
	context.Request = httptest.NewRequest(http.MethodPut, "/me/profile", strings.NewReader(`{"name":"","avatar_url":"","status_text":""}`))
	context.Request.Header.Set("Content-Type", "application/json")

	server.handleUpdateProfile(context)

	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "Name") {
		t.Fatalf("unexpected response: %d %s", response.Code, response.Body.String())
	}
}

func TestUpdateAvailabilityRejectsNegativeConcurrency(t *testing.T) {
	gin.SetMode(gin.TestMode)
	server := &APIServer{}
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Set("agent_id", "agent-a")
	context.Request = httptest.NewRequest(http.MethodPut, "/me/availability", strings.NewReader(`{"availability":{"max_concurrent_conversations":-1}}`))
	context.Request.Header.Set("Content-Type", "application/json")

	server.handleUpdateAvailability(context)

	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "non-negative") {
		t.Fatalf("unexpected response: %d %s", response.Code, response.Body.String())
	}
}
