package agentapi

import (
	"encoding/json"
	"net/http"
	"time"

	"gorm.io/gorm"

	"github.com/hybridsakura/prism-go-panel/internal/httpx"
	"github.com/hybridsakura/prism-go-panel/internal/models"
	"github.com/hybridsakura/prism-go-panel/internal/platform"
)

// Handler 负责处理自研终端 Agent 的 API 契约（/agent/v1/*）。
type Handler struct {
	core     *platform.Core
	presence *platform.AgentPresence
}

// New 创建 Agent 接口处理器。
func New(core *platform.Core, presence *platform.AgentPresence) *Handler {
	return &Handler{
		core:     core,
		presence: presence,
	}
}

type enrollRequest struct {
	EnrollToken string `json:"enroll_token"`
	Hostname    string `json:"hostname"`
	OS          string `json:"os"`
	Arch        string `json:"arch"`
	Version     string `json:"version"`
	RID         string `json:"rid"`
	Fingerprint string `json:"fingerprint"`
}

// handleEnroll 处理 Agent 首次或重装注册。
func (h *Handler) handleEnroll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.JSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	var req enrollRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.JSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if req.EnrollToken == "" {
		httpx.JSON(w, http.StatusForbidden, map[string]string{"error": "missing enroll token"})
		return
	}

	tokenHash := platform.HashToken(req.EnrollToken)
	var enrollToken models.EnrollToken
	if err := h.core.DB.Where("token_hash = ?", tokenHash).First(&enrollToken).Error; err != nil {
		httpx.JSON(w, http.StatusForbidden, map[string]string{"error": "invalid enroll token"})
		return
	}

	now := time.Now()
	if enrollToken.ExpiresAt != nil && now.After(*enrollToken.ExpiresAt) {
		httpx.JSON(w, http.StatusForbidden, map[string]string{"error": "enroll token expired"})
		return
	}

	// 幂等处理：同一台机器（根据 Fingerprint）重复注册时，返回已有 agent_id 并换发设备令牌，不消耗额度
	if req.Fingerprint != "" {
		var existing models.Agent
		if err := h.core.DB.Where("fingerprint = ? AND status = ?", req.Fingerprint, "active").First(&existing).Error; err == nil {
			devToken, err := platform.GenerateRandomToken()
			if err != nil {
				httpx.ServerError(w, err)
				return
			}
			newHash := platform.HashToken(devToken)
			_ = h.core.DB.Model(&existing).Updates(map[string]any{
				"token_hash": newHash,
				"hostname":   req.Hostname,
				"os":         req.OS,
				"arch":       req.Arch,
				"version":    req.Version,
				"ip":         httpx.ClientIP(r),
				"rid":        req.RID,
			}).Error

			httpx.JSON(w, http.StatusOK, map[string]any{
				"agent_id":     existing.ID,
				"device_token": devToken,
			})
			return
		}
	}

	// 新机器注册：核验使用次数限制
	if enrollToken.MaxUses > 0 && enrollToken.Used >= enrollToken.MaxUses {
		httpx.JSON(w, http.StatusForbidden, map[string]string{"error": "enroll token exhausted"})
		return
	}

	devToken, err := platform.GenerateRandomToken()
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	devHash := platform.HashToken(devToken)

	var newAgent models.Agent
	err = h.core.DB.Transaction(func(tx *gorm.DB) error {
		newAgent = models.Agent{
			TokenHash:   devHash,
			Fingerprint: req.Fingerprint,
			RID:         req.RID,
			Hostname:    req.Hostname,
			OS:          req.OS,
			Arch:        req.Arch,
			Version:     req.Version,
			IP:          httpx.ClientIP(r),
			GroupName:   enrollToken.GroupName,
			Status:      "active",
		}
		if err := tx.Create(&newAgent).Error; err != nil {
			return err
		}

		return tx.Model(&enrollToken).Update("used", enrollToken.Used+1).Error
	})

	if err != nil {
		httpx.ServerError(w, err)
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]any{
		"agent_id":     newAgent.ID,
		"device_token": devToken,
	})
}

type heartbeatRequest struct {
	Version  string `json:"version"`
	RID      string `json:"rid"`
	Hostname string `json:"hostname"`
}

// handleHeartbeat 处理 Agent 心跳与任务捎带。
func (h *Handler) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.JSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	rawToken := httpx.BearerToken(r)
	if rawToken == "" {
		httpx.JSON(w, http.StatusUnauthorized, map[string]string{"error": "missing device token"})
		return
	}

	tokenHash := platform.HashToken(rawToken)
	var agent models.Agent
	if err := h.core.DB.Where("token_hash = ?", tokenHash).First(&agent).Error; err != nil {
		httpx.JSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid device token"})
		return
	}
	if agent.Status != "active" {
		httpx.JSON(w, http.StatusUnauthorized, map[string]string{"error": "agent disabled"})
		return
	}

	var req heartbeatRequest
	_ = json.NewDecoder(r.Body).Decode(&req)

	// 心跳时间仅更新内存，避免低功耗机器每次心跳写库
	h.presence.Touch(agent.ID, time.Now())

	httpx.JSON(w, http.StatusOK, map[string]any{
		"interval":    int(platform.HeartbeatInterval / time.Second),
		"server_time": time.Now().Unix(),
		"tasks":       []any{},
	})
}
