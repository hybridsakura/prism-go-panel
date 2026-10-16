package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// maxBodyBytes 限制请求体大小（1MB），防止异常大包拖垮服务。
const maxBodyBytes = 1 << 20

// writeJSON 以 JSON 响应。
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// apiError 以 200 + {"error": msg} 返回，兼容 RustDesk 客户端对错误的解析方式
// （客户端只读取 error 字段，不关心 HTTP 状态码）。
func apiError(w http.ResponseWriter, msg string) {
	writeJSON(w, http.StatusOK, map[string]string{"error": msg})
}

// serverError 记录并返回 500。
func serverError(w http.ResponseWriter, err error) {
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "服务端错误"})
}

// readJSON 解析请求体为 JSON，限制大小。
func readJSON(r *http.Request, v any) error {
	body := io.LimitReader(r.Body, maxBodyBytes)
	defer r.Body.Close()
	dec := json.NewDecoder(body)
	if err := dec.Decode(v); err != nil {
		return err
	}
	return nil
}

// clientIP 提取客户端真实 IP，优先取 X-Forwarded-For 第一个地址。
func clientIP(r *http.Request) string {
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

// bearerToken 从 Authorization 头解析 Bearer token。
func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(h, "Bearer ") {
		return strings.TrimSpace(h[len("Bearer "):])
	}
	return ""
}

// toStr 把 audit 等接口中可能为数字或字符串的字段统一转为字符串。
func toStr(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(t)
	default:
		return fmt.Sprint(t)
	}
}

// convertFilesize 与 Python 参考版一致的可读文件大小格式化。
func convertFilesize(sizeBytes int64) string {
	if sizeBytes == 0 {
		return "0B"
	}
	sizeName := []string{"B", "KB", "MB", "GB", "TB", "PB", "EB", "ZB", "YB"}
	i := 0
	f := float64(sizeBytes)
	for f >= 1024 && i < len(sizeName)-1 {
		f /= 1024
		i++
	}
	return fmt.Sprintf("%.2f %s", f, sizeName[i])
}

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
