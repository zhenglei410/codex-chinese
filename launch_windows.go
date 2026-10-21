//go:build windows

package main

import (
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"unsafe"
)

type windowsGUID struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

var (
	ole32    = syscall.NewLazyDLL("ole32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	procCoInitializeEx   = ole32.NewProc("CoInitializeEx")
	procCoUninitialize   = ole32.NewProc("CoUninitialize")
	procCoCreateInstance = ole32.NewProc("CoCreateInstance")

	procCreateToolhelp32Snapshot   = kernel32.NewProc("CreateToolhelp32Snapshot")
	procProcess32FirstW            = kernel32.NewProc("Process32FirstW")
	procProcess32NextW             = kernel32.NewProc("Process32NextW")
	procOpenProcess                = kernel32.NewProc("OpenProcess")
	procQueryFullProcessImageNameW = kernel32.NewProc("QueryFullProcessImageNameW")
	procGetExitCodeProcess         = kernel32.NewProc("GetExitCodeProcess")
	procTerminateProcess           = kernel32.NewProc("TerminateProcess")
	procCloseHandle                = kernel32.NewProc("CloseHandle")
)

var (
	clsidApplicationActivationManager = windowsGUID{
		Data1: 0x45BA127D,
		Data2: 0x10A8,
		Data3: 0x46EA,
		Data4: [8]byte{0x8A, 0xB7, 0x56, 0xEA, 0x90, 0x78, 0x94, 0x3C},
	}
	iidApplicationActivationManager = windowsGUID{
		Data1: 0x2E941141,
		Data2: 0x7F97,
		Data3: 0x4756,
		Data4: [8]byte{0xBA, 0x1D, 0x9D, 0xEC, 0xDE, 0x89, 0x4A, 0x3D},
	}
)

const (
	coinitApartmentThreaded = 0x2
	clsctxAll               = 0x17
	rpcEChangedMode         = 0x80010106

	th32csSnapProcess       = 0x00000002
	invalidHandleValue      = ^uintptr(0)
	processTerminate        = 0x0001
	processQueryLimitedInfo = 0x1000
)

// launchCodex uses the native ApplicationActivationManager for MSIX builds.
// A packaged executable cannot be started correctly with CreateProcess: Windows
// denies direct execution and a process started that way has no package identity.
func launchCodex(exe string, port int) error {
	debugURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	debugArg := fmt.Sprintf("--remote-debugging-port=%d", port)
	if !isStoreCodex(exe) && !isStoreAlias(exe) {
		return startDetached(exe, debugArg, "--remote-allow-origins="+debugURL)
	}

	pkg, ok := storePackageForExecutable(exe)
	if !ok {
		return fmt.Errorf("检测到商店版 Codex，但无法从 MSIX 注册表解析 AUMID")
	}
	aumid := pkg.FamilyName + "!" + pkg.AppID
	if _, err := activatePackagedApp(aumid, debugArg+" --remote-allow-origins="+debugURL); err != nil {
		// Restore the packaged app even when the debug activation fails. This
		// fallback cannot inject Chinese, but it prevents leaving Codex closed.
		_ = startDetached("explorer.exe", `shell:AppsFolder\`+aumid)
		return fmt.Errorf("AUMID 激活失败 (%s): %w", aumid, err)
	}
	return nil
}

func openCodexFallback(exe string) error {
	if !isStoreCodex(exe) && !isStoreAlias(exe) {
		return startDetached(exe)
	}
	pkg, ok := storePackageForExecutable(exe)
	if !ok {
		return fmt.Errorf("无法解析 Codex AUMID")
	}
	return startDetached("explorer.exe", `shell:AppsFolder\`+pkg.FamilyName+"!"+pkg.AppID)
}

type appxApplication struct {
	ID         string `xml:"Id,attr"`
	Executable string `xml:"Executable,attr"`
}

type appxManifest struct {
	Applications []appxApplication `xml:"Applications>Application"`
}

type unknownVTable struct {
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr
}

type applicationActivationManagerVTable struct {
	unknownVTable
	ActivateApplication uintptr
}

type storePackage struct {
	FullName   string
	FamilyName string
	Root       string
	AppID      string
}

func storePackageForExecutable(exe string) (storePackage, bool) {
	cleanExe := filepath.Clean(strings.Trim(exe, `"`))
	packages := storePackagesFromRegistry()
	for _, p := range packages {
		for _, rel := range []string{`app\ChatGPT.exe`, `app\Codex.exe`, `app\codex.exe`, `ChatGPT.exe`, `Codex.exe`} {
			candidate := filepath.Join(p.Root, rel)
			if strings.EqualFold(filepath.Clean(candidate), cleanExe) {
				p.AppID = storeApplicationID(p.Root, filepath.Base(candidate))
				return p, true
			}
		}
	}
	// Application execution aliases live outside the package root. There is only
	// one registered Codex package in normal use; choose the newest registration.
	if isStoreAlias(exe) && len(packages) > 0 {
		p := packages[0]
		p.AppID = storeApplicationID(p.Root, "ChatGPT.exe")
		return p, true
	}
	return storePackage{}, false
}

func storePackagesFromRegistry() []storePackage {
	key, err := syscall.UTF16PtrFromString(appModelPackagesKey)
	if err != nil {
		return nil
	}
	var hKey syscall.Handle
	if err := syscall.RegOpenKeyEx(syscall.HKEY_CURRENT_USER, key, 0, syscall.KEY_READ, &hKey); err != nil {
		return nil
	}
	defer syscall.RegCloseKey(hKey)

	var packages []storePackage
	for i := uint32(0); ; i++ {
		nameBuf := make([]uint16, 512)
		nameLen := uint32(len(nameBuf))
		if err := syscall.RegEnumKeyEx(hKey, i, &nameBuf[0], &nameLen, nil, nil, nil, nil); err != nil {
			break
		}
		name := syscall.UTF16ToString(nameBuf[:nameLen])
		if !strings.HasPrefix(strings.ToLower(name), "openai.codex_") {
			continue
		}
		root := regQueryString(hKey, name, "PackageRootFolder")
		if root == "" {
			continue
		}
		packages = append(packages, storePackage{
			FullName:   name,
			FamilyName: packageFamilyName(name),
			Root:       root,
		})
	}
	// The package full name contains the version and sorts newest-first.
	for i := 1; i < len(packages); i++ {
		for j := i; j > 0 && packages[j].FullName > packages[j-1].FullName; j-- {
			packages[j], packages[j-1] = packages[j-1], packages[j]
		}
	}
	return packages
}

// PackageFullName:
//
//	OpenAI.Codex_26.1013.4894.0_x64__2p2nqsd0c76g0
//
// PackageFamilyName:
//
//	OpenAI.Codex_2p2nqsd0c76g0
func packageFamilyName(fullName string) string {
	parts := strings.Split(fullName, "_")
	if len(parts) < 2 {
		return fullName
	}
	return parts[0] + "_" + parts[len(parts)-1]
}

func storeApplicationID(root, executable string) string {
	data, err := os.ReadFile(filepath.Join(root, "AppxManifest.xml"))
	if err == nil {
		var manifest appxManifest
		if xml.Unmarshal(data, &manifest) == nil {
			want := strings.ToLower(strings.ReplaceAll(executable, `\`, `/`))
			for _, app := range manifest.Applications {
				got := strings.ToLower(strings.ReplaceAll(app.Executable, `\`, `/`))
				if got == want && app.ID != "" {
					return app.ID
				}
			}
			for _, app := range manifest.Applications {
				if app.ID != "" {
					return app.ID
				}
			}
		}
	}
	return "App"
}

func activatePackagedApp(aumid, arguments string) (uint32, error) {
	hr, _, _ := procCoInitializeEx.Call(0, coinitApartmentThreaded)
	code := int32(uint32(hr))
	shouldUninitialize := code >= 0
	if code < 0 && uint32(code) != rpcEChangedMode {
		return 0, fmt.Errorf("CoInitializeEx HRESULT=0x%08x", uint32(code))
	}
	if shouldUninitialize {
		defer procCoUninitialize.Call()
	}

	var manager unsafe.Pointer
	hr, _, _ = procCoCreateInstance.Call(
		uintptr(unsafe.Pointer(&clsidApplicationActivationManager)),
		0,
		clsctxAll,
		uintptr(unsafe.Pointer(&iidApplicationActivationManager)),
		uintptr(unsafe.Pointer(&manager)),
	)
	if code = int32(uint32(hr)); code < 0 || manager == nil {
		return 0, fmt.Errorf("CoCreateInstance HRESULT=0x%08x", uint32(code))
	}
	defer releaseCOM(manager)

	appID, err := syscall.UTF16PtrFromString(aumid)
	if err != nil {
		return 0, err
	}
	args, err := syscall.UTF16PtrFromString(arguments)
	if err != nil {
		return 0, err
	}
	var pid uint32
	activate := (*(**applicationActivationManagerVTable)(manager)).ActivateApplication
	hr, _, _ = syscall.SyscallN(
		activate,
		uintptr(manager),
		uintptr(unsafe.Pointer(appID)),
		uintptr(unsafe.Pointer(args)),
		0,
		uintptr(unsafe.Pointer(&pid)),
	)
	runtime.KeepAlive(aumid)
	runtime.KeepAlive(arguments)
	if code = int32(uint32(hr)); code < 0 {
		return 0, fmt.Errorf("ActivateApplication HRESULT=0x%08x", uint32(code))
	}
	return pid, nil
}

func releaseCOM(obj unsafe.Pointer) {
	if obj != nil {
		release := (*(**unknownVTable)(obj)).Release
		syscall.SyscallN(release, uintptr(obj))
	}
}

func startDetached(exe string, args ...string) error {
	cmd := exec.Command(exe, args...)
	cmd.Stdout, cmd.Stderr = nil, nil
	applyDetach(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

func isStoreCodex(exe string) bool {
	p := strings.ToLower(filepath.Clean(exe))
	return strings.Contains(p, `\windowsapps\openai.codex_`) &&
		(strings.HasSuffix(p, `\app\chatgpt.exe`) || strings.HasSuffix(p, `\app\codex.exe`))
}

func isStoreAlias(exe string) bool {
	p := strings.ToLower(filepath.Clean(exe))
	base := strings.ToLower(filepath.Base(p))
	if base != "chatgpt.exe" && base != "codex.exe" {
		return false
	}
	la := strings.ToLower(os.Getenv("LOCALAPPDATA"))
	return la != "" && strings.Contains(p, strings.ToLower(filepath.Join(la, "Microsoft", "WindowsApps")))
}

type processEntry32W struct {
	Size            uint32
	Usage           uint32
	ProcessID       uint32
	DefaultHeapID   uintptr
	ModuleID        uint32
	Threads         uint32
	ParentProcessID uint32
	PriClassBase    int32
	Flags           uint32
	ExeFile         [260]uint16
}

func codexProcessIDs(exe string) []uint32 {
	snapshot, _, _ := procCreateToolhelp32Snapshot.Call(th32csSnapProcess, 0)
	if snapshot == 0 || snapshot == invalidHandleValue {
		return nil
	}
	defer procCloseHandle.Call(snapshot)

	var entry processEntry32W
	entry.Size = uint32(unsafe.Sizeof(entry))
	ok, _, _ := procProcess32FirstW.Call(snapshot, uintptr(unsafe.Pointer(&entry)))
	if ok == 0 {
		return nil
	}
	var ids []uint32
	for {
		if entry.ProcessID > 4 {
			path := windowsProcessImagePath(entry.ProcessID)
			if path != "" && isCodexProcessPath(path, exe) {
				ids = append(ids, entry.ProcessID)
			}
		}
		entry.Size = uint32(unsafe.Sizeof(entry))
		ok, _, _ = procProcess32NextW.Call(snapshot, uintptr(unsafe.Pointer(&entry)))
		if ok == 0 {
			break
		}
	}
	return ids
}

func windowsProcessImagePath(pid uint32) string {
	handle, _, _ := procOpenProcess.Call(processQueryLimitedInfo, 0, uintptr(pid))
	if handle == 0 {
		return ""
	}
	defer procCloseHandle.Call(handle)
	buf := make([]uint16, 32768)
	size := uint32(len(buf))
	ok, _, _ := procQueryFullProcessImageNameW.Call(
		handle,
		0,
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&size)),
	)
	if ok == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf[:size])
}

func isCodexProcessPath(path, target string) bool {
	cleanPath := strings.ToLower(filepath.Clean(path))
	cleanTarget := strings.ToLower(filepath.Clean(target))
	if cleanPath == cleanTarget {
		return true
	}
	if strings.Contains(cleanPath, `\windowsapps\openai.codex_`) {
		return strings.HasSuffix(cleanPath, `\app\chatgpt.exe`) || strings.HasSuffix(cleanPath, `\app\codex.exe`)
	}
	return false
}

func terminateCodexProcesses(exe string) ([]uint32, error) {
	ids := codexProcessIDs(exe)
	var terminated []uint32
	var lastErr error
	for _, pid := range ids {
		handle, _, _ := procOpenProcess.Call(processTerminate, 0, uintptr(pid))
		if handle == 0 {
			lastErr = fmt.Errorf("无法打开 Codex 进程 %d", pid)
			continue
		}
		ok, _, callErr := procTerminateProcess.Call(handle, 1)
		procCloseHandle.Call(handle)
		if ok == 0 {
			lastErr = fmt.Errorf("结束 Codex 进程 %d 失败: %v", pid, callErr)
			continue
		}
		terminated = append(terminated, pid)
	}
	if len(terminated) == 0 && len(ids) > 0 && lastErr != nil {
		return nil, lastErr
	}
	return terminated, nil
}

func processAlive(pid int) bool {
	handle, _, _ := procOpenProcess.Call(processQueryLimitedInfo, 0, uintptr(uint32(pid)))
	if handle == 0 {
		return false
	}
	defer procCloseHandle.Call(handle)
	var exitCode uint32
	ok, _, _ := procGetExitCodeProcess.Call(handle, uintptr(unsafe.Pointer(&exitCode)))
	return ok != 0 && exitCode == 259 // STILL_ACTIVE
}
