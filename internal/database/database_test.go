package database

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hybridsakura/prism-go-panel/internal/config"
)

func TestSqliteFilePath(t *testing.T) {
	cases := map[string]string{
		"./prism.db":                    "./prism.db",
		"data/app.db":                   "data/app.db",
		"file:data/app.db":              "data/app.db",
		"file:data/app.db?cache=shared": "data/app.db",
		":memory:":                      ":memory:",
		"file::memory:?cache=shared":    ":memory:",
		`/abs/path/db.sqlite?_pragma=x`: `/abs/path/db.sqlite`,
	}
	for in, want := range cases {
		if got := sqliteFilePath(in); got != want {
			t.Errorf("sqliteFilePath(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestOpenSQLiteVariants 覆盖普通路径、file: URI、带查询参数、自动建目录四种形态。
func TestOpenSQLiteVariants(t *testing.T) {
	t.Chdir(t.TempDir())

	variants := []string{
		"plain.db",
		"file:uri.db",
		"file:subdir/nested.db",
		"file:q.db?cache=shared",
	}
	for _, dsn := range variants {
		cfg := &config.Config{
			DBURL:         dsn,
			TokenTTL:      time.Hour,
			OnlineTimeout: 120 * time.Second,
		}
		db, err := Open(cfg)
		if err != nil {
			t.Fatalf("Open(%q) 失败: %v", dsn, err)
		}
		sqlDB, err := db.DB()
		if err != nil {
			t.Fatalf("Open(%q) 获取连接池失败: %v", dsn, err)
		}
		// 简单读写验证
		if err := db.Exec("CREATE TABLE IF NOT EXISTS smoke (id INTEGER)").Error; err != nil {
			t.Fatalf("Open(%q) 写入失败: %v", dsn, err)
		}
		_ = sqlDB.Close()
	}

	// 自动建目录生效
	if _, err := os.Stat(filepath.Join("subdir", "nested.db")); err != nil {
		t.Fatalf("嵌套目录数据库文件未创建: %v", err)
	}
}

// TestSeedAdmin 初始管理员仅在空库创建一次。
func TestSeedAdmin(t *testing.T) {
	cfg := &config.Config{DBURL: filepath.Join(t.TempDir(), "seed.db"), TokenTTL: time.Hour}
	db, err := Open(cfg)
	if err != nil {
		t.Fatalf("Open 失败: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})

	if err := SeedAdmin(db, "adminseed", "password123"); err != nil {
		t.Fatalf("首次 SeedAdmin 失败: %v", err)
	}
	var count int64
	if err := db.Table("users").Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("应创建 1 个管理员，实际 %d, err=%v", count, err)
	}
	// 非空库再次调用应为 no-op
	if err := SeedAdmin(db, "another", "password123"); err != nil {
		t.Fatalf("二次 SeedAdmin 不应报错: %v", err)
	}
	if err := db.Table("users").Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("二次 SeedAdmin 不应新增用户，实际 %d", count)
	}
}
