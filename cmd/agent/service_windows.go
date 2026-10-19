//go:build windows

package main

import (
	"log/slog"

	"golang.org/x/sys/windows/svc"
)

// serviceName 是 Windows 服务管理器中的服务名。
const serviceName = "PrismAgent"

func runServiceIfNeeded(runFn func() int) bool {
	isSvc, err := svc.IsWindowsService()
	if err != nil || !isSvc {
		return false
	}
	slog.Info("以 Windows 服务模式运行", "service", serviceName)
	h := &agentService{runFn: runFn}
	if err := svc.Run(serviceName, h); err != nil {
		slog.Error("Windows 服务运行失败", "err", err)
	}
	return true
}

type agentService struct {
	runFn func() int
}

func (s *agentService) Execute(_ []string, r <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	status <- svc.Status{State: svc.StartPending}
	codeCh := make(chan int, 1)
	go func() { codeCh <- s.runFn() }()
	status <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}

	for {
		select {
		case code := <-codeCh:
			status <- svc.Status{State: svc.Stopped}
			return false, uint32(code)
		case req := <-r:
			switch req.Cmd {
			case svc.Interrogate:
				status <- req.CurrentStatus
			case svc.Stop, svc.Shutdown:
				status <- svc.Status{State: svc.StopPending}
				stopAgent()
				status <- svc.Status{State: svc.Stopped}
				return false, 0
			}
		}
	}
}
