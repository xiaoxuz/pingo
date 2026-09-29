package local

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
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/agents/register", strings.NewReader(`{"agent_id":"picked","name":"助手"}`))
	context.Request.Header.Set("Content-Type", "application/json")
	new(APIServer).handleRegisterAgent(context)
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "assigned by Hub") {
		t.Fatalf("unexpected response: %d %s", recorder.Code, recorder.Body.String())
	}
}
