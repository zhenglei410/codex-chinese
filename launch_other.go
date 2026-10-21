//go:build !windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

func launchCodex(exe string, port int) error {
	debugURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	cmd := exec.Command(
		exe,
		fmt.Sprintf("--remote-debugging-port=%d", port),
		"--remote-allow-origins="+debugURL,
	)
	cmd.Stdout, cmd.Stderr = nil, nil
	applyDetach(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

func codexProcessIDs(exe string) []uint32 {
	return nil
}

func openCodexFallback(exe string) error {
	cmd := exec.Command(exe)
	cmd.Stdout, cmd.Stderr = nil, nil
	applyDetach(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

func terminateCodexProcesses(exe string) ([]uint32, error) {
	return nil, nil
}

func processAlive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return p.Signal(syscall.Signal(0)) == nil
}
