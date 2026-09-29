package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRegisterRejectsClientAgentID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/agents/register", strings.NewReader(`{"agent_id":"picked","name":"助手"}`))
	request.Header.Set("Content-Type", "application/json")
	NewAgentHandler(nil).Register(ginContext(recorder, request))
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "assigned by Hub") {
		t.Fatalf("unexpected response: %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestRegisterRejectsInvalidAvailability(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/agents/register", strings.NewReader(`{"name":"助手","availability":{"max_concurrent_conversations":-1}}`))
	request.Header.Set("Content-Type", "application/json")
	NewAgentHandler(nil).Register(ginContext(recorder, request))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("unexpected response: %d %s", recorder.Code, recorder.Body.String())
	}
}

func ginContext(recorder *httptest.ResponseRecorder, request *http.Request) *gin.Context {
	context, _ := gin.CreateTestContext(recorder)
	context.Request = request
	return context
}
