package api

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/hybridsakura/prism-go-panel/internal/models"
)

// 默认端口约定（与官方 rustdesk-server 一致）：
// hbbs 以 21116 为基准：TCP 21115/21116/21118（-1/+0/+2），UDP 21116；
// hbbr 以 21117 为基准：TCP 21117/21119（+0/+2）。
const (
	defaultRendezvousPort = 21116
	defaultRelayPort      = 21117
	defaultAPIPort        = 21114
)

// hostPortRe 校验 host[:port] 形式（域名或 IP，可省略端口）。
var hostPortRe = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9.\-_]*[a-zA-Z0-9])?(:\d{1,5})?$`)

// ---------- 档案读写 ----------

func (s *Server) loadProfile() *models.ServerProfile {
	p := &models.ServerProfile{}
	if err := s.db.First(p, 1).Error; err != nil {
		p.ID = 1
	}
	return p
}

func (s *Server) handleProfileGet(w http.ResponseWriter, r *http.Request) {
	p := s.loadProfile()
	resp := map[string]any{
		"id_server":    p.IDServer,
		"relay_server": p.RelayServer,
		"api_server":   p.APIServer,
		"key":          p.Key,
		"updated_at":   formatTime(p.UpdatedAt),
		// 建议值：当前访问主机（管理员通过什么地址访问后台，客户端通常也走这个地址）
		"suggested_id_server": hostWithoutPort(r.Host),
	}
	if k := s.detectPublicKey(); k != "" && p.Key == "" {
		resp["detected_key"] = k // 通过 HBBS_KEY_FILE 探测到的公钥，仅档案为空时提示
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleProfilePut(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDServer    string `json:"id_server"`
		RelayServer string `json:"relay_server"`
		APIServer   string `json:"api_server"`
		Key         string `json:"key"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"code": 0, "msg": "请求格式错误"})
		return
	}
	for name, v := range map[string]string{
		"ID 服务器": req.IDServer, "中继服务器": req.RelayServer, "API 服务器": req.APIServer,
	} {
		v = strings.TrimSpace(v)
		if v != "" && !hostPortRe.MatchString(v) {
			writeJSON(w, http.StatusOK, map[string]any{"code": 0, "msg": name + "格式不正确，应为 host 或 host:port"})
			return
		}
	}
	key := strings.TrimSpace(req.Key)
	if key != "" && (len(key) < 32 || len(key) > 100) {
		writeJSON(w, http.StatusOK, map[string]any{"code": 0, "msg": "公钥长度异常，应为 id_ed25519.pub 的内容"})
		return
	}

	p := s.loadProfile()
	p.IDServer = strings.TrimSpace(req.IDServer)
	p.RelayServer = strings.TrimSpace(req.RelayServer)
	p.APIServer = strings.TrimSpace(req.APIServer)
	p.Key = key
	if err := s.db.Save(p).Error; err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 1})
}

// detectPublicKey 从 HBBS_KEY_FILE（hbbs 数据目录中的 id_ed25519.pub）读取公钥。
// 一体部署时把 hbbs 目录挂载进来即可免手填。
func (s *Server) detectPublicKey() string {
	if s.cfg.HBBSKeyFile == "" {
		return ""
	}
	data, err := os.ReadFile(s.cfg.HBBSKeyFile)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// ---------- 端口推导 ----------

type portInfo struct {
	Service string `json:"service"` // hbbs / hbbr / api
	Proto   string `json:"proto"`   // TCP / UDP
	Port    int    `json:"port"`
	Usage   string `json:"usage"`
}

// splitHostPort 把 host[:port] 拆成主机与端口；port 缺省时返回 def。
func splitHostPort(hp string, def int) (string, int) {
	hp = strings.TrimSpace(hp)
	if hp == "" {
		return "", def
	}
	if i := strings.LastIndexByte(hp, ':'); i >= 0 {
		if n, err := strconv.Atoi(hp[i+1:]); err == nil && n > 0 && n <= 65535 {
			return hp[:i], n
		}
	}
	return hp, def
}

// derivePorts 按档案推导应开放的端口清单。
func derivePorts(p *models.ServerProfile) []portInfo {
	_, rp := splitHostPort(p.IDServer, defaultRendezvousPort)
	_, qp := splitHostPort(p.RelayServer, defaultRelayPort)
	_, ap := splitHostPort(p.APIServer, defaultAPIPort)
	return []portInfo{
		{"hbbs", "TCP", rp - 1, "NAT 类型测试"},
		{"hbbs", "TCP", rp, "ID 注册 / 心跳（RENDEZVOUS_PORT，hbbs -p 基准）"},
		{"hbbs", "UDP", rp, "ID 注册 / 打洞"},
		{"hbbs", "TCP", rp + 2, "WebSocket 信令（网页客户端）"},
		{"hbbr", "TCP", qp, "流量中继（RELAY_PORT，hbbr -p 基准）"},
		{"hbbr", "TCP", qp + 2, "WebSocket 中继（网页客户端）"},
		{"api", "TCP", ap, "本服务（账号 / 地址簿 / 资产 / 后台）"},
	}
}

// ---------- 客户端配置生成 ----------

func (s *Server) handleClientConfig(w http.ResponseWriter, r *http.Request) {
	p := s.loadProfile()
	if p.IDServer == "" {
		writeJSON(w, http.StatusOK, map[string]any{"code": 0, "msg": "请先填写 ID 服务器地址"})
		return
	}
	key := p.Key
	if key == "" {
		key = s.detectPublicKey()
	}

	// Windows 重命名法：host 与 key 缺一不可（官方文档要求）
	filename := ""
	if key != "" {
		filename = fmt.Sprintf("rustdesk-host=%s,key=%s.exe", p.IDServer, key)
	}
	relay := p.RelayServer
	idHost, _ := splitHostPort(p.IDServer, defaultRendezvousPort)

	writeJSON(w, http.StatusOK, map[string]any{
		"code":             1,
		"filename":         filename,             // rustdesk-host=...,key=....exe
		"filename_elevate": filenameQS(filename), // 追加 ,-qs 自动提权变体
		"manual": map[string]string{ // 客户端「ID/中继服务器」界面手填内容
			"id_server":    p.IDServer,
			"relay_server": relay,
			"api_server":   p.APIServer,
			"key":          key,
		},
		"ports": derivePorts(p),
		"custom_build": map[string]string{ // GitHub Actions 编译自定义客户端所需的 secrets
			"RENDEZVOUS_SERVER": idHost,
			"RS_PUB_KEY":        key,
		},
		"notes": []string{
			"重命名法（Windows）：把下载的 rustdesk.exe 改名为上面的 filename，双击即自动完成配置；host 与 key 缺一不可。",
			"中继与 API 地址留空时客户端会自动推导（中继=ID 服务器主机:21117）。",
			"hbbs 端口以其 -p 参数为基准（默认 21116）：TCP 为基准-1/+0/+2，UDP 为基准+0；hbbr 同理（默认 21117：+0/+2）。",
			"修改 hbbs/hbbr 实际端口需调整其启动参数并重启；本页档案只用于对接与生成客户端配置。",
			"hbbs/hbbr 启动加 -k _ 可强制仅接受带 key 的加密连接。",
		},
	})
}

func filenameQS(filename string) string {
	if filename == "" {
		return ""
	}
	return strings.TrimSuffix(filename, ".exe") + ",-qs.exe"
}

// ---------- 连通性自检 ----------

func (s *Server) handleProfileCheck(w http.ResponseWriter, r *http.Request) {
	p := s.loadProfile()
	if p.IDServer == "" {
		writeJSON(w, http.StatusOK, map[string]any{"code": 0, "msg": "请先填写 ID 服务器地址"})
		return
	}
	idHost, _ := splitHostPort(p.IDServer, defaultRendezvousPort)
	relayHost, _ := splitHostPort(p.RelayServer, defaultRelayPort)
	if p.RelayServer == "" {
		relayHost = idHost
	}
	apiHost, _ := splitHostPort(p.APIServer, defaultAPIPort)
	if p.APIServer == "" {
		apiHost = hostWithoutPort(r.Host)
	}

	type checkResult struct {
		Name    string `json:"name"`
		Addr    string `json:"addr"`
		OK      bool   `json:"ok"`
		Latency string `json:"latency"`
		Error   string `json:"error"`
	}
	var results []checkResult
	for _, pi := range derivePorts(p) {
		if pi.Proto == "UDP" {
			continue // UDP 无连接，无法可靠探测
		}
		host := idHost
		switch pi.Service {
		case "hbbr":
			host = relayHost
		case "api":
			host = apiHost
		}
		if host == "" {
			continue
		}
		addr := net.JoinHostPort(host, strconv.Itoa(pi.Port))
		start := time.Now()
		conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
		res := checkResult{Name: pi.Usage, Addr: addr}
		if err != nil {
			res.Error = "不可达"
		} else {
			res.OK = true
			res.Latency = time.Since(start).Round(time.Millisecond).String()
			_ = conn.Close()
		}
		results = append(results, res)
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 1, "results": results})
}

func hostWithoutPort(hostport string) string {
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		return h
	}
	return hostport
}

// 确保 gorm 引用（loadProfile 使用 First 的错误处理）。
var _ = gorm.ErrRecordNotFound
