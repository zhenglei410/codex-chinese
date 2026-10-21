# AGENTS.md

本文件是仓库的唯一部署契约。人类只需把仓库链接交给 AI；AI 应按本文件完成构建、
安装、验证和交付，不应沿用旧版教程或自行发明启动脚本。

仓库：

```text
Gitee（国内推荐）: https://gitee.com/zhenglei410/codex-chinese
GitHub:
https://github.com/zhenglei410/codex-chinese
```

## 1. 目标

在目标机器上部署 `codex-chinese v2.0.7`：

- Windows 安装 CLI、托盘、快捷方式和当前用户登录自启动；
- 让 Codex 桌面应用显示内置 `zh-CN` 中文界面；
- 后台长驻注入在页面刷新后自动恢复；
- 最终通过 `codex-chinese -verify` 证明成功。

当前 Windows 重点验证目标是 Microsoft Store/MSIX 版 Codex。

## 2. Windows 标准部署

优先直接下载 Release，不要求克隆仓库，也不要求安装 Go 或 Node。

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

安装器会从对应 Release 下载：

- CLI；
- 托盘；
- 应用图标；
- `SHA256SUMS`。

下载后先校验 SHA256，再安装。成功时必须满足：

- Windows CLI 与托盘安装完成；
- 快捷方式直接指向 `codex-chinese-tray.exe`；
- 快捷方式图标指向 `codex-chinese.ico`；
- `HKCU\Software\Microsoft\Windows\CurrentVersion\Run\CodexChinese` 指向安装后的托盘。

安装器不主动结束当前正在使用的 Codex。完成安装后重启 Codex，再执行 `-verify`。

### 开发源码构建

仅修改代码或 Release 不可用时使用：

```powershell
git clone https://gitee.com/zhenglei410/codex-chinese.git
cd codex-chinese
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\ai_deploy.ps1 -Version v2.0.7
```

源码构建需要 Go 1.21+。正式发布前还必须安装 Node.js 并运行 prelude 测试。

## 3. 让中文立即生效

部署后有两种方式：

1. 让用户重启 Codex；登录自启动项会在下次 Windows 登录时启动托盘。
2. 在获得用户允许后，双击桌面「Codex 中文」。托盘会自动接管 Codex。

如果 AI 本身运行在当前 Codex 窗口内，直接启动托盘会让 Codex 重启一次，当前工具调用
可能中断。执行前明确告知用户；不要误判为部署失败。

重启后运行：

```powershell
& "$env:LOCALAPPDATA\Programs\codex-chinese\codex-chinese.exe" -verify
```

唯一成功判据：

```text
verify: installed=true lang=zh-CN i18n=true html=zh-CN
PASS
```

退出码必须为 `0`。

以下状态都不算成功：

- 安装目录存在；
- 快捷方式存在；
- 托盘进程存在；
- Codex 窗口能打开；
- 程序打印“完成”。

## 4. macOS / Linux

```bash
git clone https://gitee.com/zhenglei410/codex-chinese.git
cd codex-chinese
VERSION=v2.0.7 bash build.sh
go vet ./...
node test/prelude.test.mjs
./dist/codex-chinese-<os>-<arch>
./dist/codex-chinese-<os>-<arch> -verify
```

托盘守护与登录自启动目前仅支持 Windows。

## 5. 失败时

先采集证据：

```powershell
& "$env:LOCALAPPDATA\Programs\codex-chinese\codex-chinese.exe" -doctor
& "$env:LOCALAPPDATA\Programs\codex-chinese\codex-chinese.exe" -verify
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\diagnose_windows.ps1
```

日志：

```text
%TEMP%\codex-chinese-tray.log
%TEMP%\codex-chinese-supervisor.log
```

常见判断：

- `cdp not-ready`：没有受控调试端口。运行托盘或 CLI，让工具接管 Codex。
- `installed=false`：页面没有执行 prelude，检查启动链路和 supervisor 日志。
- `i18n=false`：Statsig Layer 补丁未生效，检查 Codex 版本和 `72216192` 是否变化。
- 反复启动/关闭：确认只有一个 `codex-chinese-tray.exe` 和一个 supervisor，
  并确认没有使用旧版二进制。
- 托盘启动时 Codex 被重启：这是预期行为；工具必须用带调试端口的实例启动 Codex。

修复后再跑完整验证。不要伪造 PASS，也不要把原始错误直接丢给用户。

## 6. 禁止事项

- 不要创建 `.bat` / `.vbs` 启动器。
- 不要创建指向 `cmd.exe` / `wscript.exe` 的快捷方式。
- 不要直接运行或修改 `WindowsApps` 中的 Codex 文件。
- 不要在 `WindowsApps` 中递归遍历寻找版本目录；使用注册表和 AUMID。
- 不要写死 Codex 版本目录。
- 不要同时保留多个不同端口的 supervisor。
- 不要在 Codex bootstrap 期间强制刷新页面。
- 不要把 `-verify` 失败解释成浏览器缓存问题。
- 不要读取、输出或上传 Codex API Key、令牌和对话内容。
- 不要在未经用户授权时执行 `git push`、创建 Release 或修改远程仓库。

## 7. 架构不变量

修改代码时必须保留：

1. `acquireLaunchLock()` 全局串行化启动任务。
2. `acquireSupervisorLock()` 全局保证只有一个 CDP supervisor。
3. `choosePort()` 先复用现有 Codex CDP 实例，再选择空闲端口。
4. MSIX 启动使用原生 `IApplicationActivationManager`。
5. 当前文档执行 prelude，并注册到后续文档；不强制刷新首屏。
6. 托盘守护进程通过 `guard-pid` 与托盘生命周期绑定。
7. Statsig 包装器必须返回原始 Layer / DynamicConfig 对象。
8. 托盘构建必须带 `-H=windowsgui`。

架构细节见 [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md)。

## 8. 修改后的自证

```powershell
$env:VERSION="v2.0.7"
.\build.ps1
go vet ./...
node test/prelude.test.mjs
Get-ChildItem .\dist
```

至少应在真实 Windows + MSIX Codex 环境完成一次：

```powershell
codex-chinese.exe -no-launch
codex-chinese.exe -verify
```

并确认退出码为 `0`。
