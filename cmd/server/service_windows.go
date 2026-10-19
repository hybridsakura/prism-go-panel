//go:build windows

package main

import (
	"log/slog"

	"golang.org/x/sys/windows/svc"
)

// serviceName 是 Windows 服务管理器中显示的服务名。
const serviceName = "prism-go-panel"

// runServiceIfNeeded 检测进程是否由 Windows 服务控制管理器（SCM）启动；
// 若是则接管整个进程生命周期并返回 true，否则返回 false 进入控制台模式。
func runServiceIfNeeded() bool {
	isSvc, err := svc.IsWindowsService()
	if err != nil || !isSvc {
		return false
	}
	slog.Info("以 Windows 服务模式运行", "service", serviceName)
	if err := svc.Run(serviceName, &windowsService{}); err != nil {
		slog.Error("Windows 服务运行失败", "err", err)
	}
	return true
}

// windowsService 实现 svc.Handler：把 SCM 的 Stop/Shutdown 请求转成优雅退出。
type windowsService struct{}

func (w *windowsService) Execute(_ []string, r <-chan svc.ChangeRequest, s chan<- svc.Status) (bool, uint32) {
	s <- svc.Status{State: svc.StartPending}
	codeCh := make(chan int, 1)
	go func() { codeCh <- serve() }()
	s <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}

	for {
		select {
		case code := <-codeCh:
			// 服务自身退出（如监听端口失败）
			s <- svc.Status{State: svc.Stopped}
			return false, uint32(code)
		case c := <-r:
			switch c.Cmd {
			case svc.Interrogate:
				s <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				s <- svc.Status{State: svc.StopPending}
				requestShutdown()
				code := <-codeCh
				s <- svc.Status{State: svc.Stopped}
				return false, uint32(code)
			}
		}
	}
}
