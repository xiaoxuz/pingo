package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

type SQLiteStore struct {
	db *sql.DB
}

func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	// 展开 ~
	if strings.HasPrefix(dbPath, "~") {
		home, _ := os.UserHomeDir()
		dbPath = filepath.Join(home, dbPath[1:])
	}

	// 确保目录存在
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	s := &SQLiteStore{db: db}
	if err := s.initTables(); err != nil {
		return nil, fmt.Errorf("init tables: %w", err)
	}

	return s, nil
}

func (s *SQLiteStore) Close() {
	s.db.Close()
}

func (s *SQLiteStore) initTables() error {
	tables := []string{
		`CREATE TABLE IF NOT EXISTS conversations (
			agent_id TEXT NOT NULL,
			conversation_id TEXT NOT NULL,
			type TEXT NOT NULL,
			name TEXT,
			status TEXT DEFAULT 'active',
			created_by TEXT,
			last_message_preview TEXT,
			last_message_at TEXT,
			unread_count INTEGER DEFAULT 0,
			created_at TEXT,
			closed_at TEXT,
			PRIMARY KEY (agent_id, conversation_id)
		)`,
		`CREATE TABLE IF NOT EXISTS messages (
			agent_id TEXT NOT NULL,
			message_id TEXT NOT NULL,
			conversation_id TEXT NOT NULL,
			from_agent TEXT NOT NULL,
			message_type TEXT NOT NULL,
			content_text TEXT,
			content_file TEXT,
			mentions TEXT,
			reply_to TEXT,
			system_event TEXT,
			created_at TEXT,
			PRIMARY KEY (agent_id, message_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_conv ON messages(agent_id, conversation_id, created_at)`,
		`CREATE TABLE IF NOT EXISTS friends (
			agent_id TEXT NOT NULL,
			friend_id TEXT NOT NULL,
			name TEXT,
			nickname TEXT,
			group_name TEXT,
			trust_level TEXT DEFAULT 'normal',
			status_text TEXT,
			online INTEGER DEFAULT 0,
			capabilities TEXT,
			PRIMARY KEY (agent_id, friend_id)
		)`,
		`CREATE TABLE IF NOT EXISTS friend_requests (
			agent_id TEXT NOT NULL,
			from_agent TEXT NOT NULL,
			name TEXT,
			message TEXT,
			created_at TEXT,
			PRIMARY KEY (agent_id, from_agent)
		)`,
		`CREATE TABLE IF NOT EXISTS conversation_members (
			agent_id TEXT NOT NULL,
			conversation_id TEXT NOT NULL,
			member_agent_id TEXT NOT NULL,
			role TEXT DEFAULT 'member',
			joined_at TEXT,
			left_at TEXT,
			PRIMARY KEY (agent_id, conversation_id, member_agent_id)
		)`,
		`CREATE TABLE IF NOT EXISTS approvals (
			id TEXT PRIMARY KEY,
			agent_id TEXT NOT NULL,
			type TEXT NOT NULL,
			title TEXT,
			question TEXT,
			options TEXT,
			context TEXT,
			urgency TEXT DEFAULT 'normal',
			status TEXT DEFAULT 'pending',
			decision TEXT,
			comment TEXT,
			created_at TEXT,
			decided_at TEXT,
			timeout_at TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_approvals_agent ON approvals(agent_id, status)`,
		`CREATE TABLE IF NOT EXISTS agent_todos (
			id TEXT PRIMARY KEY, agent_id TEXT NOT NULL, title TEXT NOT NULL, context TEXT,
			source_type TEXT, source_id TEXT, priority TEXT DEFAULT 'normal', status TEXT DEFAULT 'pending',
			due_at TEXT, remind_at TEXT, created_at TEXT NOT NULL, updated_at TEXT NOT NULL, completed_at TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_agent_todos_due ON agent_todos(agent_id, status, remind_at, due_at)`,
		`CREATE TABLE IF NOT EXISTS agent_pulse_settings (
			agent_id TEXT PRIMARY KEY, mode TEXT NOT NULL, quiet_start TEXT NOT NULL, quiet_end TEXT NOT NULL,
			cooldown_minutes INTEGER NOT NULL, daily_budget INTEGER NOT NULL, actions_today INTEGER NOT NULL,
			budget_date TEXT, last_pulse_at TEXT, last_profile_review_at TEXT, last_social_review_at TEXT,
			next_pulse_at TEXT, active_pulse_id TEXT, updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS agent_pulse_history (
			id TEXT PRIMARY KEY, agent_id TEXT NOT NULL, reason TEXT NOT NULL, prompt TEXT NOT NULL,
			status TEXT NOT NULL, session_id TEXT, result TEXT, action_taken INTEGER DEFAULT 0,
			created_at TEXT NOT NULL, delivered_at TEXT, completed_at TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_agent_pulse_history ON agent_pulse_history(agent_id, created_at DESC)`,
	}

	for _, table := range tables {
		if _, err := s.db.Exec(table); err != nil {
			return fmt.Errorf("create table: %w\nSQL: %s", err, table)
		}
	}

	return nil
}

// ============================================================
// Conversation methods
// ============================================================

func (s *SQLiteStore) UpsertConversation(agentID string, conv ConversationRecord) error {
	query := `
		INSERT INTO conversations (agent_id, conversation_id, type, name, status, created_by,
			last_message_preview, last_message_at, unread_count, created_at, closed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(agent_id, conversation_id) DO UPDATE SET
			name = excluded.name,
			status = excluded.status,
			last_message_preview = excluded.last_message_preview,
			last_message_at = excluded.last_message_at,
			unread_count = excluded.unread_count,
			closed_at = excluded.closed_at
	`
	_, err := s.db.Exec(query,
		agentID, conv.ConversationID, conv.Type, conv.Name, conv.Status,
		conv.CreatedBy, conv.LastMessagePreview, conv.LastMessageAt,
		conv.UnreadCount, conv.CreatedAt, conv.ClosedAt,
	)
	return err
}

func (s *SQLiteStore) ListConversations(agentID string) ([]ConversationRecord, error) {
	query := `
		SELECT conversation_id, type, name, status, created_by,
		       last_message_preview, last_message_at, unread_count, created_at, closed_at
		FROM conversations WHERE agent_id = ?
		ORDER BY COALESCE(last_message_at, created_at) DESC
	`
	rows, err := s.db.Query(query, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var convs []ConversationRecord
	for rows.Next() {
		var c ConversationRecord
		err := rows.Scan(&c.ConversationID, &c.Type, &c.Name, &c.Status, &c.CreatedBy,
			&c.LastMessagePreview, &c.LastMessageAt, &c.UnreadCount, &c.CreatedAt, &c.ClosedAt)
		if err != nil {
			return nil, err
		}
		convs = append(convs, c)
	}
	return convs, nil
}

func (s *SQLiteStore) GetConversation(agentID string, conversationID string) (*ConversationRecord, error) {
	query := `
		SELECT conversation_id, type, name, status, created_by,
		       last_message_preview, last_message_at, unread_count, created_at, closed_at
		FROM conversations WHERE agent_id = ? AND conversation_id = ?
	`
	var c ConversationRecord
	err := s.db.QueryRow(query, agentID, conversationID).Scan(
		&c.ConversationID, &c.Type, &c.Name, &c.Status, &c.CreatedBy,
		&c.LastMessagePreview, &c.LastMessageAt, &c.UnreadCount, &c.CreatedAt, &c.ClosedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &c, err
}

func (s *SQLiteStore) IncrementUnread(agentID string, conversationID string) error {
	query := `UPDATE conversations SET unread_count = unread_count + 1 WHERE agent_id = ? AND conversation_id = ?`
	_, err := s.db.Exec(query, agentID, conversationID)
	return err
}

func (s *SQLiteStore) MarkConversationRead(agentID string, conversationID string) error {
	query := `UPDATE conversations SET unread_count = 0 WHERE agent_id = ? AND conversation_id = ?`
	_, err := s.db.Exec(query, agentID, conversationID)
	return err
}

func (s *SQLiteStore) UpdateConversationLastMessage(agentID string, conversationID string, preview string, createdAt string) error {
	query := `UPDATE conversations SET last_message_preview = ?, last_message_at = ? WHERE agent_id = ? AND conversation_id = ?`
	_, err := s.db.Exec(query, preview, createdAt, agentID, conversationID)
	return err
}

func (s *SQLiteStore) GetTotalUnread(agentID string) (int, error) {
	var count int
	query := `SELECT COALESCE(SUM(unread_count), 0) FROM conversations WHERE agent_id = ?`
	err := s.db.QueryRow(query, agentID).Scan(&count)
	return count, err
}

// ============================================================
// Message methods
// ============================================================

func (s *SQLiteStore) SaveMessage(agentID string, msg MessageRecord) error {
	query := `
		INSERT OR IGNORE INTO messages (agent_id, message_id, conversation_id, from_agent,
			message_type, content_text, content_file, mentions, reply_to, system_event, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := s.db.Exec(query,
		agentID, msg.MessageID, msg.ConversationID, msg.FromAgent,
		msg.MessageType, msg.ContentText, msg.ContentFile,
		msg.Mentions, msg.ReplyTo, msg.SystemEvent, msg.CreatedAt,
	)
	return err
}

func (s *SQLiteStore) ListMessages(agentID string, conversationID string, limit int, afterMessageID string) ([]MessageRecord, error) {
	if limit <= 0 {
		limit = 50
	}

	var args []interface{}
	query := `
		SELECT message_id, conversation_id, from_agent, message_type,
		       content_text, content_file, mentions, reply_to, system_event, created_at
		FROM messages WHERE agent_id = ? AND conversation_id = ?
	`
	args = append(args, agentID, conversationID)

	if afterMessageID != "" {
		query += ` AND created_at > (SELECT created_at FROM messages WHERE agent_id = ? AND message_id = ?)`
		args = append(args, agentID, afterMessageID)
	}

	query += ` ORDER BY created_at ASC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []MessageRecord
	for rows.Next() {
		var m MessageRecord
		err := rows.Scan(&m.MessageID, &m.ConversationID, &m.FromAgent, &m.MessageType,
			&m.ContentText, &m.ContentFile, &m.Mentions, &m.ReplyTo, &m.SystemEvent, &m.CreatedAt)
		if err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	return msgs, nil
}

// ============================================================
// Friend methods
// ============================================================

func (s *SQLiteStore) UpsertFriend(agentID string, friend FriendRecord) error {
	query := `
		INSERT INTO friends (agent_id, friend_id, name, nickname, group_name, trust_level,
			status_text, online, capabilities)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(agent_id, friend_id) DO UPDATE SET
			name = excluded.name,
			nickname = excluded.nickname,
			group_name = excluded.group_name,
			trust_level = excluded.trust_level,
			status_text = excluded.status_text,
			online = excluded.online,
			capabilities = excluded.capabilities
	`
	_, err := s.db.Exec(query,
		agentID, friend.FriendID, friend.Name, friend.Nickname, friend.Group,
		friend.TrustLevel, friend.StatusText, boolToInt(friend.Online), friend.Capabilities,
	)
	return err
}

func (s *SQLiteStore) ListFriends(agentID string, group string, onlineOnly bool) ([]FriendRecord, error) {
	query := `SELECT friend_id, name, nickname, group_name, trust_level, status_text, online, capabilities FROM friends WHERE agent_id = ?`
	var args []interface{}
	args = append(args, agentID)

	if group != "" {
		query += ` AND group_name = ?`
		args = append(args, group)
	}
	if onlineOnly {
		query += ` AND online = 1`
	}
	query += ` ORDER BY online DESC, name ASC`

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var friends []FriendRecord
	for rows.Next() {
		var f FriendRecord
		var online int
		err := rows.Scan(&f.FriendID, &f.Name, &f.Nickname, &f.Group, &f.TrustLevel,
			&f.StatusText, &online, &f.Capabilities)
		if err != nil {
			return nil, err
		}
		f.Online = online == 1
		friends = append(friends, f)
	}
	return friends, nil
}

func (s *SQLiteStore) GetFriend(agentID string, friendID string) (*FriendRecord, error) {
	query := `SELECT friend_id, name, nickname, group_name, trust_level, status_text, online, capabilities FROM friends WHERE agent_id = ? AND friend_id = ?`
	var f FriendRecord
	var online int
	err := s.db.QueryRow(query, agentID, friendID).Scan(
		&f.FriendID, &f.Name, &f.Nickname, &f.Group, &f.TrustLevel,
		&f.StatusText, &online, &f.Capabilities,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	f.Online = online == 1
	return &f, err
}

func (s *SQLiteStore) UpsertFriendRequest(agentID string, req FriendRequestRecord) error {
	query := `
		INSERT OR REPLACE INTO friend_requests (agent_id, from_agent, name, message, created_at)
		VALUES (?, ?, ?, ?, ?)
	`
	_, err := s.db.Exec(query, agentID, req.FromAgent, req.Name, req.Message, req.CreatedAt)
	return err
}

func (s *SQLiteStore) ListFriendRequests(agentID string) ([]FriendRequestRecord, error) {
	query := `SELECT from_agent, name, message, created_at FROM friend_requests WHERE agent_id = ? ORDER BY created_at DESC`
	rows, err := s.db.Query(query, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reqs []FriendRequestRecord
	for rows.Next() {
		var r FriendRequestRecord
		err := rows.Scan(&r.FromAgent, &r.Name, &r.Message, &r.CreatedAt)
		if err != nil {
			return nil, err
		}
		reqs = append(reqs, r)
	}
	return reqs, nil
}

func (s *SQLiteStore) RemoveFriendRequest(agentID string, fromAgent string) error {
	query := `DELETE FROM friend_requests WHERE agent_id = ? AND from_agent = ?`
	_, err := s.db.Exec(query, agentID, fromAgent)
	return err
}

// ============================================================
// Conversation member methods
// ============================================================

func (s *SQLiteStore) UpsertMember(agentID string, conversationID string, member MemberRecord) error {
	query := `
		INSERT OR REPLACE INTO conversation_members (agent_id, conversation_id, member_agent_id, role, joined_at, left_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`
	_, err := s.db.Exec(query, agentID, conversationID, member.MemberAgentID, member.Role, member.JoinedAt, member.LeftAt)
	return err
}

func (s *SQLiteStore) ListMembers(agentID string, conversationID string) ([]MemberRecord, error) {
	query := `SELECT member_agent_id, role, joined_at, left_at FROM conversation_members WHERE agent_id = ? AND conversation_id = ?`
	rows, err := s.db.Query(query, agentID, conversationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []MemberRecord
	for rows.Next() {
		var m MemberRecord
		err := rows.Scan(&m.MemberAgentID, &m.Role, &m.JoinedAt, &m.LeftAt)
		if err != nil {
			return nil, err
		}
		members = append(members, m)
	}
	return members, nil
}

// ============================================================
// Approval methods
// ============================================================

func (s *SQLiteStore) CreateApproval(approval ApprovalRecord) error {
	query := `
		INSERT INTO approvals (id, agent_id, type, title, question, options, context,
			urgency, status, created_at, timeout_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := s.db.Exec(query,
		approval.ID, approval.AgentID, approval.Type, approval.Title, approval.Question,
		approval.Options, approval.Context, approval.Urgency, approval.Status,
		approval.CreatedAt, approval.TimeoutAt,
	)
	return err
}

func (s *SQLiteStore) GetApproval(id string) (*ApprovalRecord, error) {
	query := `
		SELECT id, agent_id, type, title, question, options, context,
		       urgency, status, decision, comment, created_at, decided_at, timeout_at
		FROM approvals WHERE id = ?
	`
	var a ApprovalRecord
	err := s.db.QueryRow(query, id).Scan(
		&a.ID, &a.AgentID, &a.Type, &a.Title, &a.Question, &a.Options, &a.Context,
		&a.Urgency, &a.Status, &a.Decision, &a.Comment, &a.CreatedAt, &a.DecidedAt, &a.TimeoutAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &a, err
}

func (s *SQLiteStore) ListPendingApprovals() ([]ApprovalRecord, error) {
	query := `
		SELECT id, agent_id, type, title, question, options, context,
		       urgency, status, decision, comment, created_at, decided_at, timeout_at
		FROM approvals WHERE status = 'pending' ORDER BY created_at DESC
	`
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var approvals []ApprovalRecord
	for rows.Next() {
		var a ApprovalRecord
		err := rows.Scan(
			&a.ID, &a.AgentID, &a.Type, &a.Title, &a.Question, &a.Options, &a.Context,
			&a.Urgency, &a.Status, &a.Decision, &a.Comment, &a.CreatedAt, &a.DecidedAt, &a.TimeoutAt,
		)
		if err != nil {
			return nil, err
		}
		approvals = append(approvals, a)
	}
	return approvals, nil
}

func (s *SQLiteStore) DecideApproval(id string, decision string, comment string) error {
	query := `
		UPDATE approvals SET status = 'decided', decision = ?, comment = ?, decided_at = ?
		WHERE id = ? AND status = 'pending'
	`
	_, err := s.db.Exec(query, decision, comment, time.Now().Format(time.RFC3339), id)
	return err
}

// ============================================================
// Agent todo and pulse methods
// ============================================================

func (s *SQLiteStore) CreateTodo(agentID string, todo TodoRecord) (TodoRecord, error) {
	now := time.Now().UTC()
	if todo.ID == "" {
		todo.ID = "todo_" + uuid.NewString()
	}
	if todo.Priority == "" {
		todo.Priority = "normal"
	}
	if todo.Status == "" {
		todo.Status = "pending"
	}
	if todo.CreatedAt.IsZero() {
		todo.CreatedAt = now
	}
	todo.UpdatedAt = now
	todo.AgentID = agentID
	_, err := s.db.Exec(`INSERT INTO agent_todos
		(id,agent_id,title,context,source_type,source_id,priority,status,due_at,remind_at,created_at,updated_at,completed_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`, todo.ID, agentID, todo.Title, todo.Context, todo.SourceType, todo.SourceID,
		todo.Priority, todo.Status, timeText(todo.DueAt), timeText(todo.RemindAt), timeText(todo.CreatedAt), timeText(todo.UpdatedAt), timeText(todo.CompletedAt))
	return todo, err
}

func (s *SQLiteStore) ListTodos(agentID, status string) ([]TodoRecord, error) {
	query := `SELECT id,agent_id,title,context,source_type,source_id,priority,status,due_at,remind_at,created_at,updated_at,completed_at FROM agent_todos WHERE agent_id=?`
	args := []any{agentID}
	if status != "" {
		query += ` AND status=?`
		args = append(args, status)
	}
	query += ` ORDER BY CASE priority WHEN 'urgent' THEN 0 WHEN 'high' THEN 1 WHEN 'normal' THEN 2 ELSE 3 END, COALESCE(remind_at,due_at,created_at)`
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []TodoRecord
	for rows.Next() {
		todo, err := scanTodo(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, todo)
	}
	return result, rows.Err()
}

func (s *SQLiteStore) ListDueTodos(agentID string, now time.Time) ([]TodoRecord, error) {
	rows, err := s.db.Query(`SELECT id,agent_id,title,context,source_type,source_id,priority,status,due_at,remind_at,created_at,updated_at,completed_at
		FROM agent_todos WHERE agent_id=? AND status='pending' AND ((remind_at!='' AND remind_at<=?) OR (remind_at='' AND due_at!='' AND due_at<=?)) ORDER BY COALESCE(remind_at,due_at)`, agentID, timeText(now), timeText(now))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []TodoRecord
	for rows.Next() {
		todo, err := scanTodo(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, todo)
	}
	return result, rows.Err()
}

func (s *SQLiteStore) CompleteTodo(agentID, id string, at time.Time) error {
	_, err := s.db.Exec(`UPDATE agent_todos SET status='completed',completed_at=?,updated_at=? WHERE agent_id=? AND id=?`, timeText(at), timeText(at), agentID, id)
	return err
}

func (s *SQLiteStore) SnoozeTodo(agentID, id string, remindAt time.Time) error {
	_, err := s.db.Exec(`UPDATE agent_todos SET remind_at=?,updated_at=? WHERE agent_id=? AND id=? AND status='pending'`, timeText(remindAt), timeText(time.Now().UTC()), agentID, id)
	return err
}

func (s *SQLiteStore) CancelTodo(agentID, id string, at time.Time) error {
	_, err := s.db.Exec(`UPDATE agent_todos SET status='cancelled',updated_at=? WHERE agent_id=? AND id=?`, timeText(at), agentID, id)
	return err
}

func (s *SQLiteStore) GetPulseSettings(agentID string) (PulseSettings, error) {
	settings := PulseSettings{AgentID: agentID, Mode: "balanced", QuietStart: "23:00", QuietEnd: "08:00", CooldownMinutes: 120, DailyBudget: 1}
	var budgetDate, lastPulse, profileReview, socialReview, nextPulse, activePulse, updated string
	err := s.db.QueryRow(`SELECT mode,quiet_start,quiet_end,cooldown_minutes,daily_budget,actions_today,COALESCE(budget_date,''),COALESCE(last_pulse_at,''),COALESCE(last_profile_review_at,''),COALESCE(last_social_review_at,''),COALESCE(next_pulse_at,''),COALESCE(active_pulse_id,''),updated_at FROM agent_pulse_settings WHERE agent_id=?`, agentID).Scan(
		&settings.Mode, &settings.QuietStart, &settings.QuietEnd, &settings.CooldownMinutes, &settings.DailyBudget, &settings.ActionsToday, &budgetDate, &lastPulse, &profileReview, &socialReview, &nextPulse, &activePulse, &updated)
	if err == sql.ErrNoRows {
		return settings, nil
	}
	if err != nil {
		return settings, err
	}
	settings.BudgetDate, settings.LastPulseAt, settings.LastProfileReviewAt, settings.LastSocialReviewAt = budgetDate, parseTime(lastPulse), parseTime(profileReview), parseTime(socialReview)
	settings.NextPulseAt, settings.ActivePulseID, settings.UpdatedAt = parseTime(nextPulse), activePulse, parseTime(updated)
	return settings, nil
}

func (s *SQLiteStore) SavePulseSettings(settings PulseSettings) error {
	if settings.Mode == "" {
		settings.Mode = "balanced"
	}
	if settings.QuietStart == "" {
		settings.QuietStart = "23:00"
	}
	if settings.QuietEnd == "" {
		settings.QuietEnd = "08:00"
	}
	if settings.CooldownMinutes <= 0 {
		settings.CooldownMinutes = 120
	}
	if settings.DailyBudget < 0 {
		settings.DailyBudget = 0
	}
	settings.UpdatedAt = time.Now().UTC()
	_, err := s.db.Exec(`INSERT INTO agent_pulse_settings(agent_id,mode,quiet_start,quiet_end,cooldown_minutes,daily_budget,actions_today,budget_date,last_pulse_at,last_profile_review_at,last_social_review_at,next_pulse_at,active_pulse_id,updated_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(agent_id) DO UPDATE SET mode=excluded.mode,quiet_start=excluded.quiet_start,quiet_end=excluded.quiet_end,cooldown_minutes=excluded.cooldown_minutes,daily_budget=excluded.daily_budget,actions_today=excluded.actions_today,budget_date=excluded.budget_date,last_pulse_at=excluded.last_pulse_at,last_profile_review_at=excluded.last_profile_review_at,last_social_review_at=excluded.last_social_review_at,next_pulse_at=excluded.next_pulse_at,active_pulse_id=excluded.active_pulse_id,updated_at=excluded.updated_at`,
		settings.AgentID, settings.Mode, settings.QuietStart, settings.QuietEnd, settings.CooldownMinutes, settings.DailyBudget, settings.ActionsToday, settings.BudgetDate, timeText(settings.LastPulseAt), timeText(settings.LastProfileReviewAt), timeText(settings.LastSocialReviewAt), timeText(settings.NextPulseAt), settings.ActivePulseID, timeText(settings.UpdatedAt))
	return err
}

func (s *SQLiteStore) CreatePulse(pulse PulseRecord) (PulseRecord, error) {
	if pulse.ID == "" {
		pulse.ID = "pulse_" + uuid.NewString()
	}
	if pulse.Status == "" {
		pulse.Status = "pending"
	}
	if pulse.CreatedAt.IsZero() {
		pulse.CreatedAt = time.Now().UTC()
	}
	_, err := s.db.Exec(`INSERT INTO agent_pulse_history(id,agent_id,reason,prompt,status,session_id,result,action_taken,created_at,delivered_at,completed_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, pulse.ID, pulse.AgentID, pulse.Reason, pulse.Prompt, pulse.Status, pulse.SessionID, pulse.Result, boolToInt(pulse.ActionTaken), timeText(pulse.CreatedAt), timeText(pulse.DeliveredAt), timeText(pulse.CompletedAt))
	return pulse, err
}

func (s *SQLiteStore) MarkPulseDelivered(agentID, id, sessionID string, at time.Time) error {
	_, err := s.db.Exec(`UPDATE agent_pulse_history SET status='delivered',session_id=?,delivered_at=? WHERE agent_id=? AND id=?`, sessionID, timeText(at), agentID, id)
	return err
}

func (s *SQLiteStore) CompletePulse(agentID, id, result string, actionTaken bool, at time.Time) error {
	_, err := s.db.Exec(`UPDATE agent_pulse_history SET status='completed',result=?,action_taken=?,completed_at=? WHERE agent_id=? AND id=?`, result, boolToInt(actionTaken), timeText(at), agentID, id)
	return err
}

func (s *SQLiteStore) ExpirePulse(agentID, id string, at time.Time) error {
	_, err := s.db.Exec(`UPDATE agent_pulse_history SET status='expired',result='Agent 未在时限内提交回执',completed_at=? WHERE agent_id=? AND id=? AND status!='completed'`, timeText(at), agentID, id)
	return err
}

func (s *SQLiteStore) ListPulses(agentID string, limit int) ([]PulseRecord, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.Query(`SELECT id,agent_id,reason,prompt,status,COALESCE(session_id,''),COALESCE(result,''),action_taken,created_at,COALESCE(delivered_at,''),COALESCE(completed_at,'') FROM agent_pulse_history WHERE agent_id=? ORDER BY created_at DESC LIMIT ?`, agentID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []PulseRecord
	for rows.Next() {
		var p PulseRecord
		var action int
		var created, delivered, completed string
		if err := rows.Scan(&p.ID, &p.AgentID, &p.Reason, &p.Prompt, &p.Status, &p.SessionID, &p.Result, &action, &created, &delivered, &completed); err != nil {
			return nil, err
		}
		p.ActionTaken = action == 1
		p.CreatedAt = parseTime(created)
		p.DeliveredAt = parseTime(delivered)
		p.CompletedAt = parseTime(completed)
		result = append(result, p)
	}
	return result, rows.Err()
}

type rowScanner interface{ Scan(...any) error }

func scanTodo(row rowScanner) (TodoRecord, error) {
	var todo TodoRecord
	var due, remind, created, updated, completed string
	err := row.Scan(&todo.ID, &todo.AgentID, &todo.Title, &todo.Context, &todo.SourceType, &todo.SourceID, &todo.Priority, &todo.Status, &due, &remind, &created, &updated, &completed)
	todo.DueAt = parseTime(due)
	todo.RemindAt = parseTime(remind)
	todo.CreatedAt = parseTime(created)
	todo.UpdatedAt = parseTime(updated)
	todo.CompletedAt = parseTime(completed)
	return todo, err
}
func timeText(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}
func parseTime(value string) time.Time {
	parsed, _ := time.Parse(time.RFC3339Nano, value)
	return parsed
}

// ============================================================
// Record types
// ============================================================

type ConversationRecord struct {
	ConversationID     string
	Type               string
	Name               string
	Status             string
	CreatedBy          string
	LastMessagePreview string
	LastMessageAt      string
	UnreadCount        int
	CreatedAt          string
	ClosedAt           string
	Members            []string
}

type MessageRecord struct {
	MessageID      string
	ConversationID string
	FromAgent      string
	MessageType    string
	ContentText    string
	ContentFile    string
	Mentions       string
	ReplyTo        string
	SystemEvent    string
	CreatedAt      string
}

type FriendRecord struct {
	FriendID     string
	Name         string
	Nickname     string
	Group        string
	TrustLevel   string
	StatusText   string
	Online       bool
	Capabilities string
}

type FriendRequestRecord struct {
	FromAgent string
	Name      string
	Message   string
	CreatedAt string
}

type MemberRecord struct {
	MemberAgentID string
	Role          string
	JoinedAt      string
	LeftAt        string
}

type ApprovalRecord struct {
	ID        string
	AgentID   string
	Type      string
	Title     string
	Question  string
	Options   string
	Context   string
	Urgency   string
	Status    string
	Decision  string
	Comment   string
	CreatedAt string
	DecidedAt string
	TimeoutAt string
}

type TodoRecord struct {
	ID          string    `json:"id"`
	AgentID     string    `json:"agent_id"`
	Title       string    `json:"title"`
	Context     string    `json:"context,omitempty"`
	SourceType  string    `json:"source_type,omitempty"`
	SourceID    string    `json:"source_id,omitempty"`
	Priority    string    `json:"priority"`
	Status      string    `json:"status"`
	DueAt       time.Time `json:"due_at,omitempty"`
	RemindAt    time.Time `json:"remind_at,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	CompletedAt time.Time `json:"completed_at,omitempty"`
}

type PulseSettings struct {
	AgentID             string    `json:"agent_id"`
	Mode                string    `json:"mode"`
	QuietStart          string    `json:"quiet_start"`
	QuietEnd            string    `json:"quiet_end"`
	BudgetDate          string    `json:"budget_date,omitempty"`
	ActivePulseID       string    `json:"active_pulse_id,omitempty"`
	CooldownMinutes     int       `json:"cooldown_minutes"`
	DailyBudget         int       `json:"daily_budget"`
	ActionsToday        int       `json:"actions_today"`
	LastPulseAt         time.Time `json:"last_pulse_at,omitempty"`
	LastProfileReviewAt time.Time `json:"last_profile_review_at,omitempty"`
	LastSocialReviewAt  time.Time `json:"last_social_review_at,omitempty"`
	NextPulseAt         time.Time `json:"next_pulse_at,omitempty"`
	UpdatedAt           time.Time `json:"updated_at,omitempty"`
}

type PulseRecord struct {
	ID          string    `json:"id"`
	AgentID     string    `json:"agent_id"`
	Reason      string    `json:"reason"`
	Prompt      string    `json:"prompt"`
	Status      string    `json:"status"`
	SessionID   string    `json:"session_id,omitempty"`
	Result      string    `json:"result,omitempty"`
	ActionTaken bool      `json:"action_taken"`
	CreatedAt   time.Time `json:"created_at"`
	DeliveredAt time.Time `json:"delivered_at,omitempty"`
	CompletedAt time.Time `json:"completed_at,omitempty"`
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
