// Package web 负责内嵌管理后台 SPA 与 RustDesk Web 控制端静态资源的托管。
package web

import (
	"embed"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/hybridsakura/prism-go-panel/internal/config"
)

//go:embed admin/index.html admin/style.css admin/app.js
var adminAssets embed.FS

// MountAdmin 在 /admin 路由组上挂载内嵌管理后台 SPA。
func MountAdmin(r chi.Router) {
	sub, err := fs.Sub(adminAssets, "admin")
	if err != nil {
		slog.Error("内嵌管理后台资源加载失败", "err", err)
		return
	}
	serve := func(name, contentType string, cache bool) http.HandlerFunc {
		return func(w http.ResponseWriter, _ *http.Request) {
			data, err := fs.ReadFile(sub, name)
			if err != nil {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", contentType)
			if cache {
				w.Header().Set("Cache-Control", "public, max-age=3600")
			} else {
				w.Header().Set("Cache-Control", "no-cache")
			}
			_, _ = w.Write(data)
		}
	}
	r.Route("/admin", func(r chi.Router) {
		r.Get("/", serve("index.html", "text/html; charset=utf-8", false))
		// 管理 SPA 资源体积小且随二进制内嵌发布，统一 no-cache 强制重校验，
		// 避免旧版浏览器缓存导致前端与后端版本错位
		r.Get("/style.css", serve("style.css", "text/css; charset=utf-8", false))
		r.Get("/app.js", serve("app.js", "text/javascript; charset=utf-8", false))
		// SPA 兜底：其余路径一律返回 index.html（前端 hash 路由）
		r.Get("/*", serve("index.html", "text/html; charset=utf-8", false))
	})
}

//go:embed webui.html
var webuiTemplate string

// MountWebClient 挂载 RustDesk Web 控制端（Flutter 编译产物，目录为 cfg.WebClientDir）。
// 目录结构需与 Python 参考版一致：<dir>/{module,js,assets,canvaskit@*,ogvjs-*}。
func MountWebClient(r chi.Router, cfg *config.Config) {
	dir := cfg.WebClientDir
	if dir == "" {
		return
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		slog.Warn("Web 控制端目录不存在，/webui 未启用", "dir", dir)
		return
	}

	entryJS := findModuleEntry(dir)
	canvasKit := findCanvasKitDir(dir)

	static := http.StripPrefix("/webui", http.FileServer(http.Dir(dir)))
	noDir := func(h http.Handler) http.HandlerFunc {
		return func(w http.ResponseWriter, req *http.Request) {
			if strings.HasSuffix(req.URL.Path, "/") {
				http.NotFound(w, req)
				return
			}
			h.ServeHTTP(w, req)
		}
	}

	index := func(w http.ResponseWriter, req *http.Request) {
		ids := cfg.IDServers
		if len(ids) == 0 {
			// 未配置 ID_SERVER 时回退为请求主机（客户端与 API 同址部署的常见形态）
			ids = []string{req.Host}
		}
		page := strings.ReplaceAll(webuiTemplate, "{{API_SERVERS}}", formatJSArray(ids))
		page = strings.ReplaceAll(page, "{{ENTRY_JS}}", entryJS)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write([]byte(page))
	}

	r.Get("/webui", index)
	r.Get("/webui/", index)
	r.Get("/webui/*", noDir(static))

	// Flutter 构建会把 canvaskit 绝对路径写进页面，保留根级路由兼容
	if canvasKit != "" {
		kitStatic := http.StripPrefix("/"+canvasKit, http.FileServer(http.Dir(filepath.Join(dir, canvasKit))))
		r.Get("/"+canvasKit+"/*", noDir(kitStatic))
	}
	slog.Info("Web 控制端已挂载", "dir", dir, "entry", entryJS)
}

// findModuleEntry 探测 Flutter module 目录下带 hash 的入口 JS。
func findModuleEntry(dir string) string {
	matches, _ := filepath.Glob(filepath.Join(dir, "module", "index.*.js"))
	if len(matches) == 0 {
		return "index.js"
	}
	return filepath.Base(matches[0])
}

// findCanvasKitDir 探测 canvaskit@* 目录名。
func findCanvasKitDir(dir string) string {
	matches, _ := filepath.Glob(filepath.Join(dir, "canvaskit@*"))
	if len(matches) == 0 {
		return ""
	}
	return filepath.Base(matches[0])
}

// formatJSArray 生成 JS 字符串数组字面量。
func formatJSArray(items []string) string {
	quoted := make([]string, 0, len(items))
	for _, s := range items {
		s = strings.ReplaceAll(s, `"`, "")
		quoted = append(quoted, `"`+s+`"`)
	}
	return strings.Join(quoted, ",")
}
