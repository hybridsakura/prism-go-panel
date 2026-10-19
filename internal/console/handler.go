package console

import (
	"github.com/hybridsakura/prism-go-panel/internal/auth"
	"github.com/hybridsakura/prism-go-panel/internal/httpx"
	"github.com/hybridsakura/prism-go-panel/internal/platform"
)

// Handler 实现管理后台接口（/admin/api/*）。
// 登录态、权限判断属于后台自身关注点，留在本包；
// 设备/冲突等业务规则通过 platform.Core 复用。
type Handler struct {
	core     *platform.Core
	sessions *auth.SessionStore
	limiter  *httpx.LoginLimiter
}

// New 创建管理后台处理器。
func New(core *platform.Core, sessions *auth.SessionStore) *Handler {
	return &Handler{
		core:     core,
		sessions: sessions,
		limiter:  httpx.NewLoginLimiter(core.Cfg.LoginRateLimit),
	}
}
