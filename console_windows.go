//go:build windows

package main

import "syscall"

var procSetConsoleOutputCP = syscall.NewLazyDLL("kernel32.dll").NewProc("SetConsoleOutputCP")

// 让 Windows 控制台以 UTF-8 显示中文，避免双击运行时乱码。
func init() {
	_, _, _ = procSetConsoleOutputCP.Call(65001)
}
