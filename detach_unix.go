//go:build !windows

package main

import (
	"os/exec"
	"syscall"
)

// 新会话，避免父进程退出时子进程收到 SIGHUP。
func applyDetach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
