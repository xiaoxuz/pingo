package local

import (
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/pingo/sidecar/internal/session"
)

func (s *APIServer) handlePingoSessions(c *gin.Context) {
	okResponse(c, s.sessions.List(c.Query("agent_id")))
}

func (s *APIServer) handlePingoSessionStatus(c *gin.Context) {
	agentID := c.Query("agent_id")
	if !s.sessions.HasSession(c.Param("id"), agentID) {
		c.JSON(http.StatusNotFound, gin.H{"ok": false, "error": "session not found"})
		return
	}
	var report session.StatusReport
	if err := c.ShouldBindJSON(&report); err != nil {
		errResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	if !validSessionState(report.State) {
		errResponse(c, http.StatusBadRequest, "invalid session state")
		return
	}
	_, transitioned := s.sessions.Transition(c.Param("id"), agentID, report)
	if transitioned && c.Query("source") == "hook" {
		eventType := "runtime." + report.State
		s.sessions.PublishSession(c.Param("id"), agentID, session.Event{Type: eventType, Reason: report.Evidence, Message: report.Detail})
	}
	if transitioned {
		log.Printf("INFO: [session-state] session=%s agent=%s sequence=%d state=%s reason=%s evidence=%s detail=%q transition_at=%s",
			c.Param("id"), agentID, report.Sequence, report.State, report.Reason, report.Evidence, report.Detail, report.TransitionAt.Format(time.RFC3339Nano))
	}
	okResponse(c, gin.H{"updated": true})
}

func validSessionState(state string) bool {
	switch state {
	case "unknown", "busy", "free":
		return true
	default:
		return false
	}
}

func (s *APIServer) handlePingoPending(c *gin.Context) {
	agentID := c.Query("agent_id")
	if !s.sessions.HasSession(c.Param("id"), agentID) {
		c.JSON(http.StatusNotFound, gin.H{"ok": false, "error": "session not found"})
		return
	}
	unread, err := s.store.GetTotalUnread(agentID)
	if err != nil {
		errResponse(c, http.StatusInternalServerError, err.Error())
		return
	}
	requests, err := s.store.ListFriendRequests(agentID)
	if err != nil {
		errResponse(c, http.StatusInternalServerError, err.Error())
		return
	}
	okResponse(c, gin.H{"pending": unread > 0 || len(requests) > 0, "unread": unread, "friend_requests": len(requests)})
}
