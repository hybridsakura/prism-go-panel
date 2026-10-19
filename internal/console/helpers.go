package console

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strconv"
	"time"
)

// sessionCookieName 管理后台会话 Cookie 名称。
const sessionCookieName = "rdadmin_session"

// parsePage 解析分页参数，带回退保护。
func parsePage(r *http.Request) (page, size int) {
	page = 1
	size = 15
	if v := r.URL.Query().Get("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			page = n
		}
	}
	if v := r.URL.Query().Get("size"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
			size = n
		}
	}
	return page, size
}

// formatTime 统一时间输出格式（RFC3339，带本地时区偏移）。
func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Local().Format(time.RFC3339)
}

// newHash 生成分享链接的随机标识（16 字节十六进制）。
func newHash() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		sum := sha256.Sum256([]byte(time.Now().String()))
		return hex.EncodeToString(sum[:16])
	}
	return hex.EncodeToString(buf)
}
