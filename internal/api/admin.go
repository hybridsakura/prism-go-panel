package api

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/hybridsakura/prism-go-panel/internal/auth"
	"github.com/hybridsakura/prism-go-panel/internal/models"
)

// ---------- 上下文与中间件 ----------

type ctxKey int

const userCtxKey ctxKey = iota

func userFromContext(ctx context.Context) *models.User {
	if u, ok := ctx.Value(userCtxKey).(*models.User); ok {
		return u
	}
	return nil
}

// requireSession 校验管理后台会话 Cookie。
func (s *Server) requireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(sessionCookieName)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "未登录"})
			return
		}
		sess, ok := s.sessions.Get(c.Value)
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "会话已过期，请重新登录"})
			return
		}
		var user models.User
		if err := s.db.First(&user, sess.UserID).Error; err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "用户不存在"})
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userCtxKey, &user)))
	})
}

// requireAdmin 在会话校验基础上要求管理员权限。
func (s *Server) requireAdmin(next http.Handler) http.Handler {
	return s.requireSession(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := userFromContext(r.Context())
		if user == nil || !user.IsAdmin {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "需要管理员权限"})
			return
		}
		next.ServeHTTP(w, r)
	}))
}

// ---------- 登录 / 注册 / 登出 ----------

func (s *Server) handleAdminLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		apiError(w, "请求方式错误！")
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := readJSON(r, &req); err != nil {
		apiError(w, "请求格式错误")
		return
	}
	if !s.limiter.Allow(clientIP(r)) {
		w.Header().Set("Retry-After", "60")
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "尝试过于频繁，请稍后再试"})
		return
	}
	var user models.User
	if err := s.db.Where("username = ?", req.Username).First(&user).Error; err != nil {
		apiError(w, "帐号或密码错误！")
		return
	}
	if !user.IsActive || !auth.CheckPassword(user.PasswordHash, req.Password) {
		apiError(w, "帐号或密码错误！")
		return
	}
	sid, err := s.sessions.Create(user.ID)
	if err != nil {
		serverError(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    sid,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(s.cfg.SessionTTL.Seconds()),
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"code": 1,
		"user": map[string]any{
			"id": user.ID, "username": user.Username, "is_admin": user.IsAdmin,
		},
	})
}

func (s *Server) handleAdminLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookieName); err == nil {
		s.sessions.Delete(c.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookieName, Value: "", Path: "/", HttpOnly: true, MaxAge: -1,
	})
	writeJSON(w, http.StatusOK, map[string]any{"code": 1})
}

func (s *Server) handleAdminRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		apiError(w, "请求方式错误！")
		return
	}
	if !s.cfg.AllowRegistration {
		writeJSON(w, http.StatusOK, map[string]any{"code": 0, "msg": "当前未开放注册，请联系管理员！"})
		return
	}
	var req struct {
		User string `json:"user"`
		Pwd  string `json:"pwd"`
	}
	if err := readJSON(r, &req); err != nil {
		apiError(w, "请求格式错误")
		return
	}
	if len([]rune(req.User)) <= 3 {
		writeJSON(w, http.StatusOK, map[string]any{"code": 0, "msg": "用户名不得小于3位"})
		return
	}
	if len(req.Pwd) < 8 || len(req.Pwd) > 20 {
		writeJSON(w, http.StatusOK, map[string]any{"code": 0, "msg": "密码长度不符合要求, 应在8~20位。"})
		return
	}
	var count int64
	if err := s.db.Model(&models.User{}).Where("username = ?", req.User).Count(&count).Error; err != nil {
		serverError(w, err)
		return
	}
	if count > 0 {
		writeJSON(w, http.StatusOK, map[string]any{"code": 0, "msg": "用户名已存在。"})
		return
	}
	// 首个注册用户自动成为管理员（与参考版一致）
	var total int64
	if err := s.db.Model(&models.User{}).Count(&total).Error; err != nil {
		serverError(w, err)
		return
	}
	hash, err := auth.HashPassword(req.Pwd)
	if err != nil {
		serverError(w, err)
		return
	}
	user := models.User{
		Username: req.User, PasswordHash: hash,
		IsActive: true, IsAdmin: total == 0,
	}
	if err := s.db.Create(&user).Error; err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 1})
}

func (s *Server) handleAdminMe(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "未登录"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id": user.ID, "username": user.Username, "is_admin": user.IsAdmin,
	})
}

// ---------- 统计 ----------

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	var devicesTotal, usersTotal, connToday, filesToday, sharesActive int64
	startOfDay := time.Now().Truncate(24 * time.Hour)

	if err := s.db.Model(&models.Device{}).Count(&devicesTotal).Error; err != nil {
		serverError(w, err)
		return
	}
	if err := s.db.Model(&models.User{}).Count(&usersTotal).Error; err != nil {
		serverError(w, err)
		return
	}
	if err := s.db.Model(&models.ConnLog{}).Where("conn_start >= ?", startOfDay).Count(&connToday).Error; err != nil {
		serverError(w, err)
		return
	}
	if err := s.db.Model(&models.FileLog{}).Where("logged_at >= ?", startOfDay).Count(&filesToday).Error; err != nil {
		serverError(w, err)
		return
	}
	if err := s.db.Model(&models.ShareLink{}).
		Where("is_used = ? AND is_expired = ?", false, false).Count(&sharesActive).Error; err != nil {
		serverError(w, err)
		return
	}

	online := s.tracker.OnlineCount()
	offline := devicesTotal - int64(online)
	if offline < 0 {
		offline = 0
	}
	rate := 0.0
	if devicesTotal > 0 {
		rate = float64(online) / float64(devicesTotal) * 100
	}
	// ID 冲突数量（无账号模式下这是唯一的告警渠道，前端据此显示横幅）
	var conflicts int64
	_ = s.db.Model(&models.IDConflict{}).Count(&conflicts).Error
	writeJSON(w, http.StatusOK, map[string]any{
		"devices_total":   devicesTotal,
		"devices_online":  online,
		"devices_offline": offline,
		"online_rate":     strconv.FormatFloat(rate, 'f', 1, 64),
		"users_total":     usersTotal,
		"conn_today":      connToday,
		"files_today":     filesToday,
		"shares_active":   sharesActive,
		"id_conflicts":    conflicts,
	})
}

// ---------- 设备列表 ----------

// deviceRow 设备列表行（合并地址簿与资产信息）。
type deviceRow struct {
	RID        string   `json:"rid"`
	Alias      string   `json:"alias"`
	Hostname   string   `json:"hostname"`
	Username   string   `json:"username"` // 系统用户名
	Owner      string   `json:"owner"`    // 平台所属用户
	Platform   string   `json:"platform"`
	Tags       []string `json:"tags"`
	CPU        string   `json:"cpu"`
	Memory     string   `json:"memory"`
	OS         string   `json:"os"`
	Version    string   `json:"version"`
	IPAddress  string   `json:"ip_address"`
	Status     string   `json:"status"`
	Conflict   bool     `json:"conflict"` // 该 RID 存在重复（多台机器共用）
	CreateTime string   `json:"create_time"`
	UpdateTime string   `json:"update_time"`
}

// conflictRIDs 返回给定 RID 中存在于冲突表的集合。
func (s *Server) conflictRIDs(rids []string) map[string]bool {
	out := map[string]bool{}
	if len(rids) == 0 {
		return out
	}
	var rows []models.IDConflict
	if err := s.db.Where("rid IN ?", rids).Find(&rows).Error; err != nil {
		return out
	}
	for _, c := range rows {
		out[c.RID] = true
	}
	return out
}

func (s *Server) handleDevices(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "未登录"})
		return
	}
	page, size := parsePage(r)
	keyword := strings.TrimSpace(r.URL.Query().Get("q"))
	showAll := r.URL.Query().Get("all") == "1" && user.IsAdmin

	var rows []deviceRow
	var total int64

	if showAll {
		// 管理员视图：全部设备，附带地址簿别名与所属用户
		var devices []models.Device
		q := s.db.Model(&models.Device{})
		if keyword != "" {
			like := "%" + keyword + "%"
			q = q.Where("rid LIKE ? OR hostname LIKE ? OR ip_address LIKE ?", like, like, like)
		}
		if err := q.Count(&total).Error; err != nil {
			serverError(w, err)
			return
		}
		if err := q.Order("update_time DESC").Offset((page - 1) * size).Limit(size).Find(&devices).Error; err != nil {
			serverError(w, err)
			return
		}
		rids := make([]string, 0, len(devices))
		for _, d := range devices {
			rids = append(rids, d.RID)
		}
		peerByRID, ownerByRID := s.peerInfoForRIDs(rids)
		conflicts := s.conflictRIDs(rids)
		for _, d := range devices {
			p := peerByRID[d.RID]
			rows = append(rows, deviceRow{
				RID: d.RID, Alias: p.Alias, Hostname: d.Hostname, Username: d.Username,
				Owner: ownerByRID[d.RID], Platform: d.OS, Tags: splitTags(p.Tags),
				CPU: d.CPU, Memory: d.Memory, OS: d.OS, Version: d.Version,
				IPAddress: d.IPAddress, Status: onlineStatus(s, d.RID), Conflict: conflicts[d.RID],
				CreateTime: formatTime(d.CreateTime), UpdateTime: formatTime(d.UpdateTime),
			})
		}
	} else {
		// 普通用户视图：自己的地址簿设备
		var peers []models.Peer
		pq := s.db.Model(&models.Peer{}).Where("uid = ?", user.ID)
		if keyword != "" {
			like := "%" + keyword + "%"
			pq = pq.Where("rid LIKE ? OR alias LIKE ? OR hostname LIKE ?", like, like, like)
		}
		if err := pq.Count(&total).Error; err != nil {
			serverError(w, err)
			return
		}
		if err := pq.Order("id DESC").Offset((page - 1) * size).Limit(size).Find(&peers).Error; err != nil {
			serverError(w, err)
			return
		}
		rids := make([]string, 0, len(peers))
		for _, p := range peers {
			rids = append(rids, p.RID)
		}
		devByRID := s.devicesForRIDs(rids)
		conflicts := s.conflictRIDs(rids)
		for _, p := range peers {
			d, ok := devByRID[p.RID]
			row := deviceRow{
				RID: p.RID, Alias: p.Alias, Hostname: p.Hostname, Username: p.Username,
				Owner: user.Username, Platform: p.Platform, Tags: splitTags(p.Tags),
				Status: offline, Conflict: conflicts[p.RID],
			}
			if ok {
				row.CPU = d.CPU
				row.Memory = d.Memory
				row.OS = d.OS
				row.Version = d.Version
				row.IPAddress = d.IPAddress
				row.Status = onlineStatus(s, d.RID)
				row.CreateTime = formatTime(d.CreateTime)
				row.UpdateTime = formatTime(d.UpdateTime)
				if row.Hostname == "" {
					row.Hostname = d.Hostname
				}
			}
			rows = append(rows, row)
		}
	}

	if rows == nil {
		rows = []deviceRow{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"total": total, "page": page, "size": size, "items": rows,
	})
}

const (
	online  = "online"
	offline = "offline"
)

func onlineStatus(s *Server, rid string) string {
	if s.tracker.Online(rid) {
		return online
	}
	return offline
}

// peerInfoForRIDs 批量查询地址簿条目与所属用户，避免 N+1 查询。
func (s *Server) peerInfoForRIDs(rids []string) (map[string]models.Peer, map[string]string) {
	peerByRID := make(map[string]models.Peer)
	ownerByRID := make(map[string]string)
	if len(rids) == 0 {
		return peerByRID, ownerByRID
	}
	var peers []models.Peer
	if err := s.db.Where("rid IN ?", rids).Find(&peers).Error; err != nil {
		return peerByRID, ownerByRID
	}
	uids := make([]uint, 0, len(peers))
	for _, p := range peers {
		peerByRID[p.RID] = p
		uids = append(uids, p.UID)
	}
	if len(uids) == 0 {
		return peerByRID, ownerByRID
	}
	var users []models.User
	if err := s.db.Where("id IN ?", uids).Find(&users).Error; err != nil {
		return peerByRID, ownerByRID
	}
	nameByID := make(map[uint]string, len(users))
	for _, u := range users {
		nameByID[u.ID] = u.Username
	}
	for rid, p := range peerByRID {
		if name, ok := nameByID[p.UID]; ok {
			ownerByRID[rid] = name
		}
	}
	return peerByRID, ownerByRID
}

// devicesForRIDs 批量查询设备信息。
func (s *Server) devicesForRIDs(rids []string) map[string]models.Device {
	out := make(map[string]models.Device)
	if len(rids) == 0 {
		return out
	}
	var devices []models.Device
	if err := s.db.Where("rid IN ?", rids).Find(&devices).Error; err != nil {
		return out
	}
	for _, d := range devices {
		out[d.RID] = d
	}
	return out
}

func splitTags(tags string) []string {
	if strings.TrimSpace(tags) == "" {
		return []string{}
	}
	parts := strings.Split(tags, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// ---------- 连接日志 / 文件日志 ----------

// formatTimePtr 安全格式化可能为空的时间指针。
func formatTimePtr(t *time.Time) string {
	if t == nil || t.IsZero() {
		return ""
	}
	return formatTime(*t)
}

func (s *Server) handleConnLogs(w http.ResponseWriter, r *http.Request) {
	page, size := parsePage(r)
	var total int64
	if err := s.db.Model(&models.ConnLog{}).Count(&total).Error; err != nil {
		serverError(w, err)
		return
	}
	var logs []models.ConnLog
	if err := s.db.Order("conn_start DESC").Offset((page - 1) * size).Limit(size).Find(&logs).Error; err != nil {
		serverError(w, err)
		return
	}
	items := make([]map[string]any, 0, len(logs))
	for _, l := range logs {
		items = append(items, map[string]any{
			"id": l.ID, "action": l.Action, "conn_id": l.ConnID,
			"from_ip": l.FromIP, "from_id": l.FromID, "rid": l.RID,
			"conn_start": formatTime(l.ConnStart),
			"conn_end":   formatTimePtr(l.ConnEnd),
			"session_id": l.SessionID, "uuid": l.UUID,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"total": total, "page": page, "size": size, "items": items})
}

func (s *Server) handleFileLogs(w http.ResponseWriter, r *http.Request) {
	page, size := parsePage(r)
	var total int64
	if err := s.db.Model(&models.FileLog{}).Count(&total).Error; err != nil {
		serverError(w, err)
		return
	}
	var logs []models.FileLog
	if err := s.db.Order("logged_at DESC").Offset((page - 1) * size).Limit(size).Find(&logs).Error; err != nil {
		serverError(w, err)
		return
	}
	items := make([]map[string]any, 0, len(logs))
	for _, l := range logs {
		items = append(items, map[string]any{
			"id": l.ID, "file": l.File, "remote_id": l.RemoteID,
			"user_id": l.UserID, "user_ip": l.UserIP, "filesize": l.Filesize,
			"direction": l.Direction, "logged_at": formatTime(l.LoggedAt),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"total": total, "page": page, "size": size, "items": items})
}

// ---------- 分享链接 ----------

func (s *Server) handleSharesList(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	var links []models.ShareLink
	if err := s.db.Where("uid = ?", user.ID).Order("create_time DESC").Limit(100).Find(&links).Error; err != nil {
		serverError(w, err)
		return
	}
	items := make([]map[string]any, 0, len(links))
	for _, l := range links {
		items = append(items, map[string]any{
			"shash": l.SHash, "peers": splitPeers(l.Peers),
			"is_used": l.IsUsed, "is_expired": l.IsExpired,
			"create_time": formatTime(l.CreateTime),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleShareCreate(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	var req struct {
		Peers []string `json:"peers"`
	}
	if err := readJSON(r, &req); err != nil || len(req.Peers) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"code": 0, "msg": "数据为空。"})
		return
	}
	// 仅允许分享自己的设备
	var peers []models.Peer
	if err := s.db.Where("uid = ? AND rid IN ?", user.ID, req.Peers).Find(&peers).Error; err != nil {
		serverError(w, err)
		return
	}
	if len(peers) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"code": 0, "msg": "未找到可分享的设备。"})
		return
	}
	rids := make([]string, 0, len(peers))
	for _, p := range peers {
		rids = append(rids, p.RID)
	}
	link := models.ShareLink{UID: user.ID, SHash: newHash(), Peers: strings.Join(rids, ",")}
	if err := s.db.Create(&link).Error; err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 1, "shash": link.SHash})
}

func (s *Server) handleShareClaim(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	shash := strings.TrimSpace(chi.URLParam(r, "shash"))
	if shash == "" {
		apiError(w, "参数不完整")
		return
	}
	var link models.ShareLink
	if err := s.db.Where("shash = ?", shash).First(&link).Error; err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"code": 0, "msg": "分享链接不存在或已失效。"})
		return
	}
	if link.IsUsed || link.IsExpired {
		writeJSON(w, http.StatusOK, map[string]any{"code": 0, "msg": "分享链接已失效。"})
		return
	}
	if link.UID == user.ID {
		writeJSON(w, http.StatusOK, map[string]any{"code": 0, "msg": "不能把链接分享给自己。"})
		return
	}

	// 复制分享方的地址簿条目到当前用户（跳过已存在的设备）
	var srcPeers []models.Peer
	if err := s.db.Where("uid = ? AND rid IN ?", link.UID, splitPeers(link.Peers)).Find(&srcPeers).Error; err != nil {
		serverError(w, err)
		return
	}
	var ownPeers []models.Peer
	if err := s.db.Where("uid = ?", user.ID).Find(&ownPeers).Error; err != nil {
		serverError(w, err)
		return
	}
	owned := make(map[string]bool, len(ownPeers))
	for _, p := range ownPeers {
		owned[p.RID] = true
	}
	added := make([]string, 0, len(srcPeers))
	err := s.db.Transaction(func(tx *gorm.DB) error {
		for _, p := range srcPeers {
			if owned[p.RID] {
				continue
			}
			np := p
			np.ID = 0
			np.UID = user.ID
			if err := tx.Create(&np).Error; err != nil {
				return err
			}
			added = append(added, p.RID)
		}
		return tx.Model(&models.ShareLink{}).Where("id = ?", link.ID).Update("is_used", true).Error
	})
	if err != nil {
		serverError(w, err)
		return
	}
	msg := "已被成功获取。"
	if len(added) == 0 {
		msg = "这些设备已在你的地址簿中。"
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 1, "msg": msg, "added": added})
}

// ---------- 用户管理（管理员） ----------

func (s *Server) handleUsersList(w http.ResponseWriter, r *http.Request) {
	var users []models.User
	if err := s.db.Order("id ASC").Find(&users).Error; err != nil {
		serverError(w, err)
		return
	}
	items := make([]map[string]any, 0, len(users))
	for _, u := range users {
		items = append(items, map[string]any{
			"id": u.ID, "username": u.Username, "is_admin": u.IsAdmin,
			"is_active": u.IsActive, "created_at": formatTime(u.CreatedAt),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleUserCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		IsAdmin  bool   `json:"is_admin"`
	}
	if err := readJSON(r, &req); err != nil {
		apiError(w, "请求格式错误")
		return
	}
	if len([]rune(req.Username)) <= 3 {
		writeJSON(w, http.StatusOK, map[string]any{"code": 0, "msg": "用户名不得小于3位"})
		return
	}
	if len(req.Password) < 8 || len(req.Password) > 20 {
		writeJSON(w, http.StatusOK, map[string]any{"code": 0, "msg": "密码长度不符合要求, 应在8~20位。"})
		return
	}
	var count int64
	if err := s.db.Model(&models.User{}).Where("username = ?", req.Username).Count(&count).Error; err != nil {
		serverError(w, err)
		return
	}
	if count > 0 {
		writeJSON(w, http.StatusOK, map[string]any{"code": 0, "msg": "用户名已存在。"})
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		serverError(w, err)
		return
	}
	user := models.User{Username: req.Username, PasswordHash: hash, IsActive: true, IsAdmin: req.IsAdmin}
	if err := s.db.Create(&user).Error; err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 1})
}

func (s *Server) handleUserDelete(w http.ResponseWriter, r *http.Request) {
	current := userFromContext(r.Context())
	id, err := strconv.ParseUint(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id == 0 {
		apiError(w, "参数错误")
		return
	}
	if uint(id) == current.ID {
		writeJSON(w, http.StatusOK, map[string]any{"code": 0, "msg": "不能删除当前登录账号。"})
		return
	}
	if err := s.db.Delete(&models.User{}, id).Error; err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 1})
}

func (s *Server) handleUserResetPassword(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseUint(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id == 0 {
		apiError(w, "参数错误")
		return
	}
	var req struct {
		Password string `json:"password"`
	}
	if err := readJSON(r, &req); err != nil {
		apiError(w, "请求格式错误")
		return
	}
	if len(req.Password) < 8 || len(req.Password) > 20 {
		writeJSON(w, http.StatusOK, map[string]any{"code": 0, "msg": "密码长度不符合要求, 应在8~20位。"})
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		serverError(w, err)
		return
	}
	if err := s.db.Model(&models.User{}).Where("id = ?", id).Update("password_hash", hash).Error; err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 1})
}
