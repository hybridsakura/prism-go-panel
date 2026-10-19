// Command prism-go-panel 是 RustDesk API 服务器的 Go 实现，
// 参照 kingmo888/rustdesk-api-server（Python/Django）重写，客户端 API 契约保持一致。
//
// 提供服务：
//   - /api/*       RustDesk 客户端契约（登录、心跳、地址簿、资产上报、审计）
//   - /admin      内嵌管理后台（设备在线状态、资产统计、日志、分享、用户管理）
//   - /webui      RustDesk Web 控制端（可选，需提供 Flutter 编译产物目录）
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"gorm.io/gorm"

	"github.com/hybridsakura/prism-go-panel/internal/api"
	"github.com/hybridsakura/prism-go-panel/internal/auth"
	"github.com/hybridsakura/prism-go-panel/internal/config"
	"github.com/hybridsakura/prism-go-panel/internal/database"
	"github.com/hybridsakura/prism-go-panel/internal/stats"
)

func main() {
	// Windows 下若由服务控制管理器（SCM）启动，则进入服务托管模式
	if runServiceIfNeeded() {
		return
	}
	// 控制台模式：监听 SIGINT/SIGTERM 触发优雅退出
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		requestShutdown()
	}()
	os.Exit(serve())
}

var (
	shutdownCh   = make(chan struct{})
	shutdownOnce sync.Once
)

// requestShutdown 请求优雅退出（幂等，可来自信号或 Windows SCM）。
func requestShutdown() {
	shutdownOnce.Do(func() { close(shutdownCh) })
}

// serve 启动所有组件并阻塞，直到收到退出请求；返回进程退出码。
func serve() int {
	cfg := config.Load()
	setupLogging(cfg.LogLevel, cfg.LogFile)

	db, err := database.Open(cfg)
	if err != nil {
		slog.Error("数据库初始化失败", "err", err)
		return 1
	}
	if err := database.SeedAdmin(db, cfg.AdminUsername, cfg.AdminPassword); err != nil {
		slog.Error("初始管理员创建失败", "err", err)
		return 1
	}
	if cfg.AdminUsername != "" && cfg.AdminPassword != "" {
		slog.Info("已确保初始管理员存在", "username", cfg.AdminUsername)
	}

	tokens := auth.NewTokenStore(db, cfg.TokenTTL)
	sessions := auth.NewSessionStore(cfg.SessionTTL)
	tracker := stats.NewTracker(cfg.OnlineTimeout)
	srv := api.NewServer(cfg, db, tokens, sessions, tracker)

	httpSrv := &http.Server{
		Addr:              fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Handler:           srv.Routes(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// 后台维护任务：清理过期 token / 会话 / 在线表陈旧项，标记过期分享链接
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	go maintain(ctx, db, tokens, sessions, tracker)

	go func() {
		slog.Info("prism-go-panel 已启动",
			"addr", httpSrv.Addr,
			"db", cfg.DBURL,
			"admin", "http://"+httpSrv.Addr+"/admin")
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("服务退出", "err", err)
			requestShutdown()
		}
	}()

	<-shutdownCh
	slog.Info("正在关闭服务…")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		slog.Error("关闭服务失败", "err", err)
		return 1
	}
	slog.Info("服务已停止")
	return 0
}

// setupLogging 配置结构化日志，级别 debug/info/warn/error；
// logFile 非空时日志同时写入文件（Windows 服务模式无控制台，建议配置）。
func setupLogging(level, logFile string) {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	out := os.Stdout
	if logFile != "" {
		if f, err := os.OpenFile(logFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
			out = f
		} else {
			slog.Warn("日志文件打开失败，回退到标准输出", "file", logFile, "err", err)
		}
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(out, &slog.HandlerOptions{Level: lvl})))
}

// maintain 周期性维护任务，每 5 分钟执行一次：
// 清理过期 token、过期会话、在线表陈旧项，并标记过期的分享链接。
func maintain(ctx context.Context, db *gorm.DB, tokens *auth.TokenStore, sessions *auth.SessionStore, tracker *stats.Tracker) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := tokens.CleanupExpired(); err != nil {
				slog.Warn("清理过期 token 失败", "err", err)
			}
			sessions.Cleanup()
			if n := tracker.PruneExpired(); n > 0 {
				slog.Debug("清理在线表陈旧项", "count", n)
			}
			if err := database.MarkExpiredShareLinks(db); err != nil {
				slog.Warn("标记过期分享链接失败", "err", err)
			}
		}
	}
}
