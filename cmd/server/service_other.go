//go:build !windows

package main

// runServiceIfNeeded 非 Windows 平台没有服务控制管理器，恒为控制台模式。
func runServiceIfNeeded() bool { return false }
