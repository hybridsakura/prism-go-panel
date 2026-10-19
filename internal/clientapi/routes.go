package clientapi

import (
	"github.com/go-chi/chi/v5"
)

// Mount 在给定路由上挂载 RustDesk 客户端契约（/api/*）。
// 该契约与官方客户端二进制绑定，路径与行为不可随意变更。
func (s *Handler) Mount(r chi.Router) {
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
}
