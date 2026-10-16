package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"rustdesk-api-go/internal/auth"
	"rustdesk-api-go/internal/config"
	"rustdesk-api-go/internal/database"
	"rustdesk-api-go/internal/stats"
)

// newTestServer 构建带临时 SQLite 数据库的完整服务。
func newTestServer(t *testing.T) (*httptest.Server, *http.Client) {
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
	db, err := database.Open(cfg)
	if err != nil {
		t.Fatalf("打开测试数据库失败: %v", err)
	}
	// Windows 下 SQLite 文件句柄不释放会导致 TempDir 清理失败；
	// 注册顺序保证执行顺序为：关闭测试服务器 -> 关闭数据库 -> 删除临时目录。
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	tokens := auth.NewTokenStore(db, cfg.TokenTTL)
	sessions := auth.NewSessionStore(cfg.SessionTTL)
	tracker := stats.NewTracker(cfg.OnlineTimeout)
	srv := NewServer(cfg, db, tokens, sessions, tracker)

	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)

	jar, _ := cookiejar.New(nil)
	return ts, &http.Client{Jar: jar}
}

func postJSON(t *testing.T, url string, body any, token string) map[string]any {
	return postJSONWith(t, http.DefaultClient, url, body, token)
}

func postJSONWith(t *testing.T, client *http.Client, url string, body any, token string) map[string]any {
	t.Helper()
	var reader io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		reader = strings.NewReader(string(data))
	}
	req, _ := http.NewRequest(http.MethodPost, url, reader)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("请求 %s 失败: %v", url, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("解析 %s 响应失败: %v, body=%s", url, err, raw)
	}
	return out
}

func getJSON(t *testing.T, client *http.Client, url, token string) map[string]any {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("请求 %s 失败: %v", url, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("解析 %s 响应失败: %v, body=%s", url, err, raw)
	}
	return out
}

// TestClientAPIContract 覆盖 RustDesk 客户端契约全流程。
func TestClientAPIContract(t *testing.T) {
	ts, client := newTestServer(t)
	base := ts.URL

	// 1. 设备先通过 sysinfo 注册（模拟客户端安装服务后的资产上报）
	resp := postJSON(t, base+"/api/sysinfo", map[string]any{
		"id": "123456789", "uuid": "uuid-001",
		"cpu": "Intel i7-12700", "hostname": "PC-ALICE",
		"memory": "16GB", "os": "Windows 11", "username": "alice", "version": "1.4.9",
	}, "")
	if resp["data"] != "ok" {
		t.Fatalf("sysinfo 响应异常: %v", resp)
	}

	// 2. 首个注册用户成为管理员
	reg := postJSON(t, base+"/admin/api/register", map[string]any{"user": "admin01", "pwd": "password123"}, "")
	if reg["code"] != float64(1) {
		t.Fatalf("注册失败: %v", reg)
	}

	// 3. 客户端登录：应自动把已注册设备加入地址簿
	login := postJSON(t, base+"/api/login", map[string]any{
		"username": "admin01", "password": "password123",
		"id": "123456789", "uuid": "uuid-001", "autoLogin": true, "type": "client",
	}, "")
	token, _ := login["access_token"].(string)
	if token == "" {
		t.Fatalf("登录未返回 access_token: %v", login)
	}
	if login["type"] != "access_token" {
		t.Fatalf("登录 type 字段异常: %v", login)
	}

	// 4. currentUser 校验 token
	me := postJSON(t, base+"/api/currentUser", map[string]any{}, token)
	if me["name"] != "admin01" {
		t.Fatalf("currentUser 异常: %v", me)
	}

	// 5. 心跳：token 续期 + 在线状态
	hb := postJSON(t, base+"/api/heartbeat", map[string]any{"id": "123456789", "uuid": "uuid-001"}, "")
	if hb["data"] != "在线" {
		t.Fatalf("heartbeat 响应异常: %v", hb)
	}

	// 6. 地址簿读取：登录时自动绑定的设备应出现
	ab := getJSON(t, client, base+"/api/ab", token)
	dataStr, _ := ab["data"].(string)
	if dataStr == "" {
		t.Fatalf("ab 响应缺少 data: %v", ab)
	}
	var abData struct {
		Tags      []string `json:"tags"`
		TagColors string   `json:"tag_colors"`
		Peers     []struct {
			ID   string   `json:"id"`
			Tags []string `json:"tags"`
		} `json:"peers"`
	}
	if err := json.Unmarshal([]byte(dataStr), &abData); err != nil {
		t.Fatalf("ab data 解析失败: %v", err)
	}
	if len(abData.Peers) != 1 || abData.Peers[0].ID != "123456789" {
		t.Fatalf("地址簿应包含自动绑定的设备: %s", dataStr)
	}
	if len(abData.Peers[0].Tags) != 0 {
		t.Fatalf("无标签设备的 tags 应为空数组而非 [\"\"]: %s", dataStr)
	}

	// 7. 地址簿全量同步（先删后插）
	abUpdate := postJSON(t, base+"/api/ab", map[string]any{"data": `{
		"tags": ["办公机"],
		"tag_colors": {"办公机": "16744192"},
		"peers": [{"id": "987654321", "username": "bob", "hostname": "PC-BOB",
		           "alias": "Bob 的电脑", "platform": "Windows", "tags": ["办公机"], "hash": "abc123"}]
	}`}, token)
	if abUpdate["code"] != float64(1) {
		t.Fatalf("ab 更新失败: %v", abUpdate)
	}
	ab2 := getJSON(t, client, base+"/api/ab", token)
	var abData2 struct {
		Tags  []string `json:"tags"`
		Peers []struct {
			ID    string   `json:"id"`
			Alias string   `json:"alias"`
			Tags  []string `json:"tags"`
		} `json:"peers"`
	}
	_ = json.Unmarshal([]byte(ab2["data"].(string)), &abData2)
	if len(abData2.Tags) != 1 || abData2.Tags[0] != "办公机" {
		t.Fatalf("标签同步异常: %v", abData2)
	}
	if len(abData2.Peers) != 1 || abData2.Peers[0].ID != "987654321" || abData2.Peers[0].Alias != "Bob 的电脑" {
		t.Fatalf("设备同步异常: %v", abData2)
	}

	// 8. 审计日志：新建 / 关闭 / 文件传输
	auditNew := postJSON(t, base+"/api/audit", map[string]any{
		"action": "new", "conn_id": 1001, "ip": "10.0.0.5",
		"id": "123456789", "session_id": 55, "uuid": "uuid-001",
	}, "")
	if auditNew["code"] != float64(1) {
		t.Fatalf("audit new 失败: %v", auditNew)
	}
	auditPeer := postJSON(t, base+"/api/audit", map[string]any{
		"conn_id": 1001, "session_id": 55, "peer": []any{"111222333", "x"},
	}, "")
	if auditPeer["code"] != float64(1) {
		t.Fatalf("audit peer 失败: %v", auditPeer)
	}
	auditFile := postJSON(t, base+"/api/audit", map[string]any{
		"is_file": true, "id": "123456789", "path": "/tmp/report.pdf",
		"peer_id": "111222333", "type": 1,
		"info": `{"ip": "10.0.0.6", "files": [["report.pdf", 1048576]]}`,
	}, "")
	if auditFile["code"] != float64(1) {
		t.Fatalf("audit file 失败: %v", auditFile)
	}
	auditClose := postJSON(t, base+"/api/audit", map[string]any{"action": "close", "conn_id": 1001}, "")
	if auditClose["code"] != float64(1) {
		t.Fatalf("audit close 失败: %v", auditClose)
	}

	// 9. 登出后 token 失效
	logout := postJSON(t, base+"/api/logout", map[string]any{"id": "123456789", "uuid": "uuid-001"}, "")
	if logout["code"] != float64(1) {
		t.Fatalf("logout 失败: %v", logout)
	}
	me2 := postJSON(t, base+"/api/currentUser", map[string]any{}, token)
	if len(me2) != 0 {
		t.Fatalf("登出后 currentUser 应返回空对象: %v", me2)
	}
}

// TestAdminAPI 覆盖管理后台：统计、设备列表、分享、用户管理。
func TestAdminAPI(t *testing.T) {
	ts, client := newTestServer(t)
	base := ts.URL

	// 注册并登录管理员（使用 client 带 cookie jar）
	postJSONWith(t, client, base+"/admin/api/register", map[string]any{"user": "root", "pwd": "password123"}, "")
	login := postJSONWith(t, client, base+"/admin/api/login", map[string]any{"username": "root", "password": "password123"}, "")
	if login["code"] != float64(1) {
		t.Fatalf("管理员登录失败: %v", login)
	}

	// 设备上线
	postJSON(t, base+"/api/sysinfo", map[string]any{
		"id": "777888999", "uuid": "uuid-777", "cpu": "Apple M2",
		"hostname": "MacBook", "memory": "8GB", "os": "macOS 15", "username": "carol", "version": "1.4.9",
	}, "")
	postJSON(t, base+"/api/heartbeat", map[string]any{"id": "777888999", "uuid": "uuid-777"}, "")

	// 统计
	statsResp := getJSON(t, client, base+"/admin/api/stats", "")
	if statsResp["devices_total"] != float64(1) {
		t.Fatalf("设备总数统计异常: %v", statsResp)
	}
	if statsResp["devices_online"] != float64(1) {
		t.Fatalf("在线设备统计异常: %v", statsResp)
	}

	// 设备列表（管理员全量视图）
	devices := getJSON(t, client, base+"/admin/api/devices?all=1", "")
	items, _ := devices["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("设备列表异常: %v", devices)
	}
	first, _ := items[0].(map[string]any)
	if first["rid"] != "777888999" || first["status"] != "online" {
		t.Fatalf("设备行异常: %v", first)
	}

	// 连接日志
	postJSON(t, base+"/api/audit", map[string]any{
		"action": "new", "conn_id": 2002, "ip": "10.0.0.9",
		"id": "777888999", "session_id": 7, "uuid": "uuid-777",
	}, "")
	connLogs := getJSON(t, client, base+"/admin/api/conn_logs", "")
	if connLogs["total"] != float64(1) {
		t.Fatalf("连接日志异常: %v", connLogs)
	}

	// 分享链接：root 先通过客户端登录绑定设备，再分享给第二个用户
	rootClientLogin := postJSON(t, base+"/api/login", map[string]any{
		"username": "root", "password": "password123", "id": "777888999", "uuid": "uuid-777",
	}, "")
	if rootClientLogin["access_token"] == nil {
		t.Fatalf("root 客户端登录失败: %v", rootClientLogin)
	}
	share := postJSONWith(t, client, base+"/admin/api/shares", map[string]any{"peers": []string{"777888999"}}, "")
	shash, _ := share["shash"].(string)
	if shash == "" {
		t.Fatalf("分享链接创建失败: %v", share)
	}

	// 第二个用户注册并领取
	postJSON(t, base+"/admin/api/register", map[string]any{"user": "guest01", "pwd": "password123"}, "")
	jar2, _ := cookiejar.New(nil)
	client2 := &http.Client{Jar: jar2}
	postJSONWith(t, client2, base+"/admin/api/login", map[string]any{"username": "guest01", "password": "password123"}, "")

	req, _ := http.NewRequest(http.MethodPost, base+"/admin/api/shares/"+shash+"/claim", nil)
	resp, err := client2.Do(req)
	if err != nil {
		t.Fatalf("领取分享失败: %v", err)
	}
	resp.Body.Close()
	// 领取后 guest01 的地址簿应包含该设备
	guestLogin := postJSON(t, base+"/api/login", map[string]any{
		"username": "guest01", "password": "password123", "id": "999", "uuid": "uuid-999",
	}, "")
	guestToken, _ := guestLogin["access_token"].(string)
	guestAB := getJSON(t, client, base+"/api/ab", guestToken)
	var guestABData struct {
		Peers []struct {
			ID string `json:"id"`
		} `json:"peers"`
	}
	_ = json.Unmarshal([]byte(guestAB["data"].(string)), &guestABData)
	if len(guestABData.Peers) != 1 || guestABData.Peers[0].ID != "777888999" {
		t.Fatalf("领取分享后地址簿异常: %s", guestAB["data"])
	}

	// 用户管理
	users := getJSON(t, client, base+"/admin/api/users", "")
	if users["items"] == nil {
		t.Fatalf("用户列表异常: %v", users)
	}
	create := postJSONWith(t, client, base+"/admin/api/users", map[string]any{
		"username": "newuser01", "password": "password123", "is_admin": false,
	}, "")
	if create["code"] != float64(1) {
		t.Fatalf("创建用户失败: %v", create)
	}

	// Excel 导出
	req2, _ := http.NewRequest(http.MethodGet, base+"/admin/api/devices/export", nil)
	resp2, err := client.Do(req2)
	if err != nil {
		t.Fatalf("导出请求失败: %v", err)
	}
	defer resp2.Body.Close()
	if ct := resp2.Header.Get("Content-Type"); !strings.Contains(ct, "spreadsheet") {
		t.Fatalf("导出 Content-Type 异常: %s", ct)
	}
	body, _ := io.ReadAll(resp2.Body)
	if len(body) < 1000 {
		t.Fatalf("导出文件过小，可能不是合法 xlsx: %d bytes", len(body))
	}
}

// TestUnauthenticatedAccess 未登录访问管理接口必须返回 401。
func TestUnauthenticatedAccess(t *testing.T) {
	ts, client := newTestServer(t)
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/admin/api/devices", nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("未登录应返回 401，实际 %d", resp.StatusCode)
	}
}

// TestConvertFilesize 文件大小格式化与参考版一致。
func TestConvertFilesize(t *testing.T) {
	cases := map[int64]string{
		0:          "0B",
		512:        "512.00 B",
		1048576:    "1.00 MB",
		1572864:    "1.50 MB",
		1073741824: "1.00 GB",
	}
	for in, want := range cases {
		if got := convertFilesize(in); got != want {
			t.Errorf("convertFilesize(%d) = %s, want %s", in, got, want)
		}
	}
}

// TestParseTagColors 兼容 tag_colors 的对象与双重编码字符串两种形态。
func TestParseTagColors(t *testing.T) {
	obj := parseTagColors([]byte(`{"a":"16744192"}`))
	if obj["a"] != "16744192" {
		t.Errorf("对象形态解析失败: %v", obj)
	}
	str := parseTagColors([]byte(`"{\"b\":\"255\"}"`))
	if str["b"] != "255" {
		t.Errorf("字符串形态解析失败: %v", str)
	}
}
