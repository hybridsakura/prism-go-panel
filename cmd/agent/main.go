package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
)

const version = "0.1.0"

var (
	cancelGlobal context.CancelFunc
	httpClient   = &http.Client{Timeout: 15 * time.Second}
)

type stateData struct {
	AgentID     uint   `json:"agent_id"`
	DeviceToken string `json:"device_token"`
	ServerURL   string `json:"server_url"`
}

func main() {
	serverFlag := flag.String("server", "", "PRISM 服务端 URL (如 http://172.18.11.236:21114)")
	tokenFlag := flag.String("token", "", "一次性注册码 (Enroll Token)")
	dataDirFlag := flag.String("data-dir", "", "数据与配置目录")
	flag.Parse()

	if runServiceIfNeeded(func() int {
		return runAgent(*serverFlag, *tokenFlag, *dataDirFlag)
	}) {
		return
	}

	os.Exit(runAgent(*serverFlag, *tokenFlag, *dataDirFlag))
}

func stopAgent() {
	if cancelGlobal != nil {
		cancelGlobal()
	}
}

func runAgent(serverURL, enrollToken, dataDir string) int {
	ctx, cancel := context.WithCancel(context.Background())
	cancelGlobal = cancel
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		slog.Info("收到终止信号，准备退出 Agent")
		cancel()
	}()

	dir := resolveDataDir(dataDir)
	_ = os.MkdirAll(dir, 0755)
	stateFile := filepath.Join(dir, "state.json")

	state, err := loadState(stateFile)
	if err != nil {
		state = &stateData{}
	}

	// 环境变量优先兜底
	if serverURL == "" {
		serverURL = os.Getenv("PRISM_SERVER")
	}
	if serverURL == "" && state.ServerURL != "" {
		serverURL = state.ServerURL
	}
	if serverURL == "" {
		serverURL = "http://127.0.0.1:21114"
	}
	serverURL = strings.TrimRight(serverURL, "/")

	if enrollToken == "" {
		enrollToken = os.Getenv("PRISM_ENROLL_TOKEN")
	}

	hostname, _ := os.Hostname()
	rid := detectRustDeskID()
	fp := machineFingerprint(hostname)

	// 1. 若无设备令牌，执行注册换发
	if state.DeviceToken == "" {
		if enrollToken == "" {
			slog.Error("缺少注册码，无法连接服务端。请使用 --token 启动或设置 PRISM_ENROLL_TOKEN")
			return 1
		}

		slog.Info("正在向服务端注册新终端...", "server", serverURL, "hostname", hostname, "rid", rid)
		resp, err := enroll(ctx, serverURL, enrollToken, hostname, rid, fp)
		if err != nil {
			slog.Error("注册失败", "err", err)
			return 2
		}

		state.AgentID = resp.AgentID
		state.DeviceToken = resp.DeviceToken
		state.ServerURL = serverURL
		if err := saveState(stateFile, state); err != nil {
			slog.Warn("保存 state.json 失败", "err", err)
		}
		slog.Info("终端注册成功！", "agent_id", state.AgentID)
	} else {
		slog.Info("使用已保存的终端凭证启动", "agent_id", state.AgentID, "server", serverURL)
	}

	// 2. 心跳主循环（10s ± 2s 抖动）
	slog.Info("心跳服务已启动 (10s 轮询)")
	for {
		err := heartbeat(ctx, serverURL, state.DeviceToken, hostname, rid)
		if err != nil {
			slog.Warn("心跳失败", "err", err)
		} else {
			slog.Debug("心跳成功", "time", time.Now().Format("15:04:05"))
		}

		// 抖动：8s ~ 12s
		jitter := time.Duration(8000+rand.Intn(4000)) * time.Millisecond
		select {
		case <-ctx.Done():
			slog.Info("心跳服务正常关闭")
			return 0
		case <-time.After(jitter):
		}
	}
}

type enrollResponse struct {
	AgentID     uint   `json:"agent_id"`
	DeviceToken string `json:"device_token"`
}

func enroll(ctx context.Context, serverURL, enrollToken, hostname, rid, fp string) (*enrollResponse, error) {
	reqBody, _ := json.Marshal(map[string]any{
		"enroll_token": enrollToken,
		"hostname":     hostname,
		"os":           runtime.GOOS,
		"arch":         runtime.GOARCH,
		"version":      version,
		"rid":          rid,
		"fingerprint":  fp,
	})

	req, err := http.NewRequestWithContext(ctx, "POST", serverURL+"/agent/v1/enroll", bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	var res enrollResponse
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, err
	}
	if res.DeviceToken == "" || res.AgentID == 0 {
		return nil, fmt.Errorf("服务端返回凭证不完整: %s", string(body))
	}
	return &res, nil
}

func heartbeat(ctx context.Context, serverURL, deviceToken, hostname, rid string) error {
	reqBody, _ := json.Marshal(map[string]any{
		"version":  version,
		"hostname": hostname,
		"rid":      rid,
	})

	req, err := http.NewRequestWithContext(ctx, "POST", serverURL+"/agent/v1/heartbeat", bytes.NewReader(reqBody))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+deviceToken)

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("终端凭证已被吊销或终端被禁用 (401)")
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

func machineFingerprint(hostname string) string {
	mac := getFirstMAC()
	h := sha256.New()
	h.Write([]byte(hostname + "|" + mac))
	return hex.EncodeToString(h.Sum(nil))[:32]
}

func getFirstMAC() string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, i := range interfaces {
		if i.Flags&net.FlagUp != 0 && !bytes.Equal(i.HardwareAddr, nil) {
			return i.HardwareAddr.String()
		}
	}
	return ""
}

func resolveDataDir(explicit string) string {
	if explicit != "" {
		return explicit
	}
	if runtime.GOOS == "windows" {
		if pd := os.Getenv("ProgramData"); pd != "" {
			return filepath.Join(pd, "Prism Agent")
		}
	}
	return "."
}

func loadState(path string) (*stateData, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s stateData
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func saveState(path string, s *stateData) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}
