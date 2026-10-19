package console

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/hybridsakura/prism-go-panel/internal/httpx"
	"github.com/hybridsakura/prism-go-panel/internal/models"
	"github.com/hybridsakura/prism-go-panel/internal/platform"
)

// handleAgentsList 列出所有已注册的 Agent 实例及实时在线状态。
func (s *Handler) handleAgentsList(w http.ResponseWriter, r *http.Request) {
	page, size := parsePage(r)
	var agents []models.Agent
	var total int64

	q := s.core.DB.Model(&models.Agent{})
	if err := q.Count(&total).Error; err != nil {
		httpx.ServerError(w, err)
		return
	}
	if err := q.Order("id DESC").Offset((page - 1) * size).Limit(size).Find(&agents).Error; err != nil {
		httpx.ServerError(w, err)
		return
	}

	now := time.Now()
	type agentItem struct {
		ID         uint   `json:"id"`
		Hostname   string `json:"hostname"`
		OS         string `json:"os"`
		Arch       string `json:"arch"`
		Version    string `json:"version"`
		RID        string `json:"rid"`
		IP         string `json:"ip"`
		GroupName  string `json:"group_name"`
		Status     string `json:"status"` // active | disabled
		Online     bool   `json:"online"`
		LastSeenAt string `json:"last_seen_at"`
		EnrolledAt string `json:"enrolled_at"`
	}

	items := make([]agentItem, 0, len(agents))
	for _, a := range agents {
		isOnline := false
		var lastSeen time.Time
		if t, ok := s.presence.LastSeen(a.ID); ok {
			lastSeen = t
		} else if a.LastSeenAt != nil {
			lastSeen = *a.LastSeenAt
		}
		// 30 秒（3 个心跳周期）内有心跳则视作在线
		if !lastSeen.IsZero() && now.Sub(lastSeen) <= 30*time.Second && a.Status == "active" {
			isOnline = true
		}

		items = append(items, agentItem{
			ID:         a.ID,
			Hostname:   a.Hostname,
			OS:         a.OS,
			Arch:       a.Arch,
			Version:    a.Version,
			RID:        a.RID,
			IP:         a.IP,
			GroupName:  a.GroupName,
			Status:     a.Status,
			Online:     isOnline,
			LastSeenAt: formatTime(lastSeen),
			EnrolledAt: formatTime(a.EnrolledAt),
		})
	}

	httpx.JSON(w, http.StatusOK, map[string]any{
		"total": total,
		"page":  page,
		"size":  size,
		"items": items,
	})
}

// handleAgentStatus 切换 Agent 的启用/禁用状态。
func (s *Handler) handleAgentStatus(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseUint(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id == 0 {
		httpx.Error(w, "无效的 Agent ID")
		return
	}

	var req struct {
		Status string `json:"status"` // active | disabled
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || (req.Status != "active" && req.Status != "disabled") {
		httpx.Error(w, "状态参数不合法，必须为 active 或 disabled")
		return
	}

	if err := s.core.DB.Model(&models.Agent{}).Where("id = ?", id).Update("status", req.Status).Error; err != nil {
		httpx.ServerError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"code": 1})
}

// handleEnrollTokensList 列出所有注册码。
func (s *Handler) handleEnrollTokensList(w http.ResponseWriter, r *http.Request) {
	var tokens []models.EnrollToken
	if err := s.core.DB.Order("id DESC").Find(&tokens).Error; err != nil {
		httpx.ServerError(w, err)
		return
	}

	now := time.Now()
	type tokenItem struct {
		ID        uint   `json:"id"`
		GroupName string `json:"group_name"`
		MaxUses   int    `json:"max_uses"`
		Used      int    `json:"used"`
		Expired   bool   `json:"expired"`
		ExpiresAt string `json:"expires_at"`
		Note      string `json:"note"`
		CreatedAt string `json:"created_at"`
	}

	items := make([]tokenItem, 0, len(tokens))
	for _, t := range tokens {
		isExpired := false
		if t.ExpiresAt != nil && now.After(*t.ExpiresAt) {
			isExpired = true
		}
		expStr := ""
		if t.ExpiresAt != nil {
			expStr = formatTime(*t.ExpiresAt)
		}

		items = append(items, tokenItem{
			ID:        t.ID,
			GroupName: t.GroupName,
			MaxUses:   t.MaxUses,
			Used:      t.Used,
			Expired:   isExpired,
			ExpiresAt: expStr,
			Note:      t.Note,
			CreatedAt: formatTime(t.CreatedAt),
		})
	}

	httpx.JSON(w, http.StatusOK, map[string]any{
		"items": items,
		"total": len(items),
	})
}

// handleEnrollTokenCreate 生成新注册码，返回生成的明文（仅返回一次）。
func (s *Handler) handleEnrollTokenCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		GroupName   string `json:"group_name"`
		MaxUses     int    `json:"max_uses"`     // <= 0 表示无限制
		ExpiresDays int    `json:"expires_days"` // <= 0 表示永不过期
		Note        string `json:"note"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	rawToken, err := platform.GenerateRandomToken()
	if err != nil {
		httpx.ServerError(w, err)
		return
	}

	var expiresAt *time.Time
	if req.ExpiresDays > 0 {
		t := time.Now().AddDate(0, 0, req.ExpiresDays)
		expiresAt = &t
	}

	if req.MaxUses <= 0 {
		req.MaxUses = 1
	}

	record := models.EnrollToken{
		TokenHash: platform.HashToken(rawToken),
		GroupName: req.GroupName,
		MaxUses:   req.MaxUses,
		ExpiresAt: expiresAt,
		Note:      req.Note,
	}

	if err := s.core.DB.Create(&record).Error; err != nil {
		httpx.ServerError(w, err)
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]any{
		"code":         1,
		"id":           record.ID,
		"enroll_token": rawToken, // 供管理员复制
	})
}

// handleEnrollTokenDelete 删除指定注册码。
func (s *Handler) handleEnrollTokenDelete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseUint(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id == 0 {
		httpx.Error(w, "参数错误")
		return
	}

	if err := s.core.DB.Delete(&models.EnrollToken{}, id).Error; err != nil {
		httpx.ServerError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"code": 1})
}
