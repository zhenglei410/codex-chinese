//go:build windows

// codex-chinese 托盘版（Windows）。
// 纯 Win32（user32/shell32），无 cgo、无第三方依赖。
// 托盘菜单：立即强制中文 / 环境诊断 / 退出。
// 实际注入由同目录的 codex-chinese.exe 完成（避免重复实现 CDP）。
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"unsafe"
)

// version 由 -ldflags "-X main.version=..." 注入（与 CLI 共用构建参数）。
var version = "dev"

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	procRegisterClassExW   = user32.NewProc("RegisterClassExW")
	procCreateWindowExW    = user32.NewProc("CreateWindowExW")
	procDefWindowProcW     = user32.NewProc("DefWindowProcW")
	procGetMessageW        = user32.NewProc("GetMessageW")
	procTranslateMessage   = user32.NewProc("TranslateMessage")
	procDispatchMessageW   = user32.NewProc("DispatchMessageW")
	procPostQuitMessage    = user32.NewProc("PostQuitMessage")
	procDestroyWindow      = user32.NewProc("DestroyWindow")
	procCreatePopupMenu    = user32.NewProc("CreatePopupMenu")
	procAppendMenuW        = user32.NewProc("AppendMenuW")
	procTrackPopupMenu     = user32.NewProc("TrackPopupMenu")
	procDestroyMenu        = user32.NewProc("DestroyMenu")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
	procGetCursorPos       = user32.NewProc("GetCursorPos")
	procLoadIconW          = user32.NewProc("LoadIconW")
	procLoadCursorW        = user32.NewProc("LoadCursorW")

	procShellNotifyIconW = shell32.NewProc("Shell_NotifyIconW")
	procGetModuleHandleW = kernel32.NewProc("GetModuleHandleW")
)

const (
	wmDestroy     = 0x0002
	wmCommand     = 0x0111
	wmLButtonUp   = 0x0202
	wmRButtonUp   = 0x0205
	wmLButtonDbl  = 0x0203
	wmContextMenu = 0x007B
	wmApp         = 0x8000
	trayCallback  = wmApp + 1

	nimAdd    = 0
	nimModify = 1
	nimDelete = 2

	nifMessage = 0x00000001
	nifIcon    = 0x00000002
	nifTip     = 0x00000004
	nifInfo    = 0x00000010

	niifInfo = 0x00000001

	mfString    = 0x00000000
	mfSeparator = 0x00000800

	tpmRightButton = 0x0002
	tpmReturnCmd   = 0x0100
	tpmNoNotify    = 0x0080

	idiApplication = 32512
	idcArrow       = 32512

	createNoWindow = 0x08000000
)

type point struct{ X, Y int32 }

type msgStruct struct {
	hwnd     uintptr
	message  uint32
	wParam   uintptr
	lParam   uintptr
	time     uint32
	pt       point
	lPrivate uint32
}

type wndClassExW struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     uintptr
	hIcon         uintptr
	hCursor       uintptr
	hbrBackground uintptr
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       uintptr
}

type guid struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

type notifyIconDataW struct {
	cbSize            uint32
	hWnd              uintptr
	uID               uint32
	uFlags            uint32
	uCallbackMessage  uint32
	hIcon             uintptr
	szTip             [128]uint16
	dwState           uint32
	dwStateMask       uint32
	szInfo            [256]uint16
	uTimeoutOrVersion uint32
	szInfoTitle       [64]uint16
	dwInfoFlags       uint32
	guidItem          guid
	hBalloonIcon      uintptr
}

var (
	gNID      notifyIconDataW
	gMenu     uintptr
	gHWND     uintptr
	gCallback uintptr
)

func copyUTF16(dst []uint16, s string) {
	src := syscall.StringToUTF16(s)
	n := len(src) - 1
	if n > len(dst)-1 {
		n = len(dst) - 1
	}
	copy(dst[:n], src[:n])
	dst[n] = 0
}

// cliPath 找同目录的 codex-chinese.exe，找不到再尝试 dist 目录（开发时）。
func cliPath() string {
	exe, err := os.Executable()
	if err == nil {
		p := filepath.Join(filepath.Dir(exe), "codex-chinese.exe")
		if _, e := os.Stat(p); e == nil {
			return p
		}
	}
	candidates := []string{
		"codex-chinese.exe",
		filepath.Join("dist", "codex-chinese-windows-amd64.exe"),
	}
	for _, c := range candidates {
		if _, e := os.Stat(c); e == nil {
			abs, _ := filepath.Abs(c)
			return abs
		}
	}
	return ""
}

func runCLI(args ...string) (string, error) {
	cli := cliPath()
	if cli == "" {
		return "", fmt.Errorf("未找到 codex-chinese.exe，请与托盘程序放在同一目录")
	}
	cmd := exec.Command(cli, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func balloon(title, text string) {
	gNID.uFlags = nifInfo
	copyUTF16(gNID.szInfoTitle[:], title)
	copyUTF16(gNID.szInfo[:], text)
	gNID.dwInfoFlags = niifInfo
	procShellNotifyIconW.Call(nimModify, uintptr(unsafe.Pointer(&gNID)))
}

func setTip(text string) {
	gNID.uFlags = nifTip
	copyUTF16(gNID.szTip[:], text)
	procShellNotifyIconW.Call(nimModify, uintptr(unsafe.Pointer(&gNID)))
}

func doForce() {
	setTip("codex-chinese：正在强制中文…")
	out, err := runCLI()
	if err != nil {
		msg := out
		if msg == "" {
			msg = err.Error()
		}
		balloon("强制中文失败", lastLine(msg))
		setTip("codex-chinese：失败")
		return
	}
	balloon("已强制中文", "Codex 已切换为中文界面")
	setTip("codex-chinese：中文已启用")
}

func doDoctor() {
	out, err := runCLI("-doctor")
	if err != nil && out == "" {
		balloon("诊断失败", err.Error())
		return
	}
	balloon("环境诊断", lastLine(out))
}

func lastLine(s string) string {
	// 取最后一行非空文本，适合放进气泡提示
	line := ""
	cur := ""
	for _, r := range s {
		if r == '\n' || r == '\r' {
			if cur != "" {
				line = cur
			}
			cur = ""
			continue
		}
		cur += string(r)
	}
	if cur != "" {
		line = cur
	}
	if len(line) > 120 {
		line = line[:120]
	}
	return line
}

func showMenu(hwnd uintptr) {
	var pt point
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	procSetForegroundWindow.Call(hwnd)
	cmd, _, _ := procTrackPopupMenu.Call(
		gMenu,
		tpmRightButton|tpmReturnCmd|tpmNoNotify,
		uintptr(pt.X), uintptr(pt.Y), 0, hwnd, 0,
	)
	switch cmd {
	case 1:
		go doForce()
	case 2:
		go doDoctor()
	case 3:
		quit()
	}
}

func quit() {
	gNID.uFlags = 0
	procShellNotifyIconW.Call(nimDelete, uintptr(unsafe.Pointer(&gNID)))
	procDestroyMenu.Call(gMenu)
	procDestroyWindow.Call(gHWND)
	procPostQuitMessage.Call(0)
}

func windowProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	switch msg {
	case wmDestroy:
		procPostQuitMessage.Call(0)
		return 0
	case trayCallback:
		mouseMsg := lParam & 0xFFFF
		switch mouseMsg {
		case wmRButtonUp, wmContextMenu:
			showMenu(hwnd)
		case wmLButtonDbl:
			go doForce()
		}
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, msg, wParam, lParam)
	return r
}

func main() {
	hInstance, _, _ := procGetModuleHandleW.Call(0)

	className, _ := syscall.UTF16PtrFromString("CodexChineseTrayWindow")
	windowName, _ := syscall.UTF16PtrFromString("codex-chinese")

	gCallback = syscall.NewCallback(windowProc)

	wc := wndClassExW{
		style:         0,
		lpfnWndProc:   gCallback,
		hInstance:     hInstance,
		hCursor:       0,
		lpszClassName: className,
	}
	wc.cbSize = uint32(unsafe.Sizeof(wc))
	hCursor, _, _ := procLoadCursorW.Call(0, idcArrow)
	wc.hCursor = hCursor

	if atom, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); atom == 0 {
		panic("RegisterClassExW failed: " + err.Error())
	}

	hwnd, _, err := procCreateWindowExW.Call(
		0, uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(windowName)),
		0, 0, 0, 0, 0, 0, 0, hInstance, 0,
	)
	if hwnd == 0 {
		panic("CreateWindowExW failed: " + err.Error())
	}
	gHWND = hwnd

	hIcon, _, _ := procLoadIconW.Call(0, idiApplication)

	// 托盘图标
	gNID = notifyIconDataW{
		hWnd:             hwnd,
		uID:              1,
		uFlags:           nifMessage | nifIcon | nifTip,
		uCallbackMessage: trayCallback,
		hIcon:            hIcon,
	}
	gNID.cbSize = uint32(unsafe.Sizeof(gNID))
	copyUTF16(gNID.szTip[:], "codex-chinese：强制 Codex 中文")
	procShellNotifyIconW.Call(nimAdd, uintptr(unsafe.Pointer(&gNID)))

	// 右键菜单
	gMenu, _, _ = procCreatePopupMenu.Call()
	itemForce, _ := syscall.UTF16PtrFromString("立即强制中文")
	itemDoctor, _ := syscall.UTF16PtrFromString("环境诊断")
	itemExit, _ := syscall.UTF16PtrFromString("退出")
	procAppendMenuW.Call(gMenu, mfString, 1, uintptr(unsafe.Pointer(itemForce)))
	procAppendMenuW.Call(gMenu, mfString, 2, uintptr(unsafe.Pointer(itemDoctor)))
	procAppendMenuW.Call(gMenu, mfSeparator, 0, 0)
	procAppendMenuW.Call(gMenu, mfString, 3, uintptr(unsafe.Pointer(itemExit)))

	balloon("codex-chinese 已启动", "右键托盘图标可强制中文或诊断")

	// 消息循环
	var m msgStruct
	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}
