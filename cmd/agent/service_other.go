//go:build !windows

package main

func runServiceIfNeeded(_ func() int) bool {
	return false
}
