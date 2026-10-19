package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// detectRustDeskID 尝试从本机常见路径探查已配置的 RustDesk ID。
func detectRustDeskID() string {
	candidates := []string{
		// 1. 服务模式路径（LocalService）
		`C:\Windows\ServiceProfiles\LocalService\AppData\Roaming\RustDesk\config\RustDesk.toml`,
	}

	// 2. 当前用户 AppData
	if appData := os.Getenv("APPDATA"); appData != "" {
		candidates = append(candidates, filepath.Join(appData, "RustDesk", "config", "RustDesk.toml"))
	}

	for _, path := range candidates {
		if id := parseIDFromToml(path); id != "" {
			return id
		}
	}
	return ""
}

// parseIDFromToml 简单按行查找 id = "xxx"
func parseIDFromToml(filePath string) string {
	f, err := os.Open(filePath)
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "id") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 && strings.TrimSpace(parts[0]) == "id" {
				val := strings.TrimSpace(parts[1])
				val = strings.Trim(val, `"' `)
				if val != "" {
					return val
				}
			}
		}
	}
	return ""
}
