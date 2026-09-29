package local

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/pingo/sidecar/internal/store"
)

func (s *APIServer) handleListTodos(c *gin.Context) {
	items, err := s.store.ListTodos(getAgentID(c), c.Query("status"))
	if err != nil {
		errResponse(c, http.StatusInternalServerError, err.Error())
		return
	}
	okResponse(c, items)
}

func (s *APIServer) handleCreateTodo(c *gin.Context) {
	var request struct {
		Title    string `json:"title"`
		Context  string `json:"context"`
		Priority string `json:"priority"`
		DueAt    string `json:"due_at"`
		RemindAt string `json:"remind_at"`
	}
	if err := c.ShouldBindJSON(&request); err != nil || strings.TrimSpace(request.Title) == "" {
		errResponse(c, http.StatusBadRequest, "title is required")
		return
	}
	dueAt, err := optionalTime(request.DueAt)
	if err != nil {
		errResponse(c, http.StatusBadRequest, "invalid due_at")
		return
	}
	remindAt, err := optionalTime(request.RemindAt)
	if err != nil {
		errResponse(c, http.StatusBadRequest, "invalid remind_at")
		return
	}
	item, err := s.store.CreateTodo(getAgentID(c), store.TodoRecord{Title: strings.TrimSpace(request.Title), Context: request.Context, Priority: request.Priority, DueAt: dueAt, RemindAt: remindAt})
	if err != nil {
		errResponse(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusCreated, gin.H{"ok": true, "data": item})
}

func (s *APIServer) handleCompleteTodo(c *gin.Context) {
	if err := s.store.CompleteTodo(getAgentID(c), c.Param("id"), time.Now().UTC()); err != nil {
		errResponse(c, http.StatusInternalServerError, err.Error())
		return
	}
	okResponse(c, gin.H{"ok": true})
}

func (s *APIServer) handleSnoozeTodo(c *gin.Context) {
	var request struct {
		RemindAt string `json:"remind_at"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		errResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	remindAt, err := optionalTime(request.RemindAt)
	if err != nil || remindAt.IsZero() {
		errResponse(c, http.StatusBadRequest, "valid remind_at is required")
		return
	}
	if err := s.store.SnoozeTodo(getAgentID(c), c.Param("id"), remindAt); err != nil {
		errResponse(c, http.StatusInternalServerError, err.Error())
		return
	}
	okResponse(c, gin.H{"ok": true})
}

func (s *APIServer) handleCancelTodo(c *gin.Context) {
	if err := s.store.CancelTodo(getAgentID(c), c.Param("id"), time.Now().UTC()); err != nil {
		errResponse(c, http.StatusInternalServerError, err.Error())
		return
	}
	okResponse(c, gin.H{"ok": true})
}

func (s *APIServer) handlePulseContext(c *gin.Context) {
	agentID := getAgentID(c)
	settings, err := s.store.GetPulseSettings(agentID)
	if err != nil {
		errResponse(c, 500, err.Error())
		return
	}
	todos, err := s.store.ListTodos(agentID, "pending")
	if err != nil {
		errResponse(c, 500, err.Error())
		return
	}
	history, err := s.store.ListPulses(agentID, 10)
	if err != nil {
		errResponse(c, 500, err.Error())
		return
	}
	okResponse(c, gin.H{"settings": settings, "todos": todos, "history": history})
}

func (s *APIServer) handleCompletePulse(c *gin.Context) {
	agentID := getAgentID(c)
	var request struct {
		PulseID     string `json:"pulse_id"`
		Result      string `json:"result"`
		ActionTaken bool   `json:"action_taken"`
	}
	if err := c.ShouldBindJSON(&request); err != nil || request.PulseID == "" {
		errResponse(c, 400, "pulse_id is required")
		return
	}
	settings, err := s.store.GetPulseSettings(agentID)
	if err != nil {
		errResponse(c, 500, err.Error())
		return
	}
	if settings.ActivePulseID != request.PulseID {
		errResponse(c, 409, "pulse is not active")
		return
	}
	now := time.Now().UTC()
	history, err := s.store.ListPulses(agentID, 20)
	if err != nil {
		errResponse(c, 500, err.Error())
		return
	}
	reason := ""
	for _, item := range history {
		if item.ID == request.PulseID {
			reason = item.Reason
			break
		}
	}
	if err := s.store.CompletePulse(agentID, request.PulseID, request.Result, request.ActionTaken, now); err != nil {
		errResponse(c, 500, err.Error())
		return
	}
	settings.ActivePulseID = ""
	if reason == "profile_review" {
		settings.LastProfileReviewAt = now
	}
	if reason == "social_review" {
		settings.LastSocialReviewAt = now
	}
	if err := s.store.SavePulseSettings(settings); err != nil {
		errResponse(c, 500, err.Error())
		return
	}
	okResponse(c, gin.H{"ok": true})
}

func (s *APIServer) handleGetPulseSettings(c *gin.Context) {
	settings, err := s.store.GetPulseSettings(getAgentID(c))
	if err != nil {
		errResponse(c, 500, err.Error())
		return
	}
	okResponse(c, settings)
}

func (s *APIServer) handleUpdatePulseSettings(c *gin.Context) {
	agentID := getAgentID(c)
	current, err := s.store.GetPulseSettings(agentID)
	if err != nil {
		errResponse(c, 500, err.Error())
		return
	}
	var request struct {
		Mode            string `json:"mode"`
		QuietStart      string `json:"quiet_start"`
		QuietEnd        string `json:"quiet_end"`
		CooldownMinutes int    `json:"cooldown_minutes"`
		DailyBudget     int    `json:"daily_budget"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		errResponse(c, 400, err.Error())
		return
	}
	if request.Mode != "off" && request.Mode != "restrained" && request.Mode != "balanced" && request.Mode != "active" {
		errResponse(c, 400, "invalid mode")
		return
	}
	if request.CooldownMinutes < 15 || request.DailyBudget < 0 {
		errResponse(c, 400, "invalid cooldown or daily budget")
		return
	}
	if _, ok := parseClock(request.QuietStart); !ok {
		errResponse(c, 400, "invalid quiet_start")
		return
	}
	if _, ok := parseClock(request.QuietEnd); !ok {
		errResponse(c, 400, "invalid quiet_end")
		return
	}
	current.Mode, current.QuietStart, current.QuietEnd = request.Mode, request.QuietStart, request.QuietEnd
	current.CooldownMinutes, current.DailyBudget = request.CooldownMinutes, request.DailyBudget
	if err := s.store.SavePulseSettings(current); err != nil {
		errResponse(c, 500, err.Error())
		return
	}
	okResponse(c, current)
}

func (s *APIServer) handlePulseHistory(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	items, err := s.store.ListPulses(getAgentID(c), limit)
	if err != nil {
		errResponse(c, 500, err.Error())
		return
	}
	okResponse(c, items)
}

func optionalTime(value string) (time.Time, error) {
	if strings.TrimSpace(value) == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, value)
}
func parseClock(value string) (int, bool) {
	parsed, err := time.Parse("15:04", value)
	if err != nil {
		return 0, false
	}
	return parsed.Hour()*60 + parsed.Minute(), true
}
