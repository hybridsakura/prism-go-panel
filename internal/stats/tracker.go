// Package stats 提供设备在线状态跟踪与统计，数据全部在内存，O(1) 判定。
package stats

import (
	"sync"
	"time"
)

// Tracker 记录每台设备最近一次心跳/sysinfo 的时间，用于在线判定。
type Tracker struct {
	mu       sync.RWMutex
	lastSeen map[string]time.Time
	timeout  time.Duration
}

// NewTracker 创建跟踪器，timeout 内没有心跳即视为离线。
func NewTracker(timeout time.Duration) *Tracker {
	return &Tracker{lastSeen: make(map[string]time.Time), timeout: timeout}
}

// Touch 更新设备最近活跃时间。
func (t *Tracker) Touch(rid string) {
	if rid == "" {
		return
	}
	t.mu.Lock()
	t.lastSeen[rid] = time.Now()
	t.mu.Unlock()
}

// Online 判断设备当前是否在线。
func (t *Tracker) Online(rid string) bool {
	t.mu.RLock()
	ts, ok := t.lastSeen[rid]
	t.mu.RUnlock()
	return ok && time.Since(ts) <= t.timeout
}

// OnlineCount 统计当前在线设备数（异步清理过期项，避免锁竞争）。
func (t *Tracker) OnlineCount() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	now := time.Now()
	n := 0
	for _, ts := range t.lastSeen {
		if now.Sub(ts) <= t.timeout {
			n++
		}
	}
	return n
}

// PruneExpired 清理超过阈值的陈旧条目（周期性任务）。
func (t *Tracker) PruneExpired() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()
	n := 0
	for rid, ts := range t.lastSeen {
		if now.Sub(ts) > t.timeout {
			delete(t.lastSeen, rid)
			n++
		}
	}
	return n
}
