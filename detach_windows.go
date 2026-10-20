//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

// 让 Codex 在我们的程序退出后继续运行：脱离控制台、独立进程组。
func applyDetach(cmd *exec.Cmd) {
	const (
		detachedProcess       = 0x00000008
		createNewProcessGroup = 0x00000200
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: detachedProcess | createNewProcessGroup,
	}
}
