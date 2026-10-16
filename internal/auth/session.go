package auth

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

// Session 管理后台会话。
type Session struct {
	UserID uint
	Expiry time.Time
}

// SessionStore 管理后台会话存储（进程内存，服务重启后会话失效）。
type SessionStore struct {
	mu       sync.RWMutex
	sessions map[string]Session
	ttl      time.Duration
}

// NewSessionStore 创建会话存储，ttl 为会话有效期。
func NewSessionStore(ttl time.Duration) *SessionStore {
	return &SessionStore{sessions: make(map[string]Session), ttl: ttl}
}

// Create 为指定用户创建新会话，返回会话 ID。
func (s *SessionStore) Create(userID uint) (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	id := hex.EncodeToString(buf)
	s.mu.Lock()
	s.sessions[id] = Session{UserID: userID, Expiry: time.Now().Add(s.ttl)}
	s.mu.Unlock()
	return id, nil
}

// Get 查询会话，过期或不存在时返回 false。
func (s *SessionStore) Get(id string) (Session, bool) {
	if id == "" {
		return Session{}, false
	}
	s.mu.RLock()
	sess, ok := s.sessions[id]
	s.mu.RUnlock()
	if !ok || time.Now().After(sess.Expiry) {
		if ok {
			s.Delete(id)
		}
		return Session{}, false
	}
	return sess, true
}

// Delete 删除会话（登出）。
func (s *SessionStore) Delete(id string) {
	s.mu.Lock()
	delete(s.sessions, id)
	s.mu.Unlock()
}

// Cleanup 清理全部过期会话（周期性任务）。
func (s *SessionStore) Cleanup() {
	s.mu.Lock()
	now := time.Now()
	for id, sess := range s.sessions {
		if now.After(sess.Expiry) {
			delete(s.sessions, id)
		}
	}
	s.mu.Unlock()
}
