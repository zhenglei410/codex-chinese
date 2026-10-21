//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

var (
	sessionKernel32        = syscall.NewLazyDLL("kernel32.dll")
	procCreateMutexW       = sessionKernel32.NewProc("CreateMutexW")
	procReleaseMutex       = sessionKernel32.NewProc("ReleaseMutex")
	procSessionCloseHandle = sessionKernel32.NewProc("CloseHandle")
)

func acquireSupervisorLock(port int) (func(), bool) {
	return acquireNamedMutex(`Local\CodexChineseSupervisor`)
}

func acquireLaunchLock() (func(), bool) {
	return acquireNamedMutex(`Local\CodexChineseLauncher`)
}

func acquireNamedMutex(name string) (func(), bool) {
	mutexName, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return func() {}, false
	}
	handle, _, callErr := procCreateMutexW.Call(0, 0, uintptr(unsafe.Pointer(mutexName)))
	if handle == 0 {
		return func() {}, false
	}
	if callErr == syscall.Errno(183) { // ERROR_ALREADY_EXISTS
		procSessionCloseHandle.Call(handle)
		return func() {}, false
	}
	return func() {
		procReleaseMutex.Call(handle)
		procSessionCloseHandle.Call(handle)
	}, true
}
