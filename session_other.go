//go:build !windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func acquireSupervisorLock(port int) (func(), bool) {
	return acquireLockFile("codex-chinese-supervisor.lock")
}

func acquireLaunchLock() (func(), bool) {
	return acquireLockFile("codex-chinese-launcher.lock")
}

func acquireLockFile(name string) (func(), bool) {
	path := filepath.Join(os.TempDir(), name)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return func() {}, false
	}
	fmt.Fprintf(f, "%d\n", os.Getpid())
	_ = f.Close()
	return func() {
		_ = os.Remove(path)
	}, true
}
