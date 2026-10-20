//go:build windows

package main

import (
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"unsafe"
)

// 当前用户的 MSIX/商店应用仓库，普通权限即可读取。
const appModelPackagesKey = `Software\Classes\Local Settings\Software\Microsoft\Windows\CurrentVersion\AppModel\Repository\Packages`

// storeCandidatesFromRegistry 从 AppModel 仓库里找出商店（MSIX）版 Codex 的真实路径。
//
// 为什么不能只靠 filepath.Glob：C:\Program Files\WindowsApps 对普通用户
// 只有"已知完整路径可访问"的权限，**没有列目录权限**，Glob 必然返回空。
// 而注册表里的 PackageRootFolder 是可读的，拿到根目录后拼 app\ChatGPT.exe 即可。
//
// 商店版 Codex 的可执行文件叫 ChatGPT.exe，包名形如
// OpenAI.Codex_26.1013.4894.0_x64__2p2nqsd0c76g0，版本号随机。
func storeCandidatesFromRegistry() []string {
	p, err := syscall.UTF16PtrFromString(appModelPackagesKey)
	if err != nil {
		return nil
	}
	var hKey syscall.Handle
	if err := syscall.RegOpenKeyEx(syscall.HKEY_CURRENT_USER, p, 0, syscall.KEY_READ, &hKey); err != nil {
		return nil
	}
	defer syscall.RegCloseKey(hKey)

	type pkg struct{ name, root string }
	var pkgs []pkg
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
		pkgs = append(pkgs, pkg{name, root})
	}
	// 包名内含版本号，字典序倒序即新版本在前
	sort.Slice(pkgs, func(i, j int) bool { return pkgs[i].name > pkgs[j].name })

	var out []string
	for _, p := range pkgs {
		for _, rel := range []string{`app\ChatGPT.exe`, `app\Codex.exe`, `app\codex.exe`, `ChatGPT.exe`} {
			out = append(out, filepath.Join(p.root, rel))
		}
	}
	return out
}

// regQueryString 读取注册表字符串值，失败返回空串。
func regQueryString(parent syscall.Handle, sub, value string) string {
	sp, err := syscall.UTF16PtrFromString(sub)
	if err != nil {
		return ""
	}
	var h syscall.Handle
	if err := syscall.RegOpenKeyEx(parent, sp, 0, syscall.KEY_READ, &h); err != nil {
		return ""
	}
	defer syscall.RegCloseKey(h)

	vp, err := syscall.UTF16PtrFromString(value)
	if err != nil {
		return ""
	}
	var typ uint32
	buf := make([]uint16, 1024)
	size := uint32(len(buf) * 2)
	if err := syscall.RegQueryValueEx(h, vp, nil, &typ, (*byte)(unsafe.Pointer(&buf[0])), &size); err != nil {
		return ""
	}
	if typ != syscall.REG_SZ && typ != syscall.REG_EXPAND_SZ {
		return ""
	}
	return syscall.UTF16ToString(buf)
}
