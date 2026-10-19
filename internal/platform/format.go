package platform

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// ToStr 把外部接口中可能为数字/布尔/字符串的字段统一转为字符串。
func ToStr(v any) string {
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

// ConvertFilesize 把字节数格式化为可读大小（与 Python 参考版一致）。
func ConvertFilesize(sizeBytes int64) string {
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

// SplitPeers 把逗号分隔的设备 ID 字符串拆成列表（空串返回空切片）。
func SplitPeers(s string) []string {
	if strings.TrimSpace(s) == "" {
		return []string{}
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// ParseTagColors 兼容 tag_colors 的两种编码：JSON 对象或双重编码的 JSON 字符串。
func ParseTagColors(raw json.RawMessage) map[string]string {
	if len(raw) == 0 {
		return nil
	}
	var m map[string]string
	if json.Unmarshal(raw, &m) == nil {
		return m
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		var m2 map[string]string
		if json.Unmarshal([]byte(s), &m2) == nil {
			return m2
		}
	}
	return nil
}
