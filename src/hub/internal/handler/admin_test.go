package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestAdminRejectsMissingSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/admin/stats", AdminSessionMiddleware(nil), func(c *gin.Context) { c.Status(http.StatusOK) })
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/admin/stats", nil))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("admin route without session: %d", response.Code)
	}
}

func TestAdminOriginRequiresSameHostAndScheme(t *testing.T) {
	for _, test := range []struct {
		origin   string
		expected bool
	}{
		{"https://hub.example.com", true},
		{"https://attacker.example.com", false},
		{"http://hub.example.com", false},
		{"https://hub.example.com.attacker.test", false},
		{"", false},
	} {
		request := httptest.NewRequest(http.MethodPost, "https://hub.example.com/api/admin/login", nil)
		request.Header.Set("Origin", test.origin)
		if got := sameAdminOrigin(request); got != test.expected {
			t.Errorf("origin %q: got %v, want %v", test.origin, got, test.expected)
		}
	}
}
