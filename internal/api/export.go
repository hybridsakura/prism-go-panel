package api

import (
	"net/http"
	"time"

	"github.com/xuri/excelize/v2"

	"github.com/hybridsakura/prism-go-panel/internal/models"
)

// handleExportDevices 导出全部设备资产为 xlsx（仅管理员）。
// 列与 Python 参考版的 down_peers 一致，并附加在线状态与所属用户。
func (s *Server) handleExportDevices(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if user == nil || !user.IsAdmin {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "需要管理员权限"})
		return
	}

	var devices []models.Device
	if err := s.db.Order("update_time DESC").Find(&devices).Error; err != nil {
		serverError(w, err)
		return
	}
	rids := make([]string, 0, len(devices))
	for _, d := range devices {
		rids = append(rids, d.RID)
	}
	_, ownerByRID := s.peerInfoForRIDs(rids)

	f := excelize.NewFile()
	defer f.Close()
	sheet := "设备信息"
	if err := f.SetSheetName(f.GetSheetName(0), sheet); err != nil {
		serverError(w, err)
		return
	}

	headers := []string{"rid", "cpu", "hostname", "memory", "os", "uuid", "username",
		"version", "ip_address", "create_time", "update_time", "rust_user", "status"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellValue(sheet, cell, h)
	}
	style, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"D3D1C7"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center"},
	})
	_ = f.SetCellStyle(sheet, "A1", "M1", style)

	now := time.Now()
	for i, d := range devices {
		row := i + 2
		status := "离线"
		if s.tracker.Online(d.RID) {
			status = "在线"
		}
		owner := ownerByRID[d.RID]
		if owner == "" {
			owner = "未登录"
		}
		values := []any{
			d.RID, d.CPU, d.Hostname, d.Memory, d.OS, d.UUID, d.Username,
			d.Version, d.IPAddress,
			d.CreateTime.Local().Format("2006-01-02 15:04"),
			d.UpdateTime.Local().Format("2006-01-02 15:04"),
			owner, status,
		}
		for j, v := range values {
			cell, _ := excelize.CoordinatesToCellName(j+1, row)
			_ = f.SetCellValue(sheet, cell, v)
		}
	}
	_ = f.SetColWidth(sheet, "A", "M", 18)

	filename := "DeviceInfo-" + now.Format("20060102-150405") + ".xlsx"
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", "attachment; filename="+filename)
	if err := f.Write(w); err != nil {
		return
	}
}
