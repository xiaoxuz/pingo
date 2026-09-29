package auth

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

type AgentStore interface {
	GetAgentIDByToken(token string) (string, error)
}

// HTTPAuthMiddleware HTTP 认证中间件
func HTTPAuthMiddleware(store AgentStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"ok":    false,
				"error": "missing authorization header",
			})
			c.Abort()
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			c.JSON(http.StatusUnauthorized, gin.H{
				"ok":    false,
				"error": "invalid authorization format",
			})
			c.Abort()
			return
		}

		token := parts[1]
		agentID, err := store.GetAgentIDByToken(token)
		if err != nil || agentID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"ok":    false,
				"error": "invalid token",
			})
			c.Abort()
			return
		}

		c.Set("agent_id", agentID)
		c.Next()
	}
}

// GetAgentID 从 context 中获取 agent_id
func GetAgentID(c *gin.Context) string {
	v, exists := c.Get("agent_id")
	if !exists {
		return ""
	}
	id, ok := v.(string)
	if !ok {
		return ""
	}
	return id
}
