package agentapi

import (
	"github.com/go-chi/chi/v5"
)

// Mount 在给定路由上挂载 Agent API 契约（/agent/v1/*）。
func (h *Handler) Mount(r chi.Router) {
	r.Route("/agent/v1", func(r chi.Router) {
		r.Post("/enroll", h.handleEnroll)
		r.Post("/heartbeat", h.handleHeartbeat)
	})
}
