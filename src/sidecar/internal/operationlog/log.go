package operationlog

import (
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type Logger struct{ log *slog.Logger }

func New(output io.Writer) *Logger {
	return &Logger{log: slog.New(slog.NewJSONHandler(output, nil))}
}

func (logger *Logger) Event(category, action string, attributes ...any) {
	logger.log.Info("sidecar_operation", append([]any{"category", category, "action", action}, attributes...)...)
}

func Category(path string) string {
	switch {
	case strings.HasPrefix(path, "/update/"):
		return "update"
	case strings.HasPrefix(path, "/agents/"):
		return "identity"
	case strings.HasPrefix(path, "/pingo/sessions"):
		return "session"
	case strings.HasPrefix(path, "/conversations"), strings.HasPrefix(path, "/inbox"):
		return "message"
	case strings.HasPrefix(path, "/friends"), strings.HasPrefix(path, "/discover"):
		return "social"
	case strings.HasPrefix(path, "/files"):
		return "file"
	case strings.HasPrefix(path, "/approvals"):
		return "approval"
	case strings.HasPrefix(path, "/admin"):
		return "admin"
	case strings.HasPrefix(path, "/me"):
		return "profile"
	case strings.HasPrefix(path, "/pulse"), strings.HasPrefix(path, "/todos"):
		return "pulse"
	default:
		return "system"
	}
}

func (logger *Logger) Middleware() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		start := time.Now()
		ctx.Next()
		path := ctx.FullPath()
		if path == "" {
			path = "unmatched"
		}
		logger.Event(Category(path), ctx.Request.Method+" "+path,
			"status", ctx.Writer.Status(), "duration_ms", time.Since(start).Milliseconds(),
			"agent_id", ctx.GetHeader("X-Agent-ID"))
	}
}
