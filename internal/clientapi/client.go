package clientapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/hybridsakura/prism-go-panel/internal/auth"
	"github.com/hybridsakura/prism-go-panel/internal/httpx"
	"github.com/hybridsakura/prism-go-panel/internal/models"
	"github.com/hybridsakura/prism-go-panel/internal/platform"
)

// Handler 实现 RustDesk 客户端契约（/api/*）。
// 只做协议翻译，业务规则（设备、冲突、统计）在 platform 包中。
type Handler struct {
	core    *platform.Core
	tokens  *auth.TokenStore
	limiter *httpx.LoginLimiter
}

// New 创建客户端契约处理器。
func New(core *platform.Core, tokens *auth.TokenStore) *Handler {
	return &Handler{
		core:    core,
		tokens:  tokens,
		limiter: httpx.NewLoginLimiter(core.Cfg.LoginRateLimit),
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

func (s *Handler) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.Error(w, "请求方式错误！请使用POST方式。")
		return
	}
	var req loginRequest
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.Error(w, "请求格式错误")
		return
	}
	if req.Username == "" || req.Password == "" {
		httpx.Error(w, "帐号或密码错误！请重试，多次重试后将被锁定IP！")
		return
	}
	if !s.limiter.Allow(httpx.ClientIP(r)) {
		w.Header().Set("Retry-After", "60")
		httpx.JSON(w, http.StatusTooManyRequests, map[string]string{"error": "尝试过于频繁，请稍后再试"})
		return
	}

	var user models.User
	if err := s.core.DB.Where("username = ?", req.Username).First(&user).Error; err != nil {
		httpx.Error(w, "帐号或密码错误！请重试，多次重试后将被锁定IP！")
		return
	}
	if !user.IsActive || !auth.CheckPassword(user.PasswordHash, req.Password) {
		httpx.Error(w, "帐号或密码错误！请重试，多次重试后将被锁定IP！")
		return
	}

	// ID 冲突拦截：同一远程 ID 被多台机器共用时，客户端会弹出
	// 「无法验证端到端加密」。这里只拦「非原机」的一方（身份不同），
	// 提示其更换 ID；原机始终放行，避免误伤。
	if s.core.Cfg.IDConflictEnforce {
		if c, ok := s.core.FindConflict(req.ID); ok && c.PrimaryUUID != "" && req.UUID != "" && req.UUID != c.PrimaryUUID {
			_ = s.core.DB.Model(c).UpdateColumn("blocked", c.Blocked+1).Error
			httpx.Error(w, platform.ConflictMessage(c))
			return
		}
	}

	deviceInfo := "{}"
	if len(req.DeviceInfo) > 0 {
		deviceInfo = string(req.DeviceInfo)
	}
	// 记录本次登录的设备信息（与参考版一致）
	_ = s.core.DB.Model(&models.User{}).Where("id = ?", user.ID).Updates(map[string]any{
		"rid": req.ID, "uuid": req.UUID, "auto_login": req.AutoLogin,
		"rtype": req.Type, "device_info": deviceInfo,
	}).Error

	// 设备已通过 sysinfo 注册且地址簿中尚无该设备时，自动加入地址簿
	s.bindDeviceToPeer(&user, req.ID, req.UUID)

	token, err := s.tokens.Issue(&user, req.ID, req.UUID)
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"access_token": token.AccessToken,
		"type":         "access_token",
		"user":         map[string]string{"name": user.Username},
	})
}

// bindDeviceToPeer 把已注册设备自动加入用户地址簿。
func (s *Handler) bindDeviceToPeer(user *models.User, rid, uuid string) {
	if rid == "" || uuid == "" {
		return
	}
	var count int64
	if err := s.core.DB.Model(&models.Peer{}).Where("uid = ? AND rid = ?", user.ID, rid).Count(&count).Error; err != nil || count > 0 {
		return
	}
	var dev models.Device
	if err := s.core.DB.Where("uuid = ?", uuid).First(&dev).Error; err != nil {
		return
	}
	peer := models.Peer{UID: user.ID, RID: rid, Hostname: dev.Hostname, Username: dev.Username}
	_ = s.core.DB.Create(&peer).Error
}

// ---------- /api/logout ----------

func (s *Handler) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.Error(w, "请求方式错误！")
		return
	}
	var req struct {
		ID   string `json:"id"`
		UUID string `json:"uuid"`
	}
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.Error(w, "请求格式错误")
		return
	}
	if err := s.tokens.RevokeByDevice(req.ID, req.UUID); err != nil {
		httpx.ServerError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"code": 1})
}

// ---------- /api/currentUser ----------

func (s *Handler) handleCurrentUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.Error(w, "错误的提交方式！")
		return
	}
	user, token, err := s.tokens.Validate(httpx.BearerToken(r))
	if err != nil {
		// 与参考版一致：无效 token 返回空对象（HTTP 200）
		httpx.JSON(w, http.StatusOK, map[string]string{})
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
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
func (s *Handler) handleABGet(w http.ResponseWriter, r *http.Request) {
	user, _, err := s.tokens.Validate(httpx.BearerToken(r))
	if err != nil {
		httpx.Error(w, "拉取列表错误！")
		return
	}

	var tags []models.Tag
	if err := s.core.DB.Where("uid = ?", user.ID).Find(&tags).Error; err != nil {
		httpx.ServerError(w, err)
		return
	}
	var peers []models.Peer
	if err := s.core.DB.Where("uid = ?", user.ID).Find(&peers).Error; err != nil {
		httpx.ServerError(w, err)
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
			"tags":     platform.SplitPeers(p.Tags), // 空标签输出 [] 而非 [""]
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
		httpx.ServerError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"updated_at": time.Now().Format(time.RFC3339),
		"data":       string(dataJSON),
	})
}

// handleABPost 客户端全量同步地址簿（先删后插，单事务）。
func (s *Handler) handleABPost(w http.ResponseWriter, r *http.Request) {
	user, _, err := s.tokens.Validate(httpx.BearerToken(r))
	if err != nil {
		httpx.Error(w, "拉取列表错误！")
		return
	}
	var req abUpdateRequest
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.Error(w, "请求格式错误")
		return
	}
	var payload abDataPayload
	if req.Data != "" {
		if err := json.Unmarshal([]byte(req.Data), &payload); err != nil {
			httpx.Error(w, "请求格式错误")
			return
		}
	}

	tagColors := platform.ParseTagColors(payload.TagColors)

	err = s.core.DB.Transaction(func(tx *gorm.DB) error {
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
		httpx.ServerError(w, err)
		return
	}
	// 参考版此处固定返回 code=102；Go 版按成功语义返回 code=1（客户端不解析该响应）
	httpx.JSON(w, http.StatusOK, map[string]any{"code": 1, "data": "ok"})
}

// handleABGetCompat POST /api/ab/get，兼容 x86-sciter 旧客户端。
func (s *Handler) handleABGetCompat(w http.ResponseWriter, r *http.Request) {
	s.handleABGet(w, r)
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

func (s *Handler) handleSysinfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.Error(w, "错误的提交方式！")
		return
	}
	var req sysinfoRequest
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.Error(w, "请求格式错误")
		return
	}
	if req.ID == "" || req.UUID == "" {
		httpx.Error(w, "参数不完整")
		return
	}
	// ID 冲突检测：同一 RID 曾以不同主机名/不同 UUID 上报过
	var seen []models.Device
	_ = s.core.DB.Where("rid = ?", req.ID).Find(&seen).Error
	if len(seen) > 0 {
		hostnames := []string{req.Hostname}
		uuids := []string{req.UUID}
		for _, d := range seen {
			hostnames = append(hostnames, d.Hostname)
			uuids = append(uuids, d.UUID)
		}
		s.core.RecordIDConflict(req.ID, hostnames, uuids)
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
		IPAddress:  httpx.ClientIP(r),
		CreateTime: now,
		UpdateTime: now,
	}
	// UPSERT：按 (rid, uuid) 冲突时只更新资产字段，保留注册时间
	err := s.core.DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "rid"}, {Name: "uuid"}},
		DoUpdates: clause.AssignmentColumns([]string{"cpu", "hostname", "memory", "os", "username", "version", "ip_address", "update_time"}),
	}).Create(&dev).Error
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	s.core.Tracker.Touch(req.ID)
	httpx.JSON(w, http.StatusOK, map[string]any{"data": "ok"})
}

// ---------- /api/heartbeat ----------

func (s *Handler) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID   string `json:"id"`
		UUID string `json:"uuid"`
	}
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.Error(w, "请求格式错误")
		return
	}
	if req.ID == "" || req.UUID == "" {
		httpx.Error(w, "参数不完整")
		return
	}
	res := s.core.DB.Model(&models.Device{}).
		Where("rid = ? AND uuid = ?", req.ID, req.UUID).
		Updates(map[string]any{"ip_address": httpx.ClientIP(r), "update_time": time.Now()})
	if res.Error != nil {
		httpx.ServerError(w, res.Error)
		return
	}
	if res.RowsAffected > 0 {
		// 设备已注册：刷新在线状态并续期 token
		s.core.Tracker.Touch(req.ID)
		_ = s.tokens.KeepAlive(req.ID, req.UUID)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": "在线"})
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

func (s *Handler) handleAudit(w http.ResponseWriter, r *http.Request) {
	var req auditRequest
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.Error(w, "请求格式错误")
		return
	}
	now := time.Now()

	switch {
	case req.Action == "new":
		log := models.ConnLog{
			Action:    req.Action,
			ConnID:    platform.ToStr(req.ConnID),
			FromIP:    req.IP,
			RID:       req.ID,
			ConnStart: now,
			SessionID: platform.ToStr(req.SessionID),
			UUID:      req.UUID,
		}
		if err := s.core.DB.Create(&log).Error; err != nil {
			httpx.ServerError(w, err)
			return
		}
	case req.Action == "close":
		if err := s.core.DB.Model(&models.ConnLog{}).
			Where("conn_id = ?", platform.ToStr(req.ConnID)).
			Update("conn_end", now).Error; err != nil {
			httpx.ServerError(w, err)
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
			UserID:    platform.ToStr(req.PeerID),
			UserIP:    info.IP,
			RemoteID:  req.ID,
			Filesize:  platform.ConvertFilesize(size),
			Direction: direction,
			LoggedAt:  now,
		}
		if err := s.core.DB.Create(&log).Error; err != nil {
			httpx.ServerError(w, err)
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
			fromID = platform.ToStr(peer[0])
		}
		connID := platform.ToStr(req.ConnID)
		if err := s.core.DB.Model(&models.ConnLog{}).Where("conn_id = ?", connID).
			Updates(map[string]any{"session_id": platform.ToStr(req.SessionID), "from_id": fromID}).Error; err != nil {
			httpx.ServerError(w, err)
			return
		}
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"code": 1, "data": "ok"})
}

// ---------- 兼容占位接口 ----------

// handleUsersStub /api/users，与参考版一致返回成功占位。
func (s *Handler) handleUsersStub(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, http.StatusOK, map[string]any{"code": 1, "data": "好的"})
}

// handlePeersStub /api/peers，与参考版一致返回成功占位。
func (s *Handler) handlePeersStub(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, http.StatusOK, map[string]any{"code": 1, "data": "ok"})
}
