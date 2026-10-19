package console

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/hybridsakura/prism-go-panel/internal/auth"
	"github.com/hybridsakura/prism-go-panel/internal/config"
	"github.com/hybridsakura/prism-go-panel/internal/database"
	"github.com/hybridsakura/prism-go-panel/internal/models"
	"github.com/hybridsakura/prism-go-panel/internal/platform"
)

func newConsoleTestEnv(t *testing.T) (*Handler, *platform.Core, *platform.AgentPresence, *auth.SessionStore, *httptest.Server) {
	t.Helper()
	cfg := &config.Config{
		DBURL:             filepath.Join(t.TempDir(), "console.db"),
		AllowRegistration: true,
		TokenTTL:          time.Hour,
		OnlineTimeout:     120 * time.Second,
		SessionTTL:        time.Hour,
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

	core := platform.New(cfg, db, nil)
	sessions := auth.NewSessionStore(cfg.SessionTTL)
	presence := platform.NewAgentPresence()
	h := New(core, sessions, presence)

	r := chi.NewRouter()
	h.Mount(r)
	ts := httptest.NewServer(r)
	t.Cleanup(ts.Close)

	return h, core, presence, sessions, ts
}

func adminCookie(t *testing.T, core *platform.Core, sessions *auth.SessionStore) *http.Cookie {
	t.Helper()
	adminUser := models.User{
		Username:     "admin_test",
		PasswordHash: "any",
		IsActive:     true,
		IsAdmin:      true,
	}
	if err := core.DB.Create(&adminUser).Error; err != nil {
		t.Fatalf("创建管理员失败: %v", err)
	}
	sid, err := sessions.Create(adminUser.ID)
	if err != nil {
		t.Fatalf("创建管理员会话失败: %v", err)
	}
	return &http.Cookie{
		Name:  sessionCookieName,
		Value: sid,
	}
}

func TestEnrollTokensManagement(t *testing.T) {
	_, core, _, sessions, ts := newConsoleTestEnv(t)
	cookie := adminCookie(t, core, sessions)

	// 1. 创建注册码
	createBody := map[string]any{
		"group_name":   "检验科",
		"max_uses":     5,
		"expires_days": 7,
		"note":         "检验科批量安装",
	}
	buf, _ := json.Marshal(createBody)
	req, _ := http.NewRequest("POST", ts.URL+"/admin/api/enroll-tokens", bytes.NewReader(buf))
	req.AddCookie(cookie)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("创建注册码期望 200，实际 %d", resp.StatusCode)
	}
	var res map[string]any
	json.NewDecoder(resp.Body).Decode(&res)
	rawToken, _ := res["enroll_token"].(string)
	if rawToken == "" {
		t.Fatalf("应返回明文注册码: %v", res)
	}
	tokenID := uint(res["id"].(float64))

	// 2. 查询注册码列表
	reqList, _ := http.NewRequest("GET", ts.URL+"/admin/api/enroll-tokens", nil)
	reqList.AddCookie(cookie)
	respList, err := http.DefaultClient.Do(reqList)
	if err != nil {
		t.Fatalf("请求列表失败: %v", err)
	}
	defer respList.Body.Close()

	var listRes struct {
		Total int `json:"total"`
		Items []struct {
			ID        uint   `json:"id"`
			GroupName string `json:"group_name"`
			MaxUses   int    `json:"max_uses"`
		} `json:"items"`
	}
	json.NewDecoder(respList.Body).Decode(&listRes)
	if listRes.Total != 1 || listRes.Items[0].GroupName != "检验科" {
		t.Fatalf("列表查询结果异常: %+v", listRes)
	}

	// 3. 删除注册码
	reqDel, _ := http.NewRequest("DELETE", ts.URL+fmt.Sprintf("/admin/api/enroll-tokens/%d", tokenID), nil)
	reqDel.AddCookie(cookie)
	respDel, err := http.DefaultClient.Do(reqDel)
	if err != nil {
		t.Fatalf("删除请求失败: %v", err)
	}
	defer respDel.Body.Close()
	if respDel.StatusCode != http.StatusOK {
		t.Fatalf("删除期望 200，实际 %d", respDel.StatusCode)
	}
}

func TestAgentListAndStatusToggle(t *testing.T) {
	_, core, presence, sessions, ts := newConsoleTestEnv(t)
	cookie := adminCookie(t, core, sessions)

	// 手工造一个 Agent
	agent := models.Agent{
		TokenHash: "dummy-hash",
		Hostname:  "PC-ICU-01",
		OS:        "windows",
		Arch:      "amd64",
		Version:   "0.1.0",
		RID:       "999888777",
		IP:        "172.18.20.101",
		GroupName: "ICU",
		Status:    "active",
	}
	if err := core.DB.Create(&agent).Error; err != nil {
		t.Fatalf("创建 Agent 记录失败: %v", err)
	}

	// 给该 Agent 灌一个实时心跳
	presence.Touch(agent.ID, time.Now())

	// 1. 查询 Agent 列表
	req, _ := http.NewRequest("GET", ts.URL+"/admin/api/agents", nil)
	req.AddCookie(cookie)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	defer resp.Body.Close()

	var out struct {
		Total int `json:"total"`
		Items []struct {
			ID       uint   `json:"id"`
			Hostname string `json:"hostname"`
			Online   bool   `json:"online"`
			Status   string `json:"status"`
		} `json:"items"`
	}
	json.NewDecoder(resp.Body).Decode(&out)
	if out.Total != 1 || !out.Items[0].Online || out.Items[0].Status != "active" {
		t.Fatalf("查询 Agent 列表异常: %+v", out)
	}

	// 2. 禁用 Agent
	toggleBody, _ := json.Marshal(map[string]any{"status": "disabled"})
	reqToggle, _ := http.NewRequest("POST", ts.URL+fmt.Sprintf("/admin/api/agents/%d/status", agent.ID), bytes.NewReader(toggleBody))
	reqToggle.AddCookie(cookie)
	reqToggle.Header.Set("Content-Type", "application/json")
	respToggle, err := http.DefaultClient.Do(reqToggle)
	if err != nil {
		t.Fatalf("禁用请求失败: %v", err)
	}
	defer respToggle.Body.Close()
	if respToggle.StatusCode != http.StatusOK {
		t.Fatalf("期望 200，实际 %d", respToggle.StatusCode)
	}

	var updated models.Agent
	core.DB.First(&updated, agent.ID)
	if updated.Status != "disabled" {
		t.Fatalf("状态未更新为 disabled: %+v", updated)
	}
}
