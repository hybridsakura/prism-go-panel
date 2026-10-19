package platform

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"

	"gorm.io/gorm"
)

// HeartbeatInterval 规定的 Agent 心跳周期（10s）。
const HeartbeatInterval = 10 * time.Second

// HashToken 计算注册码或设备令牌的 SHA-256 哈希（十六进制小写）。
func HashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// GenerateRandomToken 生成安全的 32 字节随机令牌（十六进制 64 字符）。
func GenerateRandomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// AgentPresence 负责终端 Agent 在线心跳的内存追踪与批量落盘。
//
// 500 台终端以 10s 周期上报，约产生 50 req/s。
// 若每次心跳均 UPDATE 数据库，将对 SQLite WAL 与机械/低功耗磁盘（如 Atom NAS）
// 造成不必要的写放大与 IO 抖动。
// 故心跳只更新本组件的内存状态，每 20s 集中执行单次批量事务落盘。
type AgentPresence struct {
	mu       sync.RWMutex
	lastSeen map[uint]time.Time
}

// NewAgentPresence 创建内存心跳管理器。
func NewAgentPresence() *AgentPresence {
	return &AgentPresence{
		lastSeen: make(map[uint]time.Time),
	}
}

// Touch 记录指定 Agent 的最近心跳时间。
func (p *AgentPresence) Touch(agentID uint, t time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.lastSeen[agentID] = t
}

// LastSeen 获取指定 Agent 最近心跳时间。
func (p *AgentPresence) LastSeen(agentID uint) (time.Time, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	t, ok := p.lastSeen[agentID]
	return t, ok
}

// Flush 将内存中的心跳批量刷入数据库。
func (p *AgentPresence) Flush(db *gorm.DB) error {
	p.mu.RLock()
	snapshot := make(map[uint]time.Time, len(p.lastSeen))
	for id, t := range p.lastSeen {
		snapshot[id] = t
	}
	p.mu.RUnlock()

	if len(snapshot) == 0 {
		return nil
	}

	return db.Transaction(func(tx *gorm.DB) error {
		for id, t := range snapshot {
			if err := tx.Table("agents").Where("id = ?", id).Update("last_seen_at", t).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
