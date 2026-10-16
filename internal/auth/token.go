package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"gorm.io/gorm"

	"rustdesk-api-go/internal/models"
)

// ErrInvalidToken 表示 token 不存在或已过期。
var ErrInvalidToken = errors.New("invalid access token")

// tokenEntry 缓存条目：token 对应的用户与过期时间。
type tokenEntry struct {
	user     *models.User
	token    *models.Token
	expireAt time.Time
}

// TokenStore 管理客户端 access_token，带内存缓存，避免每次请求都查库。
type TokenStore struct {
	db    *gorm.DB
	ttl   time.Duration
	mu    sync.RWMutex
	cache map[string]*tokenEntry
}

// NewTokenStore 创建 token 存储。ttl 为 token 有效期。
func NewTokenStore(db *gorm.DB, ttl time.Duration) *TokenStore {
	return &TokenStore{db: db, ttl: ttl, cache: make(map[string]*tokenEntry)}
}

// Issue 为用户颁发 token：优先复用该用户在该设备上未过期的 token（与参考版一致）。
func (s *TokenStore) Issue(u *models.User, rid, uuid string) (*models.Token, error) {
	now := time.Now()
	var t models.Token
	err := s.db.Where("uid = ? AND rid = ? AND expire_at > ?", u.ID, rid, now).First(&t).Error
	if err == nil {
		return &t, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return nil, err
	}
	t = models.Token{
		Username:    u.Username,
		UID:         u.ID,
		RID:         rid,
		UUID:        uuid,
		AccessToken: hex.EncodeToString(buf),
		ExpireAt:    now.Add(s.ttl),
	}
	if err := s.db.Create(&t).Error; err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.cache[t.AccessToken] = &tokenEntry{user: u, token: &t, expireAt: t.ExpireAt}
	s.mu.Unlock()
	return &t, nil
}

// Validate 校验 access_token，返回所属用户与 token 记录。
func (s *TokenStore) Validate(accessToken string) (*models.User, *models.Token, error) {
	if accessToken == "" {
		return nil, nil, ErrInvalidToken
	}
	s.mu.RLock()
	entry, ok := s.cache[accessToken]
	s.mu.RUnlock()
	if !ok {
		var t models.Token
		if err := s.db.Where("access_token = ?", accessToken).First(&t).Error; err != nil {
			return nil, nil, ErrInvalidToken
		}
		var u models.User
		if err := s.db.First(&u, t.UID).Error; err != nil {
			return nil, nil, err
		}
		entry = &tokenEntry{user: &u, token: &t, expireAt: t.ExpireAt}
		s.mu.Lock()
		s.cache[accessToken] = entry
		s.mu.Unlock()
	}
	if time.Now().After(entry.expireAt) {
		s.evict(accessToken)
		return nil, nil, ErrInvalidToken
	}
	return entry.user, entry.token, nil
}

// KeepAlive 延长指定设备（rid+uuid）名下所有 token 的有效期，心跳调用。
func (s *TokenStore) KeepAlive(rid, uuid string) error {
	expireAt := time.Now().Add(s.ttl)
	if err := s.db.Model(&models.Token{}).
		Where("rid = ? AND uuid = ?", rid, uuid).
		Update("expire_at", expireAt).Error; err != nil {
		return err
	}
	s.mu.Lock()
	for _, e := range s.cache {
		if e.token.RID == rid && e.token.UUID == uuid {
			e.expireAt = expireAt
			e.token.ExpireAt = expireAt
		}
	}
	s.mu.Unlock()
	return nil
}

// RevokeByDevice 删除指定设备名下的全部 token（登出调用）。
func (s *TokenStore) RevokeByDevice(rid, uuid string) error {
	if err := s.db.Where("rid = ? AND uuid = ?", rid, uuid).Delete(&models.Token{}).Error; err != nil {
		return err
	}
	s.mu.Lock()
	for k, e := range s.cache {
		if e.token.RID == rid && e.token.UUID == uuid {
			delete(s.cache, k)
		}
	}
	s.mu.Unlock()
	return nil
}

// CleanupExpired 清理已过期的 token（周期性任务）。
func (s *TokenStore) CleanupExpired() error {
	if err := s.db.Where("expire_at < ?", time.Now()).Delete(&models.Token{}).Error; err != nil {
		return err
	}
	s.mu.Lock()
	now := time.Now()
	for k, e := range s.cache {
		if now.After(e.expireAt) {
			delete(s.cache, k)
		}
	}
	s.mu.Unlock()
	return nil
}

func (s *TokenStore) evict(accessToken string) {
	s.mu.Lock()
	delete(s.cache, accessToken)
	s.mu.Unlock()
}
