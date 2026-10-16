// Package config 负责从环境变量加载服务端配置。
package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// Config 保存服务端全部可配置项。优先级：环境变量 > 默认值。
type Config struct {
	Host              string        // 监听地址，默认 0.0.0.0
	Port              int           // 监听端口，默认 21114
	DBURL             string        // SQLite 文件路径或 MySQL DSN
	SecretKey         string        // 预留：管理后台会话签名密钥
	AllowRegistration bool          // 是否开放自助注册
	IDServers         []string      // Web 控制端使用的 ID 服务器地址列表
	TokenTTL          time.Duration // access_token 有效期，默认 7200s
	OnlineTimeout     time.Duration // 设备在线判定阈值，默认 120s
	LoginRateLimit    int           // 登录接口每分钟每 IP 最大尝试次数，0 表示关闭
	WebClientDir      string        // Flutter Web 控制端静态资源目录
	AdminUsername     string        // 初始管理员用户名（可选，首次启动播种）
	AdminPassword     string        // 初始管理员密码（可选）
	SessionTTL        time.Duration // 管理后台会话有效期，默认 12h
	LogLevel          string        // 日志级别：debug/info/warn/error，默认 info
	LogFile           string        // 日志文件路径，空则输出到标准输出（服务模式建议配置）
	HBBSKeyFile       string        // hbbs 公钥文件（id_ed25519.pub）路径，用于后台自动填入公钥
}

// Load 从环境变量读取配置，未设置时使用默认值。
func Load() *Config {
	return &Config{
		Host:              getenv("HOST", "0.0.0.0"),
		Port:              getenvInt("PORT", 21114),
		DBURL:             getenv("DB_URL", "./rustdesk-api.db"),
		SecretKey:         getenv("SECRET_KEY", ""),
		AllowRegistration: getenvBool("ALLOW_REGISTRATION", true),
		IDServers:         splitList(os.Getenv("ID_SERVER")),
		TokenTTL:          time.Duration(getenvInt("TOKEN_TTL_SECONDS", 7200)) * time.Second,
		OnlineTimeout:     time.Duration(getenvInt("ONLINE_TIMEOUT_SECONDS", 120)) * time.Second,
		LoginRateLimit:    getenvInt("LOGIN_RATE_LIMIT", 0),
		WebClientDir:      getenv("WEBCLIENT_DIR", "./webui-dist"),
		AdminUsername:     getenv("ADMIN_USER", ""),
		AdminPassword:     getenv("ADMIN_PASS", ""),
		SessionTTL:        time.Duration(getenvInt("SESSION_TTL_SECONDS", 43200)) * time.Second,
		LogLevel:          getenv("LOG_LEVEL", getenv("RUST_LOG", "info")), // RUST_LOG 为兼容别名
		LogFile:           getenv("LOG_FILE", ""),
		HBBSKeyFile:       getenv("HBBS_KEY_FILE", ""),
	}
}

func getenv(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func getenvInt(key string, def int) int {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func getenvBool(key string, def bool) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	switch v {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	}
	return def
}

// splitList 解析逗号分隔的环境变量，去掉空白与空项。
func splitList(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
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
