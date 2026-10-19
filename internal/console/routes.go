package console

import (
	"github.com/go-chi/chi/v5"
)

// Mount 在给定路由上挂载管理后台接口（/admin/api/*）。
// 静态页面由 internal/web 负责，二者共同构成 /admin。
func (s *Handler) Mount(r chi.Router) {
	r.Route("/admin/api", func(r chi.Router) {
		// 无需会话
		r.Post("/login", s.handleAdminLogin)
		r.Post("/logout", s.handleAdminLogout)
		r.Post("/register", s.handleAdminRegister)

		// 需要登录
		r.Group(func(r chi.Router) {
			r.Use(s.requireSession)
			r.Get("/me", s.handleAdminMe)
			r.Get("/stats", s.handleStats)
			r.Get("/devices", s.handleDevices)
			r.Get("/devices/export", s.handleExportDevices)
			r.Get("/conn_logs", s.handleConnLogs)
			r.Get("/file_logs", s.handleFileLogs)
			r.Get("/shares", s.handleSharesList)
			r.Post("/shares", s.handleShareCreate)
			r.Post("/shares/{shash}/claim", s.handleShareClaim)

			// 仅管理员
			r.Group(func(r chi.Router) {
				r.Use(s.requireAdmin)
				r.Get("/users", s.handleUsersList)
				r.Post("/users", s.handleUserCreate)
				r.Delete("/users/{id}", s.handleUserDelete)
				r.Post("/users/{id}/password", s.handleUserResetPassword)

				// 服务端配置档案
				r.Get("/server-profile", s.handleProfileGet)
				r.Put("/server-profile", s.handleProfilePut)
				r.Post("/server-profile/check", s.handleProfileCheck)
				r.Get("/server-profile/client-config", s.handleClientConfig)

				// ID 冲突
				r.Get("/id-conflicts", s.handleIDConflicts)
				r.Delete("/id-conflicts/{rid}", s.handleIDConflictDelete)
			})
		})
	})
}
