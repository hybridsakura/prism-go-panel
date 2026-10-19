package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/hybridsakura/prism-go-panel/internal/models"
)

// ID 冲突检测
//
// 背景：用硬盘拷贝机批量装系统时，RustDesk 的配置文件（含 ID）会被一起复制，
// 导致多台物理机共用一个远程 ID。hbbs 的 peer 表每个 ID 只保存一个公钥，
// 于是另一台机器的签名校验必然失败，客户端弹出
// 「无法验证端到端加密…服务器可能不受信任」。
//
// 判定：同一个 RID 上报过不同的主机名或不同的机器 UUID。
// 处理：记入 id_conflicts；登录时若请求方身份与「原机」不同，则拒绝并提示更换 ID
// （客户端会原样展示 error 文案）。原机始终放行，避免误伤。

// recordIDConflict 记录/合并一次 ID 冲突观察。primary 取最早注册的那台机器。
func (s *Server) recordIDConflict(rid string, hostnames, uuids []string) {
	hostnames = uniqNonEmpty(hostnames)
	uuids = uniqNonEmpty(uuids)
	if len(hostnames) < 2 && len(uuids) < 2 {
		return
	}
	hostJSON, _ := json.Marshal(hostnames)
	uuidJSON, _ := json.Marshal(uuids)

	// 原机 = 最早创建的那条设备记录
	var oldest models.Device
	_ = s.db.Where("rid = ?", rid).Order("create_time asc").First(&oldest).Error

	var c models.IDConflict
	if err := s.db.Where("rid = ?", rid).First(&c).Error; err != nil {
		c = models.IDConflict{
			RID: rid, Hostnames: string(hostJSON), UUIDs: string(uuidJSON),
			PrimaryUUID: oldest.UUID, PrimaryHostname: oldest.Hostname,
			Observations: 1,
		}
		_ = s.db.Create(&c).Error
		return
	}
	var oldH, oldU []string
	_ = json.Unmarshal([]byte(c.Hostnames), &oldH)
	_ = json.Unmarshal([]byte(c.UUIDs), &oldU)
	mergedH, _ := json.Marshal(uniqNonEmpty(append(oldH, hostnames...)))
	mergedU, _ := json.Marshal(uniqNonEmpty(append(oldU, uuids...)))
	updates := map[string]any{
		"hostnames":    string(mergedH),
		"uuids":        string(mergedU),
		"observations": c.Observations + 1,
	}
	if c.PrimaryUUID == "" && oldest.UUID != "" {
		updates["primary_uuid"] = oldest.UUID
		updates["primary_hostname"] = oldest.Hostname
	}
	_ = s.db.Model(&c).Updates(updates).Error
}

func uniqNonEmpty(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

// findConflict 查找某 RID 的冲突记录。
func (s *Server) findConflict(rid string) (*models.IDConflict, bool) {
	if rid == "" {
		return nil, false
	}
	var c models.IDConflict
	if err := s.db.Where("rid = ?", rid).First(&c).Error; err != nil {
		return nil, false
	}
	return &c, true
}

// conflictMessage 生成给客户端看的提示文案（客户端会弹窗展示 error 内容）。
func conflictMessage(c *models.IDConflict) string {
	var hosts []string
	_ = json.Unmarshal([]byte(c.Hostnames), &hosts)
	detail := ""
	if len(hosts) > 0 {
		detail = "（涉及主机：" + strings.Join(hosts, " / ") + "）"
	}
	owner := ""
	if c.PrimaryHostname != "" {
		owner = "，该 ID 已归属 " + c.PrimaryHostname
	}
	return fmt.Sprintf(
		"本机的远程 ID %s 与其他设备重复%s%s。\n"+
			"共用同一个 ID 会导致远程连接提示「无法验证端到端加密」。\n"+
			"请打开 RustDesk → 设置 → ID → 更改 ID，生成唯一 ID 后重新登录。",
		c.RID, owner, detail)
}

// ---------- 管理端接口 ----------

// handleIDConflicts 列出 ID 冲突记录（仅管理员）。
func (s *Server) handleIDConflicts(w http.ResponseWriter, r *http.Request) {
	var rows []models.IDConflict
	if err := s.db.Order("last_seen desc").Limit(500).Find(&rows).Error; err != nil {
		serverError(w, err)
		return
	}
	items := make([]map[string]any, 0, len(rows))
	for _, c := range rows {
		var hosts, uuids []string
		_ = json.Unmarshal([]byte(c.Hostnames), &hosts)
		_ = json.Unmarshal([]byte(c.UUIDs), &uuids)
		var devs []models.Device
		_ = s.db.Where("rid = ?", c.RID).Order("update_time desc").Limit(10).Find(&devs)
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
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
}

// handleIDConflictDelete 清除一条冲突记录（现场改完 ID 后由管理员操作）。
func (s *Server) handleIDConflictDelete(w http.ResponseWriter, r *http.Request) {
	rid := chi.URLParam(r, "rid")
	if rid == "" {
		writeJSON(w, http.StatusOK, map[string]any{"code": 0, "msg": "缺少 RID"})
		return
	}
	if err := s.db.Where("rid = ?", rid).Delete(&models.IDConflict{}).Error; err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 1})
}
