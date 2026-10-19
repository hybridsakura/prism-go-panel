// Package platform 是平台的领域层：设备、在线状态、ID 冲突等跨传输层共用的业务能力。
//
// 传输层（clientapi 面向 RustDesk 客户端契约，console 面向管理后台，
// 将来的 agentapi 面向自研 Agent）都只做协议翻译，业务判断集中在这里，
// 避免同一套规则在多个入口重复实现。
package platform

import (
	"encoding/json"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"github.com/hybridsakura/prism-go-panel/internal/config"
	"github.com/hybridsakura/prism-go-panel/internal/models"
	"github.com/hybridsakura/prism-go-panel/internal/stats"
)

// Core 汇聚领域层所需的依赖。字段导出以便各传输层直接使用，
// 但业务规则应尽量以方法形式收敛在本包。
type Core struct {
	DB      *gorm.DB
	Cfg     *config.Config
	Tracker *stats.Tracker
}

// New 创建领域层核心。
func New(cfg *config.Config, db *gorm.DB, tracker *stats.Tracker) *Core {
	return &Core{DB: db, Cfg: cfg, Tracker: tracker}
}

// ---------- ID 冲突 ----------

// RecordIDConflict 记录/合并一次 ID 冲突观察。原机取最早注册的那台。
//
// 背景：硬盘拷贝机克隆系统时会把 RustDesk 配置（含 ID）一起复制，
// 多台机器共用一个远程 ID；hbbs 每个 ID 只保存一个公钥，
// 另一台连接时签名校验失败，客户端提示「无法验证端到端加密」。
func (c *Core) RecordIDConflict(rid string, hostnames, uuids []string) {
	hostnames = uniqNonEmpty(hostnames)
	uuids = uniqNonEmpty(uuids)
	if len(hostnames) < 2 && len(uuids) < 2 {
		return
	}
	hostJSON, _ := json.Marshal(hostnames)
	uuidJSON, _ := json.Marshal(uuids)

	// 原机 = 最早创建的那条设备记录
	var oldest models.Device
	_ = c.DB.Where("rid = ?", rid).Order("create_time asc").First(&oldest).Error

	var row models.IDConflict
	if err := c.DB.Where("rid = ?", rid).First(&row).Error; err != nil {
		_ = c.DB.Create(&models.IDConflict{
			RID: rid, Hostnames: string(hostJSON), UUIDs: string(uuidJSON),
			PrimaryUUID: oldest.UUID, PrimaryHostname: oldest.Hostname,
			Observations: 1,
		}).Error
		return
	}
	var oldH, oldU []string
	_ = json.Unmarshal([]byte(row.Hostnames), &oldH)
	_ = json.Unmarshal([]byte(row.UUIDs), &oldU)
	mergedH, _ := json.Marshal(uniqNonEmpty(append(oldH, hostnames...)))
	mergedU, _ := json.Marshal(uniqNonEmpty(append(oldU, uuids...)))
	updates := map[string]any{
		"hostnames":    string(mergedH),
		"uuids":        string(mergedU),
		"observations": row.Observations + 1,
	}
	if row.PrimaryUUID == "" && oldest.UUID != "" {
		updates["primary_uuid"] = oldest.UUID
		updates["primary_hostname"] = oldest.Hostname
	}
	_ = c.DB.Model(&row).Updates(updates).Error
}

// FindConflict 查找某 RID 的冲突记录。
func (c *Core) FindConflict(rid string) (*models.IDConflict, bool) {
	if rid == "" {
		return nil, false
	}
	var row models.IDConflict
	if err := c.DB.Where("rid = ?", rid).First(&row).Error; err != nil {
		return nil, false
	}
	return &row, true
}

// ConflictMessage 生成给客户端看的提示文案（客户端会弹窗展示 error 内容）。
func ConflictMessage(row *models.IDConflict) string {
	var hosts []string
	_ = json.Unmarshal([]byte(row.Hostnames), &hosts)
	detail := ""
	if len(hosts) > 0 {
		detail = "（涉及主机：" + strings.Join(hosts, " / ") + "）"
	}
	owner := ""
	if row.PrimaryHostname != "" {
		owner = "，该 ID 已归属 " + row.PrimaryHostname
	}
	return fmt.Sprintf(
		"本机的远程 ID %s 与其他设备重复%s%s。\n"+
			"共用同一个 ID 会导致远程连接提示「无法验证端到端加密」。\n"+
			"请打开 RustDesk → 设置 → ID → 更改 ID，生成唯一 ID 后重新登录。",
		row.RID, owner, detail)
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
