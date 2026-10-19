// Package httpx 提供 HTTP 层通用工具：JSON 响应、请求解析、客户端 IP、
// 以及登录限速器。它不依赖任何业务模型，供 clientapi / console 共用。
package httpx

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// maxBodyBytes 限制请求体大小（1MB），防止异常大包拖垮服务。
const maxBodyBytes = 1 << 20

// JSON 以 JSON 响应。
func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// Error 以 200 + {"error": msg} 返回，兼容 RustDesk 客户端对错误的解析方式
// （客户端只读取 error 字段，不关心 HTTP 状态码）。
func Error(w http.ResponseWriter, msg string) {
	JSON(w, http.StatusOK, map[string]string{"error": msg})
}

// ServerError 返回 500（错误详情不返回给客户端）。
func ServerError(w http.ResponseWriter, err error) {
	JSON(w, http.StatusInternalServerError, map[string]string{"error": "服务端错误"})
}

// ReadJSON 解析请求体为 JSON，限制大小。
func ReadJSON(r *http.Request, v any) error {
	body := io.LimitReader(r.Body, maxBodyBytes)
	defer r.Body.Close()
	return json.NewDecoder(body).Decode(v)
}

// ClientIP 提取客户端真实 IP，优先取 X-Forwarded-For 第一个地址。
func ClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.IndexByte(xff, ','); i >= 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// BearerToken 从 Authorization 头解析 Bearer token。
func BearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(h, "Bearer ") {
		return strings.TrimSpace(h[len("Bearer "):])
	}
	return ""
}

// ---------- 登录限速 ----------

// LoginLimiter 按 IP 限制单位时间内的登录尝试次数。
type LoginLimiter struct {
	mu     sync.Mutex
	window map[string]*ipCounter
	limit  int
}

type ipCounter struct {
	count int
	until time.Time
}

// NewLoginLimiter 创建限速器；limitPerMinute <= 0 表示关闭限速。
func NewLoginLimiter(limitPerMinute int) *LoginLimiter {
	return &LoginLimiter{window: make(map[string]*ipCounter), limit: limitPerMinute}
}

// Allow 判断某 IP 当前是否允许登录。
func (l *LoginLimiter) Allow(ip string) bool {
	if l.limit <= 0 {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	c, ok := l.window[ip]
	if !ok || now.After(c.until) {
		l.window[ip] = &ipCounter{count: 1, until: now.Add(time.Minute)}
		return true
	}
	c.count++
	return c.count <= l.limit
}
