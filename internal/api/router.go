// Package api 是组合根：把领域层与各传输层装配成完整的 HTTP 服务。
//
// 分工：
//   - platform   领域层（设备、在线状态、ID 冲突）
//   - clientapi  RustDesk 客户端契约 /api/*
//   - console    管理后台接口 /admin/api/*
//   - web        管理后台静态页面
//
// 传输层不直接互相依赖，新增入口（如未来的 agentapi）只需在这里装配。
package api

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"gorm.io/gorm"

	"github.com/hybridsakura/prism-go-panel/internal/auth"
	"github.com/hybridsakura/prism-go-panel/internal/clientapi"
	"github.com/hybridsakura/prism-go-panel/internal/config"
	"github.com/hybridsakura/prism-go-panel/internal/console"
	"github.com/hybridsakura/prism-go-panel/internal/httpx"
	"github.com/hybridsakura/prism-go-panel/internal/platform"
	"github.com/hybridsakura/prism-go-panel/internal/stats"
	"github.com/hybridsakura/prism-go-panel/internal/web"
)

// Server 聚合各传输层处理器。
type Server struct {
	cfg     *config.Config
	client  *clientapi.Handler
	console *console.Handler
}

// NewServer 创建 API 服务器（组合根）。
func NewServer(cfg *config.Config, db *gorm.DB, tokens *auth.TokenStore, sessions *auth.SessionStore, tracker *stats.Tracker) *Server {
	core := platform.New(cfg, db, tracker)
	return &Server{
		cfg:     cfg,
		client:  clientapi.New(core, tokens),
		console: console.New(core, sessions),
	}
}

// Routes 构建完整路由表。
func (s *Server) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer) // panic 不会打挂服务
	r.Use(middleware.RealIP)    // 信任 X-Forwarded-For / X-Real-IP
	r.Use(s.accessLog)          // 结构化访问日志
	r.Use(middleware.Timeout(30 * time.Second))

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		httpx.JSON(w, http.StatusOK, map[string]any{"status": "ok"})
	})

	// RustDesk 客户端契约（/api/*）
	s.client.Mount(r)

	// 管理后台接口（/admin/api/*）+ 静态页面（/admin/*）
	s.console.Mount(r)
	web.MountAdmin(r)

	// RustDesk Web 控制端（/webui，可选）
	web.MountWebClient(r, s.cfg)

	return r
}

// accessLog 结构化访问日志（slog）。
func (s *Server) accessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		slog.Debug("访问日志",
			"method", r.Method, "path", r.URL.Path,
			"status", ww.Status(), "duration_ms", time.Since(start).Milliseconds(),
			"remote", r.RemoteAddr)
	})
}
