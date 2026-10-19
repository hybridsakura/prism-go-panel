package api

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/hybridsakura/prism-go-panel/internal/web"
)

// Routes 构建完整路由表。
func (s *Server) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer) // panic 不会打挂服务
	r.Use(middleware.RealIP)    // 信任 X-Forwarded-For / X-Real-IP
	r.Use(s.accessLog)          // 结构化访问日志
	r.Use(middleware.Timeout(30 * time.Second))

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
	})

	// ---------- RustDesk 客户端契约（/api） ----------
	r.Route("/api", func(r chi.Router) {
		r.Post("/login", s.handleLogin)
		r.Post("/logout", s.handleLogout)
		r.Post("/currentUser", s.handleCurrentUser)
		r.Get("/ab", s.handleABGet)
		r.Post("/ab", s.handleABPost)
		r.Post("/ab/get", s.handleABGetCompat)
		r.Post("/sysinfo", s.handleSysinfo)
		r.Post("/heartbeat", s.handleHeartbeat)
		r.Post("/audit", s.handleAudit)
		r.Post("/users", s.handleUsersStub)
		r.Post("/peers", s.handlePeersStub)
	})

	// ---------- 管理后台（/admin） ----------
	r.Route("/admin", func(r chi.Router) {
		// 无需会话的接口
		r.Post("/api/login", s.handleAdminLogin)
		r.Post("/api/logout", s.handleAdminLogout)
		r.Post("/api/register", s.handleAdminRegister)

		// 需要登录的接口
		r.Group(func(r chi.Router) {
			r.Use(s.requireSession)
			r.Get("/api/me", s.handleAdminMe)
			r.Get("/api/stats", s.handleStats)
			r.Get("/api/devices", s.handleDevices)
			r.Get("/api/devices/export", s.handleExportDevices)
			r.Get("/api/conn_logs", s.handleConnLogs)
			r.Get("/api/file_logs", s.handleFileLogs)
			r.Get("/api/shares", s.handleSharesList)
			r.Post("/api/shares", s.handleShareCreate)
			r.Post("/api/shares/{shash}/claim", s.handleShareClaim)

			// 仅管理员
			r.Group(func(r chi.Router) {
				r.Use(s.requireAdmin)
				r.Get("/api/users", s.handleUsersList)
				r.Post("/api/users", s.handleUserCreate)
				r.Delete("/api/users/{id}", s.handleUserDelete)
				r.Post("/api/users/{id}/password", s.handleUserResetPassword)

				// 服务端配置档案（对接 hbbs/hbbr 部署信息 + 客户端配置生成）
				r.Get("/api/server-profile", s.handleProfileGet)
				r.Put("/api/server-profile", s.handleProfilePut)
				r.Post("/api/server-profile/check", s.handleProfileCheck)
				r.Get("/api/server-profile/client-config", s.handleClientConfig)

				// ID 冲突（重复远程 ID）检测结果
				r.Get("/api/id-conflicts", s.handleIDConflicts)
				r.Delete("/api/id-conflicts/{rid}", s.handleIDConflictDelete)
			})
		})

		// 管理后台 SPA 静态资源
		web.MountAdmin(r)
	})

	// ---------- RustDesk Web 控制端（/webui） ----------
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
