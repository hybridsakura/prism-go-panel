// Command legacy-import 把 Python 版（kingmo888/rustdesk-api-server，Django）的
// 数据迁移到本服务：用户、设备资产、连接日志、文件日志，并据历史数据生成 ID 冲突记录。
//
// 用法：
//
//	legacy-import -src /path/to/old/db.sqlite3 -dst /path/to/prism.db [-dry-run]
//
// 密码哈希为 Django 的 pbkdf2_sha256，本服务已兼容校验，用户可继续用原密码登录。
package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	_ "github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/hybridsakura/prism-go-panel/internal/models"
)

type legacyUser struct {
	ID        int
	Username  string
	Password  string
	RID       sql.NullString
	UUID      sql.NullString
	AutoLogin sql.NullBool
	RType     sql.NullString
	Device    sql.NullString
	IsActive  sql.NullBool
	IsAdmin   sql.NullBool
	LastLogin sql.NullString
}

type legacyDevice struct {
	RID      sql.NullString
	CPU      sql.NullString
	Hostname sql.NullString
	Memory   sql.NullString
	OS       sql.NullString
	UUID     sql.NullString
	Username sql.NullString
	Version  sql.NullString
	IP       sql.NullString
	Created  sql.NullString
	Updated  sql.NullString
}

func main() {
	src := flag.String("src", "", "旧版 Django 数据库路径（db.sqlite3）")
	dst := flag.String("dst", "", "本服务数据库路径（prism.db）")
	dryRun := flag.Bool("dry-run", false, "只统计不写入")
	flag.Parse()
	if *src == "" || *dst == "" {
		fmt.Fprintln(os.Stderr, "必须指定 -src 与 -dst")
		flag.Usage()
		os.Exit(2)
	}

	old, err := sql.Open("sqlite", *src)
	if err != nil {
		log.Fatalf("打开旧库失败: %v", err)
	}
	defer old.Close()

	db, err := gorm.Open(sqlite.Open(*dst), &gorm.Config{})
	if err != nil {
		log.Fatalf("打开目标库失败: %v", err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.Token{}, &models.Tag{}, &models.Peer{},
		&models.Device{}, &models.ConnLog{}, &models.FileLog{}, &models.ShareLink{},
		&models.ServerProfile{}, &models.IDConflict{}); err != nil {
		log.Fatalf("迁移目标表失败: %v", err)
	}

	nUsers := importUsers(db, old, *dryRun)
	nDevices := importDevices(db, old, *dryRun)
	nConn, nFile := importLogs(db, old, *dryRun)
	nConf := seedConflicts(db, *dryRun)

	fmt.Printf("\n完成：用户 %d，设备 %d，连接日志 %d，文件日志 %d，ID 冲突 %d\n",
		nUsers, nDevices, nConn, nFile, nConf)
	if *dryRun {
		fmt.Println("（dry-run：未写入任何数据）")
	}
}

func importUsers(db *gorm.DB, old *sql.DB, dry bool) int {
	rows, err := old.Query(`select id, username, password, rid, uuid, "autoLogin", rtype, "deviceInfo", "is_active", "is_admin", last_login from api_userprofile`)
	if err != nil {
		fmt.Printf("读取用户失败（跳过）: %v\n", err)
		return 0
	}
	defer rows.Close()
	n := 0
	var created time.Time
	for rows.Next() {
		var u legacyUser
		if err := rows.Scan(&u.ID, &u.Username, &u.Password, &u.RID, &u.UUID, &u.AutoLogin,
			&u.RType, &u.Device, &u.IsActive, &u.IsAdmin, &u.LastLogin); err != nil {
			continue
		}
		created = parseLegacyTime(u.LastLogin.String)
		if created.IsZero() {
			created = time.Now()
		}
		user := models.User{
			Username:     u.Username,
			PasswordHash: u.Password,
			RID:          u.RID.String,
			UUID:         u.UUID.String,
			AutoLogin:    u.AutoLogin.Bool,
			RType:        u.RType.String,
			DeviceInfo:   u.Device.String,
			IsActive:     u.IsActive.Bool,
			IsAdmin:      u.IsAdmin.Bool,
			CreatedAt:    created,
		}
		if !dry {
			if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&user).Error; err != nil {
				fmt.Printf("  用户 %s 写入失败: %v\n", u.Username, err)
				continue
			}
		}
		n++
		fmt.Printf("  用户: %-12s 管理员=%-5v 哈希=%s...\n", u.Username, u.IsAdmin.Bool, truncate(u.Password, 20))
	}
	return n
}

func importDevices(db *gorm.DB, old *sql.DB, dry bool) int {
	rows, err := old.Query(`select rid, cpu, hostname, memory, os, uuid, username, version, ip_address, create_time, update_time from api_rustdesdevice`)
	if err != nil {
		fmt.Printf("读取设备失败（跳过）: %v\n", err)
		return 0
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var d legacyDevice
		if err := rows.Scan(&d.RID, &d.CPU, &d.Hostname, &d.Memory, &d.OS, &d.UUID,
			&d.Username, &d.Version, &d.IP, &d.Created, &d.Updated); err != nil {
			continue
		}
		if strings.TrimSpace(d.RID.String) == "" || strings.TrimSpace(d.UUID.String) == "" {
			continue
		}
		dev := models.Device{
			RID: d.RID.String, CPU: d.CPU.String, Hostname: d.Hostname.String,
			Memory: d.Memory.String, OS: d.OS.String, UUID: d.UUID.String,
			Username: d.Username.String, Version: d.Version.String, IPAddress: d.IP.String,
			CreateTime: parseLegacyTime(d.Created.String), UpdateTime: parseLegacyTime(d.Updated.String),
		}
		if !dry {
			if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&dev).Error; err != nil {
				continue
			}
		}
		n++
	}
	fmt.Printf("  设备资产: %d 条\n", n)
	return n
}

func importLogs(db *gorm.DB, old *sql.DB, dry bool) (int, int) {
	nConn := 0
	if rows, err := old.Query(`select action, conn_id, from_ip, from_id, rid, conn_start, conn_end, session_id, uuid from api_connlog`); err == nil {
		var batch []models.ConnLog
		for rows.Next() {
			var action, connID, fromIP, fromID, rid, start, end, session, uuid sql.NullString
			if err := rows.Scan(&action, &connID, &fromIP, &fromID, &rid, &start, &end, &session, &uuid); err != nil {
				continue
			}
			cl := models.ConnLog{
				Action: action.String, ConnID: connID.String, FromIP: fromIP.String,
				FromID: fromID.String, RID: rid.String, ConnStart: parseLegacyTime(start.String),
				SessionID: session.String, UUID: uuid.String,
			}
			if t := parseLegacyTime(end.String); !t.IsZero() {
				cl.ConnEnd = &t
			}
			batch = append(batch, cl)
			nConn++
		}
		rows.Close()
		if !dry && len(batch) > 0 {
			_ = db.CreateInBatches(&batch, 200).Error
		}
	}
	nFile := 0
	if rows, err := old.Query(`select file, remote_id, user_id, user_ip, filesize, direction, logged_at from api_filelog`); err == nil {
		var batch []models.FileLog
		for rows.Next() {
			var file, remote, userID, ip, size, logged sql.NullString
			var dir sql.NullInt64
			if err := rows.Scan(&file, &remote, &userID, &ip, &size, &dir, &logged); err != nil {
				continue
			}
			batch = append(batch, models.FileLog{
				File: file.String, RemoteID: remote.String, UserID: userID.String,
				UserIP: ip.String, Filesize: size.String, Direction: int(dir.Int64),
				LoggedAt: parseLegacyTime(logged.String),
			})
			nFile++
		}
		rows.Close()
		if !dry && len(batch) > 0 {
			_ = db.CreateInBatches(&batch, 200).Error
		}
	}
	fmt.Printf("  日志: 连接 %d 条，文件 %d 条\n", nConn, nFile)
	return nConn, nFile
}

// seedConflicts 根据历史设备数据生成 ID 冲突记录（同一 RID 多个主机名/UUID）。
func seedConflicts(db *gorm.DB, dry bool) int {
	var devs []models.Device
	if err := db.Order("create_time asc").Find(&devs).Error; err != nil {
		return 0
	}
	hosts := map[string]map[string]bool{}
	uuids := map[string]map[string]bool{}
	oldest := map[string]models.Device{}
	for _, d := range devs {
		if hosts[d.RID] == nil {
			hosts[d.RID] = map[string]bool{}
			uuids[d.RID] = map[string]bool{}
		}
		if d.Hostname != "" {
			hosts[d.RID][d.Hostname] = true
		}
		if d.UUID != "" {
			uuids[d.RID][d.UUID] = true
		}
		if _, ok := oldest[d.RID]; !ok {
			oldest[d.RID] = d
		}
	}
	n := 0
	for rid, hs := range hosts {
		if len(hs) < 2 && len(uuids[rid]) < 2 {
			continue
		}
		h := keys(hs)
		u := keys(uuids[rid])
		hj, _ := json.Marshal(h)
		uj, _ := json.Marshal(u)
		c := models.IDConflict{
			RID: rid, Hostnames: string(hj), UUIDs: string(uj),
			PrimaryUUID: oldest[rid].UUID, PrimaryHostname: oldest[rid].Hostname,
			Observations: 1,
		}
		if !dry {
			_ = db.Clauses(clause.OnConflict{DoNothing: true}).Create(&c).Error
		}
		n++
		fmt.Printf("  ID 冲突: %s → 主机 %v\n", rid, h)
	}
	return n
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// parseLegacyTime 解析 Django 风格时间（"2026-09-22 10:32:01.582977"），兼容 RFC3339。
func parseLegacyTime(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{
		"2006-01-02 15:04:05.999999", "2006-01-02 15:04:05",
		time.RFC3339Nano, time.RFC3339,
	} {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return t
		}
	}
	return time.Time{}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
