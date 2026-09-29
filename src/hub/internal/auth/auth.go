package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
)

var (
	ErrInvalidToken = errors.New("invalid token")
)

// TokenStore 管理 token 和 agent_id 的映射
// 生产环境从数据库查，这里提供内存缓存加速
type TokenStore struct {
	mu    sync.RWMutex
	cache map[string]string // token -> agent_id
}

func NewTokenStore() *TokenStore {
	return &TokenStore{
		cache: make(map[string]string),
	}
}

func (s *TokenStore) Set(token, agentID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cache[token] = agentID
}

func (s *TokenStore) Get(token string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	agentID, ok := s.cache[token]
	return agentID, ok
}

func (s *TokenStore) Delete(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.cache, token)
}

// GenerateToken 生成随机 token
func GenerateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "token-" + hex.EncodeToString(b), nil
}

// ValidateToken 验证 token 格式
func ValidateToken(token string) bool {
	if len(token) < 10 {
		return false
	}
	return true
}
