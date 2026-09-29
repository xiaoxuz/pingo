package local

import (
	"bytes"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
)

func (s *APIServer) handleAdminStats(c *gin.Context) {
	s.proxyHubJSON(c, http.MethodGet, "/api/admin/stats", nil)
}

func (s *APIServer) handleAdminAgents(c *gin.Context) {
	s.proxyHubJSON(c, http.MethodGet, "/api/admin/agents", nil)
}

func (s *APIServer) handleAdminResetToken(c *gin.Context) {
	s.proxyHubJSON(c, http.MethodPost, "/api/admin/agents/"+c.Param("id")+"/reset-token", nil)
}

func (s *APIServer) proxyHubJSON(c *gin.Context, method string, path string, body []byte) {
	req, err := http.NewRequest(method, s.config.Hub.HTTPEndpoint()+path, bytes.NewReader(body))
	if err != nil {
		errResponse(c, http.StatusInternalServerError, "create hub request failed: "+err.Error())
		return
	}
	req.Header.Set("Accept", "application/json")
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		errResponse(c, http.StatusBadGateway, "hub request failed: "+err.Error())
		return
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		errResponse(c, http.StatusBadGateway, "read hub response failed: "+err.Error())
		return
	}
	c.Data(resp.StatusCode, "application/json; charset=utf-8", data)
}
