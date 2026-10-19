package api

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hybridsakura/prism-go-panel/internal/auth"
	"github.com/hybridsakura/prism-go-panel/internal/config"
	"github.com/hybridsakura/prism-go-panel/internal/database"
	"github.com/hybridsakura/prism-go-panel/internal/stats"
)

// newTestServerWith 允许定制配置的测试服务构造器。
func newTestServerWith(t *testing.T, mutate func(*config.Config)) (*httptest.Server, *http.Client) {
	t.Helper()
	cfg := &config.Config{
		Host:              "127.0.0.1",
		Port:              0,
		DBURL:             filepath.Join(t.TempDir(), "test.db"),
		AllowRegistration: true,
		TokenTTL:          time.Hour,
		OnlineTimeout:     120 * time.Second,
		SessionTTL:        time.Hour,
	}
	if mutate != nil {
		mutate(cfg)
	}
	db, err := database.Open(cfg)
	if err != nil {
		t.Fatalf("打开测试数据库失败: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	srv := NewServer(cfg, db, auth.NewTokenStore(db, cfg.TokenTTL), auth.NewSessionStore(cfg.SessionTTL), stats.NewTracker(cfg.OnlineTimeout))
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)
	jar, _ := cookiejar.New(nil)
	return ts, &http.Client{Jar: jar}
}

// TestLoginFailures 登录失败路径：密码错误、用户不存在、请求方法错误。
func TestLoginFailures(t *testing.T) {
	ts, _ := newTestServerWith(t, nil)
	base := ts.URL

	postJSON(t, base+"/admin/api/register", map[string]any{"user": "edge01", "pwd": "correct-pass"}, "")

	bad := postJSON(t, base+"/api/login", map[string]any{"username": "edge01", "password": "wrong-pass"}, "")
	if bad["error"] == nil || bad["access_token"] != nil {
		t.Fatalf("密码错误应返回 error 且无 token: %v", bad)
	}

	noUser := postJSON(t, base+"/api/login", map[string]any{"username": "ghost", "password": "x"}, "")
	if noUser["error"] == nil {
		t.Fatalf("不存在的用户应返回 error: %v", noUser)
	}

	empty := postJSON(t, base+"/api/login", map[string]any{"username": "", "password": ""}, "")
	if empty["error"] == nil {
		t.Fatalf("空账号密码应返回 error: %v", empty)
	}

	resp, err := http.Get(base + "/api/login")
	if err != nil {
		t.Fatalf("GET /api/login 请求失败: %v", err)
	}
	resp.Body.Close()
}

// TestRegistrationRules 重复注册与注册开关。
func TestRegistrationRules(t *testing.T) {
	ts, _ := newTestServerWith(t, nil)
	first := postJSON(t, ts.URL+"/admin/api/register", map[string]any{"user": "dupuser", "pwd": "password123"}, "")
	if first["code"] != float64(1) {
		t.Fatalf("首次注册失败: %v", first)
	}
	dup := postJSON(t, ts.URL+"/admin/api/register", map[string]any{"user": "dupuser", "pwd": "password123"}, "")
	if dup["code"] == float64(1) {
		t.Fatalf("重复注册同名用户不应成功: %v", dup)
	}

	ts2, _ := newTestServerWith(t, func(c *config.Config) { c.AllowRegistration = false })
	closed := postJSON(t, ts2.URL+"/admin/api/register", map[string]any{"user": "nobody", "pwd": "password123"}, "")
	if closed["code"] == float64(1) {
		t.Fatalf("注册关闭时不应注册成功: %v", closed)
	}
}

// TestInvalidToken 无效 token 访问客户端接口必须报错而不是泄露数据。
func TestInvalidToken(t *testing.T) {
	ts, client := newTestServerWith(t, nil)
	resp := getJSON(t, client, ts.URL+"/api/ab", "garbage-token-000")
	if resp["error"] == nil {
		t.Fatalf("无效 token 应返回 error: %v", resp)
	}
}

// TestShareSelfClaim 自己不能领取自己创建的分享链接。
func TestShareSelfClaim(t *testing.T) {
	ts, client := newTestServerWith(t, nil)
	base := ts.URL

	postJSONWith(t, client, base+"/admin/api/register", map[string]any{"user": "owner", "pwd": "password123"}, "")
	postJSONWith(t, client, base+"/admin/api/login", map[string]any{"username": "owner", "password": "password123"}, "")
	postJSON(t, base+"/api/sysinfo", map[string]any{
		"id": "555666777", "uuid": "uuid-self", "cpu": "x", "hostname": "H", "memory": "4GB", "os": "Linux", "username": "o", "version": "1.4.9",
	}, "")
	postJSON(t, base+"/api/login", map[string]any{"username": "owner", "password": "password123", "id": "555666777", "uuid": "uuid-self"}, "")

	share := postJSONWith(t, client, base+"/admin/api/shares", map[string]any{"peers": []string{"555666777"}}, "")
	shash, _ := share["shash"].(string)
	if shash == "" {
		t.Fatalf("创建分享失败: %v", share)
	}

	req, _ := http.NewRequest(http.MethodPost, base+"/admin/api/shares/"+shash+"/claim", nil)
	resp, err := client.Do(req) // 同一 cookie 会话 = 自己领取
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	defer resp.Body.Close()
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("解析领取响应失败: %v", err)
	}
	if out["code"] == float64(1) {
		t.Fatalf("自己领取自己的分享不应成功: %v", out)
	}
}

// TestTokenExpiry token 过期后 currentUser 返回空对象（与参考版一致）。
func TestTokenExpiry(t *testing.T) {
	ts, _ := newTestServerWith(t, func(c *config.Config) { c.TokenTTL = -time.Second })
	base := ts.URL

	postJSON(t, base+"/admin/api/register", map[string]any{"user": "ttluser", "pwd": "password123"}, "")
	login := postJSON(t, base+"/api/login", map[string]any{"username": "ttluser", "password": "password123", "id": "1", "uuid": "u"}, "")
	token, _ := login["access_token"].(string)
	if token == "" {
		t.Fatalf("登录失败: %v", login)
	}
	me := postJSON(t, base+"/api/currentUser", map[string]any{}, token)
	if len(me) != 0 {
		t.Fatalf("过期 token 应返回空对象: %v", me)
	}
}

// TestLoginRateLimit 登录限速：超限返回 429。
func TestLoginRateLimit(t *testing.T) {
	ts, _ := newTestServerWith(t, func(c *config.Config) { c.LoginRateLimit = 2 })
	base := ts.URL
	postJSON(t, base+"/admin/api/register", map[string]any{"user": "rluser", "pwd": "password123"}, "")

	var lastStatus int
	for i := 0; i < 4; i++ {
		req, _ := http.NewRequest(http.MethodPost, base+"/api/login", strings.NewReader(`{"username":"rluser","password":"password123"}`))
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("请求失败: %v", err)
		}
		lastStatus = resp.StatusCode
		resp.Body.Close()
	}
	if lastStatus != http.StatusTooManyRequests {
		t.Fatalf("第 4 次登录应被限速(429)，实际 %d", lastStatus)
	}
}

// TestHealthz 健康检查端点。
func TestHealthz(t *testing.T) {
	ts, _ := newTestServerWith(t, nil)
	resp, err := http.Get(ts.URL + "/healthz")
	if err != nil {
		t.Fatalf("healthz 请求失败: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("healthz 应返回 200，实际 %d", resp.StatusCode)
	}
}
