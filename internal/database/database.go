// Package database 负责数据库连接、连接池调优与自动迁移。
package database

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/glebarez/sqlite" // 纯 Go SQLite 驱动（无需 CGO）
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/hybridsakura/prism-go-panel/internal/auth"
	"github.com/hybridsakura/prism-go-panel/internal/config"
	"github.com/hybridsakura/prism-go-panel/internal/models"
)

// shareTTL 分享链接有效期，与 Python 参考版一致（15 分钟）。
const shareTTL = 15 * time.Minute

// slogWriter 把 GORM 的日志输出桥接到 slog。
type slogWriter struct{}

func (slogWriter) Printf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	// GORM 的错误日志（Error/Trace）提升为 Warn，其余按 Info 记录
	slog.Warn("gorm", "msg", msg)
}

// Open 根据配置打开数据库（默认 SQLite，DSN 以 mysql:// 开头或包含 @tcp( 时使用 MySQL），
// 并执行自动迁移。SQLite 采用 WAL 模式以支持读写并发。
func Open(cfg *config.Config) (*gorm.DB, error) {
	mysqlMode := strings.HasPrefix(cfg.DBURL, "mysql://") || strings.Contains(cfg.DBURL, "@tcp(")

	var dialector gorm.Dialector
	if mysqlMode {
		dialector = mysql.Open(strings.TrimPrefix(cfg.DBURL, "mysql://"))
	} else {
		if dir := filepath.Dir(sqliteFilePath(cfg.DBURL)); dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, fmt.Errorf("创建数据库目录失败: %w", err)
			}
		}
		sep := "?"
		if strings.Contains(cfg.DBURL, "?") {
			sep = "&" // DSN 已带查询参数（如 file:x.db?cache=shared），追加而非新建
		}
		dsn := cfg.DBURL + sep +
			"_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(1)"
		dialector = sqlite.Open(dsn)
	}

	db, err := gorm.Open(dialector, &gorm.Config{
		PrepareStmt:            true, // 预编译语句，热点路径免去重复解析
		SkipDefaultTransaction: true, // 单条写入跳过显式事务，降低往返
		Logger: gormlogger.New(slogWriter{}, gormlogger.Config{
			SlowThreshold:             300 * time.Millisecond,
			LogLevel:                  gormlogger.Warn,
			IgnoreRecordNotFoundError: true, // token 校验等场景会频繁查不到记录，避免噪音
		}),
	})
	if err != nil {
		return nil, fmt.Errorf("打开数据库失败: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("获取底层连接池失败: %w", err)
	}
	if mysqlMode {
		sqlDB.SetMaxOpenConns(32)
		sqlDB.SetMaxIdleConns(8)
	} else {
		// SQLite 写操作串行化，WAL 模式下读取可并发
		sqlDB.SetMaxOpenConns(16)
		sqlDB.SetMaxIdleConns(4)
	}
	sqlDB.SetConnMaxLifetime(time.Hour)

	if !mysqlMode {
		// DSN 内的 busy_timeout 为连接级设置；journal_mode 为库级持久设置，此处兜底再执行一次
		_ = db.Exec("PRAGMA journal_mode=WAL").Error
	}

	if err := db.AutoMigrate(
		&models.User{},
		&models.Token{},
		&models.Tag{},
		&models.Peer{},
		&models.Device{},
		&models.ConnLog{},
		&models.FileLog{},
		&models.ShareLink{},
		&models.ServerProfile{},
		&models.IDConflict{},
		&models.EnrollToken{},
		&models.Agent{},
	); err != nil {
		return nil, fmt.Errorf("数据库迁移失败: %w", err)
	}
	return db, nil
}

// sqliteFilePath 从 SQLite DSN 中提取文件系统路径，兼容 file: URI 与查询参数。
// 例："file:data/app.db?cache=shared" -> "data/app.db"
func sqliteFilePath(dsn string) string {
	p := strings.TrimPrefix(dsn, "file:")
	if i := strings.IndexByte(p, '?'); i >= 0 {
		p = p[:i]
	}
	return p
}

// SeedAdmin 在配置了管理员账号且用户表为空时创建初始管理员（首个用户即管理员）。
func SeedAdmin(db *gorm.DB, username, password string) error {
	if username == "" || password == "" {
		return nil
	}
	var count int64
	if err := db.Model(&models.User{}).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	admin := models.User{Username: username, PasswordHash: hash, IsActive: true, IsAdmin: true}
	if err := db.Create(&admin).Error; err != nil {
		return fmt.Errorf("创建初始管理员失败: %w", err)
	}
	return nil
}

// MarkExpiredShareLinks 将超过 shareTTL 且未被使用的分享链接标记为过期。
func MarkExpiredShareLinks(db *gorm.DB) error {
	deadline := time.Now().Add(-shareTTL)
	return db.Model(&models.ShareLink{}).
		Where("is_used = ? AND is_expired = ? AND create_time < ?", false, false, deadline).
		Update("is_expired", true).Error
}
