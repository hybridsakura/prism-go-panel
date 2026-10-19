package api

import (
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

// loginAdmin 注册并登录首个用户（自动成为管理员），返回带会话的 client。
func loginAdmin(t *testing.T, base string, client *http.Client) {
	t.Helper()
	reg := postJSONWith(t, client, base+"/admin/api/register", map[string]any{"user": "adminuser", "pwd": "password123"}, "")
	if reg["code"] != float64(1) {
		t.Fatalf("注册管理员失败: %v", reg)
	}
	login := postJSONWith(t, client, base+"/admin/api/login", map[string]any{"username": "adminuser", "password": "password123"}, "")
	if login["code"] != float64(1) {
		t.Fatalf("管理员登录失败: %v", login)
	}
}

func putJSON(t *testing.T, client *http.Client, url string, body any) map[string]any {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPut, url, strings.NewReader(mustJSON(body)))
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("PUT %s 失败: %v", url, err)
	}
	defer resp.Body.Close()
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("解析 PUT %s 响应失败: %v", url, err)
	}
	return out
}

func mustJSON(v any) string {
	data, _ := json.Marshal(v)
	return string(data)
}

// TestServerProfileFlow 档案读写 + 客户端配置生成（自定义端口）。
func TestServerProfileFlow(t *testing.T) {
	ts, client := newTestServerWith(t, nil)
	base := ts.URL
	loginAdmin(t, base, client)

	// 初始为空档案，但有建议值
	p0 := getJSON(t, client, base+"/admin/api/server-profile", "")
	if p0["id_server"] != "" {
		t.Fatalf("初始档案应为空: %v", p0)
	}
	if p0["suggested_id_server"] == nil || p0["suggested_id_server"] == "" {
		t.Fatalf("应返回建议的 ID 服务器地址: %v", p0)
	}

	// 未填写 ID 服务器时不能生成配置
	cc0 := getJSON(t, client, base+"/admin/api/server-profile/client-config", "")
	if cc0["code"] != float64(0) {
		t.Fatalf("无 ID 服务器时应拒绝生成: %v", cc0)
	}

	// 非法格式被拒绝
	bad := putJSON(t, client, base+"/admin/api/server-profile", map[string]any{"id_server": "bad host!"})
	if bad["code"] != float64(0) {
		t.Fatalf("非法地址应被拒绝: %v", bad)
	}

	// 保存：自定义端口 31000 + 自定义中继 32000 + key
	ok := putJSON(t, client, base+"/admin/api/server-profile", map[string]any{
		"id_server": "rust.example.com:31000", "relay_server": "rust.example.com:32000",
		"api_server": "rust.example.com:21114", "key": "O2VuMnkNyGdVBbGNp6TQNJHZYCbXzOGfNiMjcBfLkZ4=",
	})
	if ok["code"] != float64(1) {
		t.Fatalf("保存档案失败: %v", ok)
	}
	p1 := getJSON(t, client, base+"/admin/api/server-profile", "")
	if p1["id_server"] != "rust.example.com:31000" || p1["relay_server"] != "rust.example.com:32000" {
		t.Fatalf("档案读取与保存不一致: %v", p1)
	}

	// 生成客户端配置
	cc := getJSON(t, client, base+"/admin/api/server-profile/client-config", "")
	if cc["code"] != float64(1) {
		t.Fatalf("生成失败: %v", cc)
	}
	wantFN := "rustdesk-host=rust.example.com:31000,key=O2VuMnkNyGdVBbGNp6TQNJHZYCbXzOGfNiMjcBfLkZ4=.exe"
	if cc["filename"] != wantFN {
		t.Fatalf("重命名串不正确:\n got %v\nwant %v", cc["filename"], wantFN)
	}
	if qs, _ := cc["filename_elevate"].(string); qs == "" || qs[:len(qs)-8] == "" {
		t.Fatalf("提权变体缺失: %v", cc)
	}

	// 端口推导：31000 基准 → TCP 30999/31000/31002 + UDP 31000；中继 32000/32002；API 21114
	ports, _ := cc["ports"].([]any)
	got := map[string]bool{}
	for _, pi := range ports {
		m, _ := pi.(map[string]any)
		key := m["service"].(string) + "|" + m["proto"].(string) + "|" + strconv.Itoa(int(m["port"].(float64)))
		got[key] = true
	}
	expect := []string{"hbbs|TCP|30999", "hbbs|TCP|31000", "hbbs|UDP|31000", "hbbs|TCP|31002", "hbbr|TCP|32000", "hbbr|TCP|32002", "api|TCP|21114"}
	for _, e := range expect {
		if !got[e] {
			t.Fatalf("端口推导缺少 %s，实际 %v", e, got)
		}
	}

	// Actions 编译 secrets
	build, _ := cc["custom_build"].(map[string]any)
	if build["RENDEZVOUS_SERVER"] != "rust.example.com" || build["RS_PUB_KEY"] == "" {
		t.Fatalf("编译 secrets 异常: %v", build)
	}
}

// TestProfileCheck 连通性自检：本地监听一个端口模拟 hbbs。
func TestProfileCheck(t *testing.T) {
	ts, client := newTestServerWith(t, nil)
	base := ts.URL
	loginAdmin(t, base, client)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("监听失败: %v", err)
	}
	defer ln.Close()
	_, portStr, _ := net.SplitHostPort(ln.Addr().String())
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()

	putJSON(t, client, base+"/admin/api/server-profile", map[string]any{"id_server": "127.0.0.1:" + portStr})

	req, _ := http.NewRequest(http.MethodPost, base+"/admin/api/server-profile/check", nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("check 请求失败: %v", err)
	}
	defer resp.Body.Close()
	var out struct {
		Code    int `json:"code"`
		Results []struct {
			Addr string `json:"addr"`
			OK   bool   `json:"ok"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("解析 check 响应失败: %v", err)
	}
	if out.Code != 1 || len(out.Results) == 0 {
		t.Fatalf("check 响应异常: %+v", out)
	}
	var found bool
	for _, r := range out.Results {
		if r.Addr == "127.0.0.1:"+portStr {
			found = true
			if !r.OK {
				t.Fatalf("监听中的端口应可达: %+v", r)
			}
		}
	}
	if !found {
		t.Fatalf("结果中缺少被测端口: %+v", out.Results)
	}
}

// TestProfileAdminOnly 非管理员不能访问配置档案接口。
func TestProfileAdminOnly(t *testing.T) {
	ts, client := newTestServerWith(t, nil)
	base := ts.URL
	loginAdmin(t, base, client)

	// 注册第二个（非管理员）用户，换成它的会话
	postJSON(t, base+"/admin/api/register", map[string]any{"user": "normaluser", "pwd": "password123"}, "")
	postJSONWith(t, client, base+"/admin/api/logout", nil, "")
	postJSONWith(t, client, base+"/admin/api/login", map[string]any{"username": "normaluser", "password": "password123"}, "")

	resp, err := client.Get(base + "/admin/api/server-profile")
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("普通用户访问档案接口应被 403/401，实际 %d", resp.StatusCode)
	}
}
