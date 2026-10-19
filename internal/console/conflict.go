package console

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/hybridsakura/prism-go-panel/internal/httpx"
	"github.com/hybridsakura/prism-go-panel/internal/models"
)

// ---------- 管理端接口 ----------

// handleIDConflicts 列出 ID 冲突记录（仅管理员）。
func (s *Handler) handleIDConflicts(w http.ResponseWriter, r *http.Request) {
	var rows []models.IDConflict
	if err := s.core.DB.Order("last_seen desc").Limit(500).Find(&rows).Error; err != nil {
		httpx.ServerError(w, err)
		return
	}
	items := make([]map[string]any, 0, len(rows))
	for _, c := range rows {
		var hosts, uuids []string
		_ = json.Unmarshal([]byte(c.Hostnames), &hosts)
		_ = json.Unmarshal([]byte(c.UUIDs), &uuids)
		var devs []models.Device
		_ = s.core.DB.Where("rid = ?", c.RID).Order("update_time desc").Limit(10).Find(&devs)
		devList := make([]map[string]any, 0, len(devs))
		for _, d := range devs {
			devList = append(devList, map[string]any{
				"hostname": d.Hostname, "username": d.Username, "os": d.OS,
				"ip_address": d.IPAddress, "update_time": formatTime(d.UpdateTime),
				"uuid": d.UUID,
			})
		}
		items = append(items, map[string]any{
			"rid": c.RID, "hostnames": hosts, "uuids": uuids,
			"primary_uuid": c.PrimaryUUID, "primary_hostname": c.PrimaryHostname,
			"observations": c.Observations, "blocked": c.Blocked,
			"first_seen": formatTime(c.FirstSeen), "last_seen": formatTime(c.LastSeen),
			"devices": devList,
		})
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
}

// handleIDConflictDelete 清除一条冲突记录（现场改完 ID 后由管理员操作）。
func (s *Handler) handleIDConflictDelete(w http.ResponseWriter, r *http.Request) {
	rid := chi.URLParam(r, "rid")
	if rid == "" {
		httpx.JSON(w, http.StatusOK, map[string]any{"code": 0, "msg": "缺少 RID"})
		return
	}
	if err := s.core.DB.Where("rid = ?", rid).Delete(&models.IDConflict{}).Error; err != nil {
		httpx.ServerError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"code": 1})
}
