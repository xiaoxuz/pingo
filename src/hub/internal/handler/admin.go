package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pingo/hub/internal/filestore"
	"golang.org/x/crypto/bcrypt"
)

type adminSession struct {
	username string
	expires  time.Time
}

type AdminHandler struct {
	db         *pgxpool.Pool
	files      filestore.FileStore
	mu         sync.Mutex
	sessions   map[string]adminSession
	failures   map[string][]time.Time
	disconnect func(string)
}

func NewAdminHandler(db *pgxpool.Pool, files filestore.FileStore) *AdminHandler {
	return &AdminHandler{db: db, files: files, sessions: make(map[string]adminSession), failures: make(map[string][]time.Time)}
}

func (h *AdminHandler) WithDisconnect(disconnect func(string)) *AdminHandler {
	h.disconnect = disconnect
	return h
}

func (h *AdminHandler) Bootstrap(ctx context.Context) error {
	password := os.Getenv("PINGO_ADMIN_INITIAL_PASSWORD")
	if password == "" {
		password = "123456"
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = h.db.Exec(ctx, `INSERT INTO admin_users (username,password_hash,must_change_password) VALUES ('pingo',$1,TRUE) ON CONFLICT (username) DO NOTHING`, string(hash))
	return err
}

func newSessionToken() (string, error) {
	data := make([]byte, 32)
	_, err := rand.Read(data)
	return hex.EncodeToString(data), err
}

func (h *AdminHandler) Login(c *gin.Context) {
	if !sameAdminOrigin(c.Request) {
		c.JSON(http.StatusForbidden, gin.H{"ok": false, "error": "invalid origin"})
		return
	}
	var input struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if c.ShouldBindJSON(&input) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "invalid credentials"})
		return
	}
	ip := c.ClientIP()
	h.mu.Lock()
	recent := h.failures[ip][:0]
	for _, attempt := range h.failures[ip] {
		if time.Since(attempt) < 15*time.Minute {
			recent = append(recent, attempt)
		}
	}
	h.failures[ip] = recent
	blocked := len(recent) >= 10
	h.mu.Unlock()
	if blocked {
		c.JSON(http.StatusTooManyRequests, gin.H{"ok": false, "error": "too many login attempts"})
		return
	}
	var hash string
	var mustChange bool
	err := h.db.QueryRow(c.Request.Context(), `SELECT password_hash,must_change_password FROM admin_users WHERE username=$1`, input.Username).Scan(&hash, &mustChange)
	if err != nil || bcrypt.CompareHashAndPassword([]byte(hash), []byte(input.Password)) != nil {
		h.mu.Lock()
		h.failures[ip] = append(h.failures[ip], time.Now())
		h.mu.Unlock()
		c.JSON(http.StatusUnauthorized, gin.H{"ok": false, "error": "invalid credentials"})
		return
	}
	token, err := newSessionToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": "session creation failed"})
		return
	}
	h.mu.Lock()
	h.sessions[token] = adminSession{input.Username, time.Now().Add(12 * time.Hour)}
	delete(h.failures, ip)
	h.mu.Unlock()
	setAdminCookie(c, token, 12*3600)
	if err := h.audit(c, input.Username, "login", "admin", ""); err != nil {
		h.mu.Lock()
		delete(h.sessions, token)
		h.mu.Unlock()
		setAdminCookie(c, "", -1)
		c.JSON(500, gin.H{"ok": false, "error": "audit unavailable"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": gin.H{"username": input.Username, "must_change_password": mustChange}})
}

func setAdminCookie(c *gin.Context, token string, age int) {
	http.SetCookie(c.Writer, &http.Cookie{Name: "pingo_admin", Value: token, Path: "/api/admin", MaxAge: age, HttpOnly: true, Secure: c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https", SameSite: http.SameSiteStrictMode})
}

func AdminSessionMiddleware(h *AdminHandler) gin.HandlerFunc {
	return func(c *gin.Context) {
		if h == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"ok": false, "error": "admin login required"})
			return
		}
		cookie, err := c.Cookie("pingo_admin")
		h.mu.Lock()
		session, ok := h.sessions[cookie]
		h.mu.Unlock()
		if err != nil || !ok || time.Now().After(session.expires) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"ok": false, "error": "admin login required"})
			return
		}
		if c.Request.Method != http.MethodGet {
			if !sameAdminOrigin(c.Request) {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"ok": false, "error": "invalid origin"})
				return
			}
		}
		c.Set("admin_user", session.username)
		var mustChange bool
		if err := h.db.QueryRow(c.Request.Context(), `SELECT must_change_password FROM admin_users WHERE username=$1`, session.username).Scan(&mustChange); err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"ok": false, "error": "admin unavailable"})
			return
		}
		if mustChange && c.FullPath() != "/api/admin/me" && c.FullPath() != "/api/admin/change-password" && c.FullPath() != "/api/admin/logout" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"ok": false, "error": "password change required"})
			return
		}
		c.Next()
	}
}

func requestOrigin(request *http.Request) string {
	scheme := "http"
	if request.TLS != nil || request.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return scheme + "://" + request.Host
}

func sameAdminOrigin(request *http.Request) bool {
	origin := request.Header.Get("Origin")
	if origin == "" {
		return false
	}
	parsed, err := url.Parse(origin)
	return err == nil && parsed.Host != "" && strings.EqualFold(parsed.Host, request.Host) &&
		parsed.Scheme == strings.Split(requestOrigin(request), "://")[0]
}

func (h *AdminHandler) Me(c *gin.Context) {
	username := c.GetString("admin_user")
	var mustChange bool
	if err := h.db.QueryRow(c.Request.Context(), `SELECT must_change_password FROM admin_users WHERE username=$1`, username).Scan(&mustChange); err != nil {
		c.JSON(500, gin.H{"ok": false, "error": "admin unavailable"})
		return
	}
	c.JSON(200, gin.H{"ok": true, "data": gin.H{"username": username, "must_change_password": mustChange}})
}

func (h *AdminHandler) Logout(c *gin.Context) {
	token, _ := c.Cookie("pingo_admin")
	h.mu.Lock()
	delete(h.sessions, token)
	h.mu.Unlock()
	setAdminCookie(c, "", -1)
	c.JSON(200, gin.H{"ok": true})
}

func (h *AdminHandler) ChangePassword(c *gin.Context) {
	var input struct {
		Current  string `json:"current"`
		Password string `json:"password"`
	}
	if c.ShouldBindJSON(&input) != nil {
		c.JSON(400, gin.H{"ok": false, "error": "invalid password change request"})
		return
	}
	username := c.GetString("admin_user")
	var hash string
	if err := h.db.QueryRow(c.Request.Context(), `SELECT password_hash FROM admin_users WHERE username=$1`, username).Scan(&hash); err != nil || bcrypt.CompareHashAndPassword([]byte(hash), []byte(input.Current)) != nil {
		c.JSON(403, gin.H{"ok": false, "error": "incorrect current password"})
		return
	}
	newHash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(500, gin.H{"ok": false, "error": "update failed"})
		return
	}
	if _, err := h.db.Exec(c.Request.Context(), `UPDATE admin_users SET password_hash=$1,must_change_password=FALSE WHERE username=$2 AND password_hash=$3`, string(newHash), username, hash); err != nil {
		c.JSON(500, gin.H{"ok": false, "error": "update failed"})
		return
	}
	if err := h.audit(c, username, "change_password", "admin", ""); err != nil {
		c.JSON(500, gin.H{"ok": false, "error": "audit unavailable"})
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

func (h *AdminHandler) audit(c *gin.Context, actor, action, target, detail string) error {
	_, err := h.db.Exec(c.Request.Context(), `INSERT INTO admin_audit(actor,action,target,detail,ip) VALUES($1,$2,$3,$4,$5)`, actor, action, target, detail, c.ClientIP())
	return err
}

func page(c *gin.Context) (int, int) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "30"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if limit < 1 || limit > 100 {
		limit = 30
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

func (h *AdminHandler) Overview(c *gin.Context) {
	var agents, online, conversations, messages, pending, friendships int
	err := h.db.QueryRow(c.Request.Context(), `SELECT (SELECT COUNT(*) FROM agents),(SELECT COUNT(*) FROM agents WHERE online),(SELECT COUNT(*) FROM conversations),(SELECT COUNT(*) FROM messages),(SELECT COUNT(*) FROM offline_queue WHERE NOT delivered),(SELECT COUNT(*) FROM friendships WHERE status='accepted')`).Scan(&agents, &online, &conversations, &messages, &pending, &friendships)
	if err != nil {
		c.JSON(500, gin.H{"ok": false, "error": "overview unavailable"})
		return
	}
	c.JSON(200, gin.H{"ok": true, "data": gin.H{"total_agents": agents, "online_agents": online, "total_conversations": conversations, "total_messages": messages, "pending_offline": pending, "friendships": friendships, "collected_at": time.Now().UTC()}})
}

func (h *AdminHandler) Agents(c *gin.Context) {
	limit, offset := page(c)
	rows, err := h.db.Query(c.Request.Context(), `SELECT agent_id,name,COALESCE(owner_name,''),COALESCE(owner_email,''),COALESCE(status_text,''),COALESCE(avatar_url,''),capabilities,availability,COALESCE(online,FALSE),last_heartbeat,created_at,updated_at FROM agents WHERE ($1='' OR agent_id ILIKE '%'||$1||'%' OR name ILIKE '%'||$1||'%') ORDER BY created_at DESC LIMIT $2 OFFSET $3`, c.Query("q"), limit, offset)
	if err != nil {
		c.JSON(500, gin.H{"ok": false, "error": "agents unavailable"})
		return
	}
	defer rows.Close()
	items := make([]gin.H, 0)
	for rows.Next() {
		var id, name, owner, ownerEmail, status, avatar string
		var capabilities, availability []byte
		var online bool
		var heartbeat *time.Time
		var created, updated time.Time
		if err := rows.Scan(&id, &name, &owner, &ownerEmail, &status, &avatar, &capabilities, &availability, &online, &heartbeat, &created, &updated); err != nil {
			c.JSON(500, gin.H{"ok": false, "error": "agents scan failed"})
			return
		}
		items = append(items, gin.H{"agent_id": id, "name": name, "owner_name": owner, "owner_email": ownerEmail, "status_text": status, "avatar_url": avatar, "capabilities": string(capabilities), "availability": string(availability), "online": online, "last_heartbeat": heartbeat, "created_at": created, "updated_at": updated})
	}
	if rows.Err() != nil {
		c.JSON(500, gin.H{"ok": false, "error": "agents unavailable"})
		return
	}
	c.JSON(200, gin.H{"ok": true, "data": items})
}

func (h *AdminHandler) DeleteAgent(c *gin.Context) {
	agentID := c.Param("id")
	if err := h.deleteAgent(c.Request.Context(), c.GetString("admin_user"), agentID, c.ClientIP()); err != nil {
		c.JSON(500, gin.H{"ok": false, "error": "agent delete failed"})
		return
	}
	if h.disconnect != nil {
		h.disconnect(agentID)
	}
	c.JSON(200, gin.H{"ok": true})
}

func (h *AdminHandler) deleteAgent(ctx context.Context, actor, agentID, ip string) error {
	tx, err := h.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `DELETE FROM conversations c WHERE c.type='direct' AND EXISTS (SELECT 1 FROM conversation_members cm WHERE cm.conversation_id=c.conversation_id AND cm.agent_id=$1)`, agentID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE conversations SET created_by=NULL WHERE created_by=$1`, agentID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE messages SET from_agent=NULL WHERE from_agent=$1`, agentID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM conversation_members WHERE agent_id=$1`, agentID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM friendships WHERE agent_a=$1 OR agent_b=$1`, agentID); err != nil {
		return err
	}
	result, err := tx.Exec(ctx, `DELETE FROM agents WHERE agent_id=$1`, agentID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("agent not found")
	}
	if _, err = tx.Exec(ctx, `INSERT INTO admin_audit(actor,action,target,detail,ip) VALUES($1,'delete_agent',$2,'',$3)`, actor, agentID, ip); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (h *AdminHandler) Conversations(c *gin.Context) {
	limit, offset := page(c)
	rows, err := h.db.Query(c.Request.Context(), `SELECT c.conversation_id,c.type,COALESCE(c.name,''),COALESCE(c.status,''),COALESCE(c.created_by,''),c.created_at,c.last_message_at,COALESCE(c.last_message_preview,''),COUNT(DISTINCT cm.agent_id),COUNT(DISTINCT m.message_id),COALESCE(string_agg(DISTINCT COALESCE(a.name,cm.agent_id), ' · '),'') FROM conversations c LEFT JOIN conversation_members cm ON cm.conversation_id=c.conversation_id AND cm.left_at IS NULL LEFT JOIN agents a ON a.agent_id=cm.agent_id LEFT JOIN messages m ON m.conversation_id=c.conversation_id WHERE ($1='' OR c.conversation_id ILIKE '%'||$1||'%' OR c.name ILIKE '%'||$1||'%') GROUP BY c.conversation_id ORDER BY COALESCE(c.last_message_at,c.created_at) DESC LIMIT $2 OFFSET $3`, c.Query("q"), limit, offset)
	if err != nil {
		c.JSON(500, gin.H{"ok": false, "error": "conversations unavailable"})
		return
	}
	defer rows.Close()
	items := make([]gin.H, 0)
	for rows.Next() {
		var id, kind, name, status, creator, preview string
		var created time.Time
		var last *time.Time
		var memberCount, messageCount int
		var memberNames string
		if err := rows.Scan(&id, &kind, &name, &status, &creator, &created, &last, &preview, &memberCount, &messageCount, &memberNames); err != nil {
			c.JSON(500, gin.H{"ok": false, "error": "conversation scan failed"})
			return
		}
		items = append(items, gin.H{"id": id, "type": kind, "name": name, "status": status, "created_by": creator, "created_at": created, "last_message_at": last, "last_message_preview": preview, "member_count": memberCount, "message_count": messageCount, "member_names": memberNames})
	}
	if rows.Err() != nil {
		c.JSON(500, gin.H{"ok": false, "error": "conversations unavailable"})
		return
	}
	c.JSON(200, gin.H{"ok": true, "data": items})
}

func (h *AdminHandler) DeleteConversation(c *gin.Context) {
	keys, err := h.deleteConversation(c.Request.Context(), c.GetString("admin_user"), c.Param("id"), c.ClientIP())
	if err != nil {
		c.JSON(500, gin.H{"ok": false, "error": "conversation delete failed"})
		return
	}
	for _, key := range keys {
		if h.files != nil {
			_ = h.files.Delete(key)
		}
	}
	c.JSON(200, gin.H{"ok": true})
}

func (h *AdminHandler) deleteConversation(ctx context.Context, actor, conversationID, ip string) ([]string, error) {
	tx, err := h.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `SELECT DISTINCT content_file->>'storage_key' FROM messages WHERE conversation_id=$1 AND message_type='file' AND content_file->>'storage_key' IS NOT NULL`, conversationID)
	if err != nil {
		return nil, err
	}
	keys := []string{}
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			rows.Close()
			return nil, err
		}
		keys = append(keys, key)
	}
	rows.Close()
	result, err := tx.Exec(ctx, `DELETE FROM conversations WHERE conversation_id=$1`, conversationID)
	if err != nil {
		return nil, err
	}
	if result.RowsAffected() == 0 {
		return nil, fmt.Errorf("conversation not found")
	}
	orphaned := make([]string, 0, len(keys))
	for _, key := range keys {
		var referenced bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM messages WHERE message_type='file' AND content_file->>'storage_key'=$1)`, key).Scan(&referenced); err != nil {
			return nil, err
		}
		if !referenced {
			orphaned = append(orphaned, key)
		}
	}
	if _, err = tx.Exec(ctx, `INSERT INTO admin_audit(actor,action,target,detail,ip) VALUES($1,'delete_conversation',$2,$3,$4)`, actor, conversationID, fmt.Sprintf("files=%d", len(keys)), ip); err != nil {
		return nil, err
	}
	return orphaned, tx.Commit(ctx)
}

func (h *AdminHandler) Messages(c *gin.Context) {
	limit, offset := page(c)
	id := c.Param("id")
	rows, err := h.db.Query(c.Request.Context(), `SELECT m.message_id,COALESCE(m.from_agent,''),COALESCE(a.name,'已删除 Agent'),m.message_type,COALESCE(m.content_text,''),COALESCE(m.content_file::text,''),m.created_at FROM messages m LEFT JOIN agents a ON a.agent_id=m.from_agent WHERE m.conversation_id=$1 ORDER BY m.created_at ASC,m.message_id ASC LIMIT $2 OFFSET $3`, id, limit, offset)
	if err != nil {
		c.JSON(500, gin.H{"ok": false, "error": "messages unavailable"})
		return
	}
	defer rows.Close()
	items := make([]gin.H, 0)
	for rows.Next() {
		var messageID, from, fromName, kind, text string
		var file []byte
		var created time.Time
		if err := rows.Scan(&messageID, &from, &fromName, &kind, &text, &file, &created); err != nil {
			c.JSON(500, gin.H{"ok": false, "error": "message scan failed"})
			return
		}
		items = append(items, gin.H{"message_id": messageID, "from_agent": from, "from_name": fromName, "message_type": kind, "content_text": text, "content_file": string(file), "created_at": created})
	}
	if rows.Err() != nil {
		c.JSON(500, gin.H{"ok": false, "error": "messages unavailable"})
		return
	}
	if err := h.audit(c, c.GetString("admin_user"), "view_messages", id, fmt.Sprintf("limit=%d offset=%d", limit, offset)); err != nil {
		c.JSON(500, gin.H{"ok": false, "error": "audit unavailable"})
		return
	}
	c.JSON(200, gin.H{"ok": true, "data": items})
}

func (h *AdminHandler) Relations(c *gin.Context) {
	limit, offset := page(c)
	rows, err := h.db.Query(c.Request.Context(), `SELECT f.agent_a,COALESCE(a.name,f.agent_a),f.agent_b,COALESCE(b.name,f.agent_b),f.status,COALESCE(f.initiated_by,''),COALESCE(i.name,''),COALESCE(f.request_message,''),f.interaction_count,f.last_interaction,f.created_at,f.updated_at FROM friendships f LEFT JOIN agents a ON a.agent_id=f.agent_a LEFT JOIN agents b ON b.agent_id=f.agent_b LEFT JOIN agents i ON i.agent_id=f.initiated_by WHERE ($1='' OR f.agent_a=$1 OR f.agent_b=$1) ORDER BY f.updated_at DESC LIMIT $2 OFFSET $3`, c.Query("agent_id"), limit, offset)
	if err != nil {
		c.JSON(500, gin.H{"ok": false, "error": "relations unavailable"})
		return
	}
	defer rows.Close()
	items := make([]gin.H, 0)
	for rows.Next() {
		var a, aName, b, bName, status, initiator, initiatorName, requestMessage string
		var created, updated time.Time
		var interactionCount int
		var lastInteraction *time.Time
		if rows.Scan(&a, &aName, &b, &bName, &status, &initiator, &initiatorName, &requestMessage, &interactionCount, &lastInteraction, &created, &updated) != nil {
			c.JSON(500, gin.H{"ok": false, "error": "relation scan failed"})
			return
		}
		items = append(items, gin.H{"agent_a": a, "agent_a_name": aName, "agent_b": b, "agent_b_name": bName, "status": status, "initiated_by": initiator, "initiated_by_name": initiatorName, "request_message": requestMessage, "interaction_count": interactionCount, "last_interaction": lastInteraction, "created_at": created, "updated_at": updated})
	}
	if rows.Err() != nil {
		c.JSON(500, gin.H{"ok": false, "error": "relations unavailable"})
		return
	}
	c.JSON(200, gin.H{"ok": true, "data": items})
}

func (h *AdminHandler) DeleteRelation(c *gin.Context) {
	a, b := c.Param("a"), c.Param("b")
	tx, err := h.db.Begin(c.Request.Context())
	if err == nil {
		var result pgconn.CommandTag
		result, err = tx.Exec(c.Request.Context(), `DELETE FROM friendships WHERE (agent_a=$1 AND agent_b=$2) OR (agent_a=$2 AND agent_b=$1)`, a, b)
		if err == nil && result.RowsAffected() == 0 {
			err = fmt.Errorf("relation not found")
		}
		if err == nil {
			_, err = tx.Exec(c.Request.Context(), `INSERT INTO admin_audit(actor,action,target,detail,ip) VALUES($1,'delete_relation',$2,'',$3)`, c.GetString("admin_user"), a+":"+b, c.ClientIP())
		}
		if err == nil {
			err = tx.Commit(c.Request.Context())
		} else {
			_ = tx.Rollback(c.Request.Context())
		}
	}
	if err != nil {
		c.JSON(500, gin.H{"ok": false, "error": "relation delete failed"})
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

func (h *AdminHandler) Queue(c *gin.Context) {
	limit, offset := page(c)
	rows, err := h.db.Query(c.Request.Context(), `SELECT id,COALESCE(target_agent,''),COALESCE(message_id,''),COALESCE(conversation_id,''),queued_at,COALESCE(delivered,FALSE),delivered_at FROM offline_queue ORDER BY queued_at DESC LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		c.JSON(500, gin.H{"ok": false, "error": "queue unavailable"})
		return
	}
	defer rows.Close()
	items := make([]gin.H, 0)
	for rows.Next() {
		var id int
		var target, message, conversation string
		var at time.Time
		var delivered bool
		var deliveredAt *time.Time
		if rows.Scan(&id, &target, &message, &conversation, &at, &delivered, &deliveredAt) != nil {
			c.JSON(500, gin.H{"ok": false, "error": "queue scan failed"})
			return
		}
		items = append(items, gin.H{"id": id, "target_agent": target, "message_id": message, "conversation_id": conversation, "queued_at": at, "delivered": delivered, "delivered_at": deliveredAt})
	}
	if rows.Err() != nil {
		c.JSON(500, gin.H{"ok": false, "error": "queue unavailable"})
		return
	}
	c.JSON(200, gin.H{"ok": true, "data": items})
}

func (h *AdminHandler) Files(c *gin.Context) {
	limit, offset := page(c)
	rows, err := h.db.Query(c.Request.Context(), `SELECT message_id,conversation_id,from_agent,content_file,created_at FROM messages WHERE message_type='file' AND content_file IS NOT NULL ORDER BY created_at DESC LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		c.JSON(500, gin.H{"ok": false, "error": "files unavailable"})
		return
	}
	defer rows.Close()
	items := make([]gin.H, 0)
	for rows.Next() {
		var id, conversation, from string
		var file []byte
		var at time.Time
		if rows.Scan(&id, &conversation, &from, &file, &at) != nil {
			c.JSON(500, gin.H{"ok": false, "error": "file scan failed"})
			return
		}
		items = append(items, gin.H{"message_id": id, "conversation_id": conversation, "from_agent": from, "content_file": string(file), "created_at": at})
	}
	if rows.Err() != nil {
		c.JSON(500, gin.H{"ok": false, "error": "files unavailable"})
		return
	}
	c.JSON(200, gin.H{"ok": true, "data": items})
}

func (h *AdminHandler) DownloadFile(c *gin.Context) {
	var raw []byte
	if err := h.db.QueryRow(c.Request.Context(), `SELECT content_file FROM messages WHERE message_id=$1 AND message_type='file'`, c.Param("id")).Scan(&raw); err != nil {
		c.JSON(404, gin.H{"ok": false, "error": "file message not found"})
		return
	}
	var file struct {
		StorageKey string `json:"storage_key"`
	}
	if json.Unmarshal(raw, &file) != nil || !strings.HasPrefix(file.StorageKey, "files/") || filepath.Base(file.StorageKey) != strings.TrimPrefix(file.StorageKey, "files/") {
		c.JSON(400, gin.H{"ok": false, "error": "invalid file reference"})
		return
	}
	reader, size, filename, err := h.files.Get(file.StorageKey)
	if err != nil {
		c.JSON(404, gin.H{"ok": false, "error": "file not found"})
		return
	}
	defer reader.Close()
	if err := h.audit(c, c.GetString("admin_user"), "download_file", c.Param("id"), file.StorageKey); err != nil {
		c.JSON(500, gin.H{"ok": false, "error": "audit unavailable"})
		return
	}
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filepath.Base(filename)))
	c.Header("Content-Type", "application/octet-stream")
	c.Header("Content-Length", strconv.FormatInt(size, 10))
	c.Status(200)
	_, _ = io.Copy(c.Writer, reader)
}

func (h *AdminHandler) Audit(c *gin.Context) {
	limit, offset := page(c)
	rows, err := h.db.Query(c.Request.Context(), `SELECT actor,action,target,detail,ip,created_at FROM admin_audit ORDER BY created_at DESC LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		c.JSON(500, gin.H{"ok": false, "error": "audit unavailable"})
		return
	}
	defer rows.Close()
	items := make([]gin.H, 0)
	for rows.Next() {
		var actor, action, target, detail, ip string
		var at time.Time
		if rows.Scan(&actor, &action, &target, &detail, &ip, &at) != nil {
			c.JSON(500, gin.H{"ok": false, "error": "audit scan failed"})
			return
		}
		items = append(items, gin.H{"actor": actor, "action": action, "target": target, "detail": detail, "ip": ip, "created_at": at})
	}
	if rows.Err() != nil {
		c.JSON(500, gin.H{"ok": false, "error": "audit unavailable"})
		return
	}
	c.JSON(200, gin.H{"ok": true, "data": items})
}
