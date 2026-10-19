package api

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"rustdesk-api-go/internal/auth"
	"rustdesk-api-go/internal/config"
	"rustdesk-api-go/internal/models"
	"rustdesk-api-go/internal/stats"
)

// sessionCookieName 管理后台会话 Cookie 名称。
const sessionCookieName = "rdadmin_session"

// Server 聚合全部依赖，实现两类接口：
//   - /api/*       RustDesk 客户端契约（与 Python 参考版行为一致）
//   - /admin/api/* 管理后台 JSON 接口
type Server struct {
	cfg      *config.Config
	db       *gorm.DB
	tokens   *auth.TokenStore
	sessions *auth.SessionStore
	tracker  *stats.Tracker
	limiter  *loginLimiter
}

// NewServer 创建 API 服务器。
func NewServer(cfg *config.Config, db *gorm.DB, tokens *auth.TokenStore, sessions *auth.SessionStore, tracker *stats.Tracker) *Server {
	return &Server{
		cfg:      cfg,
		db:       db,
		tokens:   tokens,
		sessions: sessions,
		tracker:  tracker,
		limiter:  newLoginLimiter(cfg.LoginRateLimit),
	}
}

// ---------- /api/login ----------

type loginRequest struct {
	Username   string          `json:"username"`
	Password   string          `json:"password"`
	ID         string          `json:"id"`
	UUID       string          `json:"uuid"`
	AutoLogin  bool            `json:"autoLogin"`
	Type       string          `json:"type"`
	DeviceInfo json.RawMessage `json:"deviceInfo"`
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		apiError(w, "请求方式错误！请使用POST方式。")
		return
	}
	var req loginRequest
	if err := readJSON(r, &req); err != nil {
		apiError(w, "请求格式错误")
		return
	}
	if req.Username == "" || req.Password == "" {
		apiError(w, "帐号或密码错误！请重试，多次重试后将被锁定IP！")
		return
	}
	if !s.limiter.Allow(clientIP(r)) {
		w.Header().Set("Retry-After", "60")
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "尝试过于频繁，请稍后再试"})
		return
	}

	var user models.User
	if err := s.db.Where("username = ?", req.Username).First(&user).Error; err != nil {
		apiError(w, "帐号或密码错误！请重试，多次重试后将被锁定IP！")
		return
	}
	if !user.IsActive || !auth.CheckPassword(user.PasswordHash, req.Password) {
		apiError(w, "帐号或密码错误！请重试，多次重试后将被锁定IP！")
		return
	}

	// ID 冲突拦截：同一远程 ID 被多台机器共用时，客户端会弹出
	// 「无法验证端到端加密」。这里只拦「非原机」的一方（身份不同），
	// 提示其更换 ID；原机始终放行，避免误伤。
	if s.cfg.IDConflictEnforce {
		if c, ok := s.findConflict(req.ID); ok && c.PrimaryUUID != "" && req.UUID != "" && req.UUID != c.PrimaryUUID {
			_ = s.db.Model(c).UpdateColumn("blocked", c.Blocked+1).Error
			apiError(w, conflictMessage(c))
			return
		}
	}

	deviceInfo := "{}"
	if len(req.DeviceInfo) > 0 {
		deviceInfo = string(req.DeviceInfo)
	}
	// 记录本次登录的设备信息（与参考版一致）
	_ = s.db.Model(&models.User{}).Where("id = ?", user.ID).Updates(map[string]any{
		"rid": req.ID, "uuid": req.UUID, "auto_login": req.AutoLogin,
		"rtype": req.Type, "device_info": deviceInfo,
	}).Error

	// 设备已通过 sysinfo 注册且地址簿中尚无该设备时，自动加入地址簿
	s.bindDeviceToPeer(&user, req.ID, req.UUID)

	token, err := s.tokens.Issue(&user, req.ID, req.UUID)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"access_token": token.AccessToken,
		"type":         "access_token",
		"user":         map[string]string{"name": user.Username},
	})
}

// bindDeviceToPeer 把已注册设备自动加入用户地址簿。
func (s *Server) bindDeviceToPeer(user *models.User, rid, uuid string) {
	if rid == "" || uuid == "" {
		return
	}
	var count int64
	if err := s.db.Model(&models.Peer{}).Where("uid = ? AND rid = ?", user.ID, rid).Count(&count).Error; err != nil || count > 0 {
		return
	}
	var dev models.Device
	if err := s.db.Where("uuid = ?", uuid).First(&dev).Error; err != nil {
		return
	}
	peer := models.Peer{UID: user.ID, RID: rid, Hostname: dev.Hostname, Username: dev.Username}
	_ = s.db.Create(&peer).Error
}

// ---------- /api/logout ----------

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		apiError(w, "请求方式错误！")
		return
	}
	var req struct {
		ID   string `json:"id"`
		UUID string `json:"uuid"`
	}
	if err := readJSON(r, &req); err != nil {
		apiError(w, "请求格式错误")
		return
	}
	if err := s.tokens.RevokeByDevice(req.ID, req.UUID); err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 1})
}

// ---------- /api/currentUser ----------

func (s *Server) handleCurrentUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		apiError(w, "错误的提交方式！")
		return
	}
	user, token, err := s.tokens.Validate(bearerToken(r))
	if err != nil {
		// 与参考版一致：无效 token 返回空对象（HTTP 200）
		writeJSON(w, http.StatusOK, map[string]string{})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"access_token": token.AccessToken,
		"type":         "access_token",
		"name":         user.Username,
	})
}

// ---------- /api/ab（地址簿） ----------

type abPeerPayload struct {
	ID       string   `json:"id"`
	Username string   `json:"username"`
	Hostname string   `json:"hostname"`
	Alias    string   `json:"alias"`
	Platform string   `json:"platform"`
	Tags     []string `json:"tags"`
	Hash     string   `json:"hash"`
}

type abUpdateRequest struct {
	Data string `json:"data"`
}

type abDataPayload struct {
	Tags      []string        `json:"tags"`
	TagColors json.RawMessage `json:"tag_colors"` // 可能是对象，也可能是双重编码的字符串
	Peers     []abPeerPayload `json:"peers"`
}

// handleABGet 读取地址簿。GET /api/ab 与 POST /api/ab/get（旧版 sciter 客户端）共用。
func (s *Server) handleABGet(w http.ResponseWriter, r *http.Request) {
	user, _, err := s.tokens.Validate(bearerToken(r))
	if err != nil {
		apiError(w, "拉取列表错误！")
		return
	}

	var tags []models.Tag
	if err := s.db.Where("uid = ?", user.ID).Find(&tags).Error; err != nil {
		serverError(w, err)
		return
	}
	var peers []models.Peer
	if err := s.db.Where("uid = ?", user.ID).Find(&peers).Error; err != nil {
		serverError(w, err)
		return
	}

	tagNames := make([]string, 0, len(tags))
	tagColors := make(map[string]int, len(tags))
	for _, t := range tags {
		tagNames = append(tagNames, t.Name)
		if c, e := strconv.Atoi(strings.TrimSpace(t.Color)); e == nil {
			tagColors[t.Name] = c
		}
	}
	peersResult := make([]map[string]any, 0, len(peers))
	for _, p := range peers {
		peersResult = append(peersResult, map[string]any{
			"id":       p.RID,
			"username": p.Username,
			"hostname": p.Hostname,
			"alias":    p.Alias,
			"platform": p.Platform,
			"tags":     splitPeers(p.Tags), // 空标签输出 [] 而非 [""]
			"hash":     p.RHash,
		})
	}
	tagColorsJSON, _ := json.Marshal(tagColors)
	data := map[string]any{
		"tags":       tagNames,
		"peers":      peersResult,
		"tag_colors": string(tagColorsJSON), // 与参考版一致：tag_colors 为 JSON 字符串
	}
	dataJSON, err := json.Marshal(data)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"updated_at": time.Now().Format(time.RFC3339),
		"data":       string(dataJSON),
	})
}

// handleABPost 客户端全量同步地址簿（先删后插，单事务）。
func (s *Server) handleABPost(w http.ResponseWriter, r *http.Request) {
	user, _, err := s.tokens.Validate(bearerToken(r))
	if err != nil {
		apiError(w, "拉取列表错误！")
		return
	}
	var req abUpdateRequest
	if err := readJSON(r, &req); err != nil {
		apiError(w, "请求格式错误")
		return
	}
	var payload abDataPayload
	if req.Data != "" {
		if err := json.Unmarshal([]byte(req.Data), &payload); err != nil {
			apiError(w, "请求格式错误")
			return
		}
	}

	tagColors := parseTagColors(payload.TagColors)

	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("uid = ?", user.ID).Delete(&models.Tag{}).Error; err != nil {
			return err
		}
		if err := tx.Where("uid = ?", user.ID).Delete(&models.Peer{}).Error; err != nil {
			return err
		}
		for _, name := range payload.Tags {
			tag := models.Tag{UID: user.ID, Name: name, Color: tagColors[name]}
			if err := tx.Create(&tag).Error; err != nil {
				return err
			}
		}
		for _, p := range payload.Peers {
			peer := models.Peer{
				UID:      user.ID,
				RID:      p.ID,
				Username: p.Username,
				Hostname: p.Hostname,
				Alias:    p.Alias,
				Platform: p.Platform,
				Tags:     strings.Join(p.Tags, ","),
				RHash:    p.Hash,
			}
			if err := tx.Create(&peer).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		serverError(w, err)
		return
	}
	// 参考版此处固定返回 code=102；Go 版按成功语义返回 code=1（客户端不解析该响应）
	writeJSON(w, http.StatusOK, map[string]any{"code": 1, "data": "ok"})
}

// handleABGetCompat POST /api/ab/get，兼容 x86-sciter 旧客户端。
func (s *Server) handleABGetCompat(w http.ResponseWriter, r *http.Request) {
	s.handleABGet(w, r)
}

// parseTagColors 兼容 tag_colors 的两种编码：JSON 对象或双重编码的 JSON 字符串。
func parseTagColors(raw json.RawMessage) map[string]string {
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

// ---------- /api/sysinfo（设备资产上报） ----------

type sysinfoRequest struct {
	ID       string `json:"id"`
	UUID     string `json:"uuid"`
	CPU      string `json:"cpu"`
	Hostname string `json:"hostname"`
	Memory   string `json:"memory"`
	OS       string `json:"os"`
	Username string `json:"username"`
	Version  string `json:"version"`
}

func (s *Server) handleSysinfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		apiError(w, "错误的提交方式！")
		return
	}
	var req sysinfoRequest
	if err := readJSON(r, &req); err != nil {
		apiError(w, "请求格式错误")
		return
	}
	if req.ID == "" || req.UUID == "" {
		apiError(w, "参数不完整")
		return
	}
	// ID 冲突检测：同一 RID 曾以不同主机名/不同 UUID 上报过
	var seen []models.Device
	_ = s.db.Where("rid = ?", req.ID).Find(&seen).Error
	if len(seen) > 0 {
		hostnames := []string{req.Hostname}
		uuids := []string{req.UUID}
		for _, d := range seen {
			hostnames = append(hostnames, d.Hostname)
			uuids = append(uuids, d.UUID)
		}
		s.recordIDConflict(req.ID, hostnames, uuids)
	}
	now := time.Now()
	dev := models.Device{
		RID:        req.ID,
		UUID:       req.UUID,
		CPU:        req.CPU,
		Hostname:   req.Hostname,
		Memory:     req.Memory,
		OS:         req.OS,
		Username:   req.Username,
		Version:    req.Version,
		IPAddress:  clientIP(r),
		CreateTime: now,
		UpdateTime: now,
	}
	// UPSERT：按 (rid, uuid) 冲突时只更新资产字段，保留注册时间
	err := s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "rid"}, {Name: "uuid"}},
		DoUpdates: clause.AssignmentColumns([]string{"cpu", "hostname", "memory", "os", "username", "version", "ip_address", "update_time"}),
	}).Create(&dev).Error
	if err != nil {
		serverError(w, err)
		return
	}
	s.tracker.Touch(req.ID)
	writeJSON(w, http.StatusOK, map[string]any{"data": "ok"})
}

// ---------- /api/heartbeat ----------

func (s *Server) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID   string `json:"id"`
		UUID string `json:"uuid"`
	}
	if err := readJSON(r, &req); err != nil {
		apiError(w, "请求格式错误")
		return
	}
	if req.ID == "" || req.UUID == "" {
		apiError(w, "参数不完整")
		return
	}
	res := s.db.Model(&models.Device{}).
		Where("rid = ? AND uuid = ?", req.ID, req.UUID).
		Updates(map[string]any{"ip_address": clientIP(r), "update_time": time.Now()})
	if res.Error != nil {
		serverError(w, res.Error)
		return
	}
	if res.RowsAffected > 0 {
		// 设备已注册：刷新在线状态并续期 token
		s.tracker.Touch(req.ID)
		_ = s.tokens.KeepAlive(req.ID, req.UUID)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": "在线"})
}

// ---------- /api/audit（连接/文件日志） ----------

type auditRequest struct {
	Action    string          `json:"action"`
	ConnID    any             `json:"conn_id"`
	IP        string          `json:"ip"`
	ID        string          `json:"id"`
	SessionID any             `json:"session_id"`
	UUID      string          `json:"uuid"`
	IsFile    bool            `json:"is_file"`
	Info      any             `json:"info"`
	Path      string          `json:"path"`
	PeerID    any             `json:"peer_id"`
	Type      any             `json:"type"`
	Peer      json.RawMessage `json:"peer"`
}

func (s *Server) handleAudit(w http.ResponseWriter, r *http.Request) {
	var req auditRequest
	if err := readJSON(r, &req); err != nil {
		apiError(w, "请求格式错误")
		return
	}
	now := time.Now()

	switch {
	case req.Action == "new":
		log := models.ConnLog{
			Action:    req.Action,
			ConnID:    toStr(req.ConnID),
			FromIP:    req.IP,
			RID:       req.ID,
			ConnStart: now,
			SessionID: toStr(req.SessionID),
			UUID:      req.UUID,
		}
		if err := s.db.Create(&log).Error; err != nil {
			serverError(w, err)
			return
		}
	case req.Action == "close":
		if err := s.db.Model(&models.ConnLog{}).
			Where("conn_id = ?", toStr(req.ConnID)).
			Update("conn_end", now).Error; err != nil {
			serverError(w, err)
			return
		}
	case req.IsFile:
		var info struct {
			IP    string  `json:"ip"`
			Files [][]any `json:"files"`
		}
		// info 可能是 JSON 对象或 JSON 字符串（双重编码），兼容两种形态
		if req.Info != nil {
			var infoBytes []byte
			switch v := req.Info.(type) {
			case string:
				infoBytes = []byte(v)
			default:
				infoBytes, _ = json.Marshal(v)
			}
			if len(infoBytes) > 0 {
				_ = json.Unmarshal(infoBytes, &info)
			}
		}
		size := int64(0)
		if len(info.Files) > 0 && len(info.Files[0]) > 1 {
			if f, ok := info.Files[0][1].(float64); ok {
				size = int64(f)
			}
		}
		direction := 0
		if f, ok := req.Type.(float64); ok {
			direction = int(f)
		}
		log := models.FileLog{
			File:      req.Path,
			UserID:    toStr(req.PeerID),
			UserIP:    info.IP,
			RemoteID:  req.ID,
			Filesize:  convertFilesize(size),
			Direction: direction,
			LoggedAt:  now,
		}
		if err := s.db.Create(&log).Error; err != nil {
			serverError(w, err)
			return
		}
	default:
		// 连接建立后的补充信息：session_id 与对端 ID
		var peer []any
		if len(req.Peer) > 0 {
			_ = json.Unmarshal(req.Peer, &peer)
		}
		fromID := ""
		if len(peer) > 0 {
			fromID = toStr(peer[0])
		}
		connID := toStr(req.ConnID)
		if err := s.db.Model(&models.ConnLog{}).Where("conn_id = ?", connID).
			Updates(map[string]any{"session_id": toStr(req.SessionID), "from_id": fromID}).Error; err != nil {
			serverError(w, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 1, "data": "ok"})
}

// ---------- 兼容占位接口 ----------

// handleUsersStub /api/users，与参考版一致返回成功占位。
func (s *Server) handleUsersStub(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"code": 1, "data": "好的"})
}

// handlePeersStub /api/peers，与参考版一致返回成功占位。
func (s *Server) handlePeersStub(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"code": 1, "data": "ok"})
}

// newHash 生成分享链接的随机标识（32 字节十六进制）。
func newHash() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		// crypto/rand 失败属于极端情况，退化为时间戳哈希保证唯一性
		sum := sha256.Sum256([]byte(time.Now().String()))
		return hex.EncodeToString(sum[:16])
	}
	return hex.EncodeToString(buf)
}

// splitPeers 把逗号分隔的设备 ID 字符串拆成列表。
func splitPeers(s string) []string {
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

// ---------- 登录限速 ----------

type loginLimiter struct {
	mu     sync.Mutex
	window map[string]*ipCounter
	limit  int
}

type ipCounter struct {
	count int
	until time.Time
}

func newLoginLimiter(limitPerMinute int) *loginLimiter {
	return &loginLimiter{window: make(map[string]*ipCounter), limit: limitPerMinute}
}

// Allow 判断某 IP 当前是否允许登录。limit <= 0 表示关闭限速。
func (l *loginLimiter) Allow(ip string) bool {
	if l.limit <= 0 {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	c, ok := l.window[ip]
	if !ok || now.After(c.until) {
		l.window[ip] = &ipCounter{count: 1, until: now.Add(time.Minute)}
		return true
	}
	c.count++
	return c.count <= l.limit
}
