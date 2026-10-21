# 故障排查

## 先执行

```powershell
$cli = "$env:LOCALAPPDATA\Programs\codex-chinese\codex-chinese.exe"

& $cli -version
& $cli -doctor
& $cli -verify

powershell -NoProfile -ExecutionPolicy Bypass `
  -File .\scripts\diagnose_windows.ps1
```

日志：

```text
%TEMP%\codex-chinese-tray.log
%TEMP%\codex-chinese-supervisor.log
```

## 成功标准

```text
verify: installed=true lang=zh-CN i18n=true html=zh-CN
PASS
```

且退出码为 `0`。

## 常见问题

### `cdp not-ready`

Codex 不是由本工具启动，或 Codex 已退出。

处理：

1. 双击桌面「Codex 中文」。
2. 或在已开调试端口的实例上运行：

```powershell
& $cli -no-launch
```

### `installed=false`

页面没有执行 prelude。

检查：

- 是否只有一个托盘进程；
- supervisor 是否在运行；
- supervisor 日志是否显示反复连接/断开；
- 是否误用旧版本二进制。

### `i18n=false`

注入标记存在，但 Statsig 配置没有补到应用读取通道。

检查：

- Codex 版本；
- `72216192` 是否仍为当前 Layer/DynamicConfig 名；
- `getLayer` / `DynamicConfig` 包装是否保留原始返回值；
- 是否运行 `node test/prelude.test.mjs`。

### 窗口反复启动和关闭

历史上由多个不同端口的 supervisor 互相重启导致。

当前修复：

- 全局 launcher 锁；
- 全局 supervisor 锁；
- 优先复用已有 CDP 实例；
- 托盘内防重复执行。

处理：

1. 退出托盘。
2. 确认只保留新版安装目录。
3. 重新双击桌面「Codex 中文」。

查看进程：

```powershell
Get-CimInstance Win32_Process |
  Where-Object { $_.Name -match '^codex-chinese' } |
  Select-Object Name,ProcessId,CommandLine
```

正常应只有一个托盘和一个带 `-supervise` 的 CLI。

### 托盘启动后 Codex 重启一次

预期行为。

Codex 必须带 `--remote-debugging-port` 启动。如果已有普通实例占用单实例锁，
工具会结束该实例并从原生 AUMID 重新启动。

使用 `-no-restart` 可禁止该行为，但不会完成汉化。

### 找不到 Codex

先看：

```powershell
& $cli -doctor
```

MSIX 版应该输出：

```text
codex: C:\Program Files\WindowsApps\OpenAI.Codex_<版本>\app\ChatGPT.exe
```

不要把版本目录写死；工具会从 AppModel 注册表动态解析。

### Windows SmartScreen

当前产物未签名。选择“更多信息 → 仍要运行”。

### 托盘没有开机启动

检查：

```powershell
(Get-ItemProperty `
  'HKCU:\Software\Microsoft\Windows\CurrentVersion\Run').CodexChinese
```

重新安装可恢复：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass `
  -File .\scripts\install_windows.ps1 `
  -Version v2.0.6 -AssetDir .\dist -Mode tray
```

### macOS 无法打开

```bash
xattr -d com.apple.quarantine ./codex-chinese-darwin-arm64
```

## 卸载

```powershell
powershell -NoProfile -ExecutionPolicy Bypass `
  -File .\scripts\install_windows.ps1 -Uninstall
```

不会修改 Codex 本体。
