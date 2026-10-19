package api

import (
	"encoding/json"
	"strings"
	"testing"

	"rustdesk-api-go/internal/config"
)

// TestIDConflictDetectionAndBlock 同一 RID 出现不同主机/不同 UUID → 判为冲突并阻止登录。
func TestIDConflictDetectionAndBlock(t *testing.T) {
	ts, client := newTestServerWith(t, func(c *config.Config) { c.IDConflictEnforce = true })
	base := ts.URL

	// 管理员（用于后续查看冲突列表）
	postJSON(t, base+"/admin/api/register", map[string]any{"user": "admin01", "pwd": "password123"}, "")
	postJSONWith(t, client, base+"/admin/api/login", map[string]any{"username": "admin01", "password": "password123"}, "")
	// 普通用户（客户端登录用）
	postJSONWith(t, client, base+"/admin/api/register", map[string]any{"user": "user01", "pwd": "password123"}, "")

	// 第一台机器上报
	sysinfo(t, base, map[string]any{
		"id": "5550001", "uuid": "uuid-aaa", "hostname": "PC-A",
		"cpu": "i5", "memory": "8G", "os": "Windows 11", "username": "alice", "version": "1.4.9",
	})
	// 正常登录应成功
	login := postJSON(t, base+"/api/login", map[string]any{
		"username": "user01", "password": "password123", "id": "5550001", "uuid": "uuid-aaa",
	}, "")
	if login["access_token"] == nil {
		t.Fatalf("无冲突时登录应成功: %v", login)
	}

	// 第二台机器（克隆）用同一个 RID，但 uuid/主机名不同
	sysinfo(t, base, map[string]any{
		"id": "5550001", "uuid": "uuid-bbb", "hostname": "PC-B",
		"cpu": "i5", "memory": "8G", "os": "Windows 11", "username": "bob", "version": "1.4.9",
	})

	// 克隆机（uuid-bbb）登录应被拒绝，并返回提示文案
	blocked := postJSON(t, base+"/api/login", map[string]any{
		"username": "user01", "password": "password123", "id": "5550001", "uuid": "uuid-bbb",
	}, "")
	if blocked["access_token"] != nil {
		t.Fatalf("克隆机不应获得 token: %v", blocked)
	}
	msg, _ := blocked["error"].(string)
	if msg == "" || !strings.Contains(msg, "ID") {
		t.Fatalf("应返回包含 ID 的提示文案: %v", blocked)
	}
	if !strings.Contains(msg, "PC-A") || !strings.Contains(msg, "PC-B") {
		t.Fatalf("提示中应列出冲突主机名: %s", msg)
	}

	// 原机（uuid-aaa，最早注册）必须仍然可以登录，不能被误伤
	primary := postJSON(t, base+"/api/login", map[string]any{
		"username": "user01", "password": "password123", "id": "5550001", "uuid": "uuid-aaa",
	}, "")
	if primary["access_token"] == nil {
		t.Fatalf("原机登录不应被拦截: %v", primary)
	}

	// 管理端能看到冲突记录
	resp, err := client.Get(base + "/admin/api/id-conflicts")
	if err != nil {
		t.Fatalf("请求冲突列表失败: %v", err)
	}
	defer resp.Body.Close()
	var out struct {
		Total int `json:"total"`
		Items []struct {
			RID       string   `json:"rid"`
			Hostnames []string `json:"hostnames"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("解析冲突列表失败: %v", err)
	}
	if out.Total != 1 || out.Items[0].RID != "5550001" || len(out.Items[0].Hostnames) < 2 {
		t.Fatalf("冲突列表异常: %+v", out)
	}
}

// TestIDConflictSameMachineNoFalsePositive 同一台机器重复上报不应误报。
func TestIDConflictSameMachineNoFalsePositive(t *testing.T) {
	ts, client := newTestServerWith(t, func(c *config.Config) { c.IDConflictEnforce = true })
	base := ts.URL
	postJSON(t, base+"/admin/api/register", map[string]any{"user": "admin02", "pwd": "password123"}, "")

	for i := 0; i < 3; i++ {
		sysinfo(t, base, map[string]any{
			"id": "6660001", "uuid": "uuid-same", "hostname": "PC-SAM",
			"cpu": "i5", "memory": "8G", "os": "Win", "username": "u", "version": "1.4.9",
		})
	}
	postJSONWith(t, client, base+"/admin/api/login", map[string]any{"username": "admin02", "password": "password123"}, "")
	resp, err := client.Get(base + "/admin/api/id-conflicts")
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	defer resp.Body.Close()
	var out struct {
		Total int `json:"total"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if out.Total != 0 {
		t.Fatalf("同机重复上报不应产生冲突: %+v", out)
	}
}

func sysinfo(t *testing.T, base string, body map[string]any) {
	t.Helper()
	r := postJSON(t, base+"/api/sysinfo", body, "")
	if r["data"] != "ok" {
		t.Fatalf("sysinfo 失败: %v", r)
	}
}
