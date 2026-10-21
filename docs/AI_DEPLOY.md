# AI 部署手册

目标版本：`v2.0.7`
目标平台：Windows 10/11（优先 MSIX/Store 版 Codex）

本文供 AI 编码代理执行。完成标准不是“命令没报错”，而是
`codex-chinese.exe -verify` 退出码为 `0`。

## 1. 最短流程

直接下载 Release，不需要克隆源码、Go 或 Node。

国内推荐：

```powershell
$installer = Join-Path $env:TEMP "codex-chinese-install.ps1"
Invoke-WebRequest -UseBasicParsing `
  -Uri "https://gitee.com/zhenglei410/codex-chinese/raw/master/scripts/install_windows.ps1" `
  -OutFile $installer
powershell -NoProfile -ExecutionPolicy Bypass -File $installer `
  -Version v2.0.7 -ReleaseSource gitee -Mode tray
```

海外使用：

```powershell
$installer = Join-Path $env:TEMP "codex-chinese-install.ps1"
Invoke-WebRequest -UseBasicParsing `
  -Uri "https://raw.githubusercontent.com/zhenglei410/codex-chinese/main/scripts/install_windows.ps1" `
  -OutFile $installer
powershell -NoProfile -ExecutionPolicy Bypass -File $installer `
  -Version v2.0.7 -ReleaseSource github -Mode tray
```

安装器会自动下载 CLI、托盘、图标和 `SHA256SUMS`，校验后完成安装。

## 2. 前置条件

| 工具 | 要求 | 缺失时 |
|---|---|---|
| PowerShell | Windows PowerShell 5.1+ | 使用系统自带版本 |
| Codex | 已安装并至少启动过一次 | 先启动 Codex |
| Release | `v2.0.7` 已发布 | 改用源码构建流程 |

Release 安装不需要 Git、Go、Node 或 Visual C++ 运行库。

## 3. Release 安装器做了什么

1. 从 Gitee 或 GitHub Release 下载当前架构的 CLI、托盘和图标。
2. 下载 `SHA256SUMS` 并校验全部文件。
3. 停止旧安装目录中的托盘和 supervisor。
4. 安装到 `%LOCALAPPDATA%\Programs\codex-chinese`。
5. 创建桌面与开始菜单快捷方式。
6. 写入当前用户登录启动项。

安装完成后重启 Codex，或通过桌面快捷方式启动托盘。

## 4. 源码构建（开发用）

只有修改源码或 Release 不可用时才使用：

```powershell
git clone https://gitee.com/zhenglei410/codex-chinese.git
cd codex-chinese
powershell -NoProfile -ExecutionPolicy Bypass `
  -File .\scripts\ai_deploy.ps1 -Version v2.0.7
```

源码构建需要 Git、Go 1.21+；正式发布前还需要 Node.js 运行 prelude 测试。

手动等价步骤：

```powershell
$env:VERSION = "v2.0.7"
.\build.ps1

go vet ./...
node .\test\prelude.test.mjs

powershell -NoProfile -ExecutionPolicy Bypass `
  -File .\scripts\install_windows.ps1 `
  -Version v2.0.7 `
  -AssetDir .\dist `
  -Mode tray
```

8 个二进制产物及图标：

```text
codex-chinese-windows-amd64.exe
codex-chinese-windows-arm64.exe
codex-chinese-darwin-amd64
codex-chinese-darwin-arm64
codex-chinese-linux-amd64
codex-chinese-linux-arm64
codex-chinese-tray-windows-amd64.exe
codex-chinese-tray-windows-arm64.exe
codex-chinese.ico
```

## 5. 安装结果

```text
%LOCALAPPDATA%\Programs\codex-chinese\codex-chinese.exe
%LOCALAPPDATA%\Programs\codex-chinese\codex-chinese-tray.exe
%LOCALAPPDATA%\Programs\codex-chinese\codex-chinese.ico
桌面\Codex 中文.lnk
HKCU\Software\Microsoft\Windows\CurrentVersion\Run\CodexChinese
```

快捷方式必须直接指向 `codex-chinese-tray.exe`。

登录启动项值：

```text
"C:\Users\<用户>\AppData\Local\Programs\codex-chinese\codex-chinese-tray.exe"
```

## 6. 启动与接管

托盘启动后等待约 6 秒，然后执行一次接管：

- 如果已有受控 Codex CDP 实例：直接复用；
- 如果没有：通过原生 MSIX AUMID 启动；
- 如果 Codex 已运行但未开放 CDP：结束该实例并重新启动一次。

不要手动先启动 Codex 后再运行注入器。必须让本工具控制启动参数。

## 7. 成功验证

```powershell
$cli = "$env:LOCALAPPDATA\Programs\codex-chinese\codex-chinese.exe"
& $cli -verify
$LASTEXITCODE
```

期望：

```text
verify: installed=true lang=zh-CN i18n=true html=zh-CN
PASS
0
```

`html=` 是附加诊断字段。核心成功字段是：

- `installed=true`
- `lang=zh-CN`
- `i18n=true`
- 退出码 `0`

## 8. 失败处理

先运行：

```powershell
$cli = "$env:LOCALAPPDATA\Programs\codex-chinese\codex-chinese.exe"
& $cli -doctor
& $cli -verify
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\diagnose_windows.ps1
```

日志：

```text
%TEMP%\codex-chinese-tray.log
%TEMP%\codex-chinese-supervisor.log
```

对照：

| 现象 | 原因 | 动作 |
|---|---|---|
| `cdp not-ready` | Codex 不是由工具启动，或已退出 | 运行托盘/CLI 接管 |
| `installed=false` | prelude 未在页面执行 | 检查 supervisor 日志 |
| `i18n=false` | Statsig Layer 未命中 | 检查 Codex 版本和 `72216192` |
| 反复启动关闭 | 多版本/多端口 supervisor | 只保留当前安装目录的进程 |
| 窗口不重开 | 启动进程跟着旧 Codex 被结束 | 从桌面快捷方式或登录项启动托盘 |

修复后必须重新跑 `-verify`，不可只检查进程是否存在。

## 9. 更新

覆盖安装即可：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass `
  -File .\scripts\install_windows.ps1 `
  -Version <版本> -AssetDir .\dist -Mode tray
```

安装器会删除旧快捷方式并重建正确入口。

## 10. 卸载

```powershell
powershell -NoProfile -ExecutionPolicy Bypass `
  -File .\scripts\install_windows.ps1 -Uninstall
```

不会修改 Codex 本体。
