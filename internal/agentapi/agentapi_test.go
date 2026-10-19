package agentapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/hybridsakura/prism-go-panel/internal/config"
	"github.com/hybridsakura/prism-go-panel/internal/database"
	"github.com/hybridsakura/prism-go-panel/internal/models"
	"github.com/hybridsakura/prism-go-panel/internal/platform"
)

// ---------- 测试基建 ----------

type agentEnv struct {
	srv      *httptest.Server
	db       any
	core     *platform.Core
	presence *platform.AgentPresence
}

func newAgentEnv(t *testing.T) (*agentEnv, func(method, path string, body any, token string) (int, map[string]any)) {
	t.Helper()
	cfg := &config.Config{
		DBURL:             filepath.Join(t.TempDir(), "agent.db"),
		AllowRegistration: true,
		TokenTTL:          time.Hour,
		OnlineTimeout:     120 * time.Second,
		SessionTTL:        time.Hour,
	}
	db, err := database.Open(cfg)
	if err != nil {
		t.Fatalf("打开测试库失败: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	core := platform.New(cfg, db, nil)
	presence := platform.NewAgentPresence()
	h := New(core, presence)

	r := chi.NewRouter()
	h.Mount(r)
	ts := httptest.NewServer(r)
	t.Cleanup(ts.Close)

	call := func(method, path string, body any, token string) (int, map[string]any) {
		var buf bytes.Buffer
		if body != nil {
			_ = json.NewEncoder(&buf).Encode(body)
		}
		req, err := http.NewRequest(method, ts.URL+path, &buf)
		if err != nil {
			t.Fatalf("构造请求失败: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("请求失败: %v", err)
		}
		defer resp.Body.Close()
		var out map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&out)
		return resp.StatusCode, out
	}
	return &agentEnv{srv: ts, db: db, core: core, presence: presence}, call
}

// createToken 直接写库创建注册码，返回明文。
func createToken(t *testing.T, core *platform.Core, maxUses int, expires *time.Time) string {
	t.Helper()
	raw := "tok-" + time.Now().Format("150405.000000000")
	row := models.EnrollToken{
		TokenHash: platform.HashToken(raw),
		MaxUses:   maxUses,
		ExpiresAt: expires,
	}
	if err := core.DB.Create(&row).Error; err != nil {
		t.Fatalf("创建注册码失败: %v", err)
	}
	return raw
}

func enrollBody(token string) map[string]any {
	return map[string]any{
		"enroll_token": token,
		"hostname":     "PC-TEST-01",
		"os":           "windows",
		"arch":         "amd64",
		"version":      "0.1.0",
		"rid":          "123456789",
		"fingerprint":  "fp-abc",
	}
}

// ---------- F1 设备注册 ----------

func TestEnrollSuccess(t *testing.T) {
	env, call := newAgentEnv(t)
	token := createToken(t, env.core, 1, nil)

	code, body := call("POST", "/agent/v1/enroll", enrollBody(token), "")
	if code != http.StatusOK {
		t.Fatalf("期望 200，实际 %d: %v", code, body)
	}
	if body["device_token"] == nil || body["device_token"] == "" {
		t.Fatalf("应返回设备令牌: %v", body)
	}
	if body["agent_id"] == nil {
		t.Fatalf("应返回 agent_id: %v", body)
	}

	var agent models.Agent
	if err := env.core.DB.Where("hostname = ?", "PC-TEST-01").First(&agent).Error; err != nil {
		t.Fatalf("Agent 未落库: %v", err)
	}
	if agent.RID != "123456789" || agent.Status != "active" {
		t.Fatalf("Agent 字段异常: %+v", agent)
	}
	// 库里只应存哈希，不能存明文
	if agent.TokenHash == body["device_token"] {
		t.Fatal("数据库不应保存明文令牌")
	}
}

func TestEnrollRejectsInvalidToken(t *testing.T) {
	_, call := newAgentEnv(t)
	code, body := call("POST", "/agent/v1/enroll", enrollBody("not-a-real-token"), "")
	if code != http.StatusForbidden {
		t.Fatalf("无效注册码应返回 403，实际 %d: %v", code, body)
	}
}

func TestEnrollRejectsExpiredToken(t *testing.T) {
	env, call := newAgentEnv(t)
	past := time.Now().Add(-time.Minute)
	token := createToken(t, env.core, 10, &past)

	code, _ := call("POST", "/agent/v1/enroll", enrollBody(token), "")
	if code != http.StatusForbidden {
		t.Fatalf("过期注册码应返回 403，实际 %d", code)
	}
}

func TestEnrollRejectsExhaustedToken(t *testing.T) {
	env, call := newAgentEnv(t)
	token := createToken(t, env.core, 1, nil)

	if code, _ := call("POST", "/agent/v1/enroll", enrollBody(token), ""); code != http.StatusOK {
		t.Fatalf("首次注册应成功，实际 %d", code)
	}
	// 第二台机器用同一注册码
	second := enrollBody(token)
	second["hostname"] = "PC-TEST-02"
	second["fingerprint"] = "fp-xyz"
	code, _ := call("POST", "/agent/v1/enroll", second, "")
	if code != http.StatusForbidden {
		t.Fatalf("超出使用次数应返回 403，实际 %d", code)
	}
}

func TestEnrollIdempotentByFingerprint(t *testing.T) {
	env, call := newAgentEnv(t)
	token := createToken(t, env.core, 1, nil)

	code1, body1 := call("POST", "/agent/v1/enroll", enrollBody(token), "")
	if code1 != http.StatusOK {
		t.Fatalf("首次注册失败: %d %v", code1, body1)
	}
	// 同一台机器重复注册（如重装 Agent），应返回同一 agent_id，且不再消耗次数
	code2, body2 := call("POST", "/agent/v1/enroll", enrollBody(token), "")
	if code2 != http.StatusOK {
		t.Fatalf("重复注册应幂等成功，实际 %d: %v", code2, body2)
	}
	if body1["agent_id"] != body2["agent_id"] {
		t.Fatalf("重复注册应返回同一 agent_id: %v vs %v", body1["agent_id"], body2["agent_id"])
	}
	var count int64
	env.core.DB.Model(&models.Agent{}).Count(&count)
	if count != 1 {
		t.Fatalf("重复注册不应新增 Agent，实际 %d 条", count)
	}
}

// ---------- F2 心跳 ----------

func TestHeartbeatRequiresToken(t *testing.T) {
	_, call := newAgentEnv(t)
	code, _ := call("POST", "/agent/v1/heartbeat", map[string]any{"version": "0.1.0"}, "")
	if code != http.StatusUnauthorized {
		t.Fatalf("无令牌应返回 401，实际 %d", code)
	}
	code, _ = call("POST", "/agent/v1/heartbeat", map[string]any{"version": "0.1.0"}, "bogus")
	if code != http.StatusUnauthorized {
		t.Fatalf("错误令牌应返回 401，实际 %d", code)
	}
}

func TestHeartbeatReturnsInterval(t *testing.T) {
	env, call := newAgentEnv(t)
	token := createToken(t, env.core, 1, nil)
	_, body := call("POST", "/agent/v1/enroll", enrollBody(token), "")
	deviceToken, _ := body["device_token"].(string)

	code, hb := call("POST", "/agent/v1/heartbeat", map[string]any{
		"version": "0.1.0", "rid": "123456789", "hostname": "PC-TEST-01",
	}, deviceToken)
	if code != http.StatusOK {
		t.Fatalf("心跳应成功，实际 %d: %v", code, hb)
	}
	if got, _ := hb["interval"].(float64); got != float64(platform.HeartbeatInterval/time.Second) {
		t.Fatalf("心跳间隔应为 %v 秒，实际 %v", platform.HeartbeatInterval/time.Second, hb["interval"])
	}
	if hb["server_time"] == nil {
		t.Fatalf("应返回 server_time: %v", hb)
	}
}

// TestHeartbeatDoesNotWriteDB 心跳只更新内存，落盘由 Flush 负责（保护 Atom 上的 SQLite）。
func TestHeartbeatDoesNotWriteDB(t *testing.T) {
	env, call := newAgentEnv(t)
	token := createToken(t, env.core, 1, nil)
	_, body := call("POST", "/agent/v1/enroll", enrollBody(token), "")
	deviceToken, _ := body["device_token"].(string)
	agentID := uint(body["agent_id"].(float64))

	code, hb := call("POST", "/agent/v1/heartbeat", map[string]any{"version": "0.2.0"}, deviceToken)
	if code != http.StatusOK {
		t.Fatalf("心跳失败: %d %v", code, hb)
	}

	// 数据库里的 last_seen_at 仍未更新
	var agent models.Agent
	if err := env.core.DB.First(&agent, agentID).Error; err != nil {
		t.Fatalf("查询 Agent 失败: %v", err)
	}
	if agent.LastSeenAt != nil {
		t.Fatalf("心跳不应直接写库，last_seen_at=%v", agent.LastSeenAt)
	}
	// 内存里已记录
	if _, ok := env.presence.LastSeen(agentID); !ok {
		t.Fatal("内存中应有该 Agent 的最近心跳")
	}
	// Flush 后才落盘
	if err := env.presence.Flush(env.core.DB); err != nil {
		t.Fatalf("Flush 失败: %v", err)
	}
	env.core.DB.First(&agent, agentID)
	if agent.LastSeenAt == nil {
		t.Fatal("Flush 后应写入 last_seen_at")
	}
}

func TestDisabledAgentRejected(t *testing.T) {
	env, call := newAgentEnv(t)
	token := createToken(t, env.core, 1, nil)
	_, body := call("POST", "/agent/v1/enroll", enrollBody(token), "")
	deviceToken, _ := body["device_token"].(string)

	env.core.DB.Model(&models.Agent{}).Where("1 = 1").Update("status", "disabled")

	code, _ := call("POST", "/agent/v1/heartbeat", map[string]any{"version": "0.1.0"}, deviceToken)
	if code != http.StatusUnauthorized {
		t.Fatalf("被禁用的 Agent 应返回 401，实际 %d", code)
	}
}
