# AI 部署手册

目标版本：`v2.0.6`
目标平台：Windows 10/11（优先 MSIX/Store 版 Codex）

本文供 AI 编码代理执行。完成标准不是“命令没报错”，而是
`codex-chinese.exe -verify` 退出码为 `0`。

## 1. 最短流程

```powershell
git clone https://github.com/zhenglei410/codex-chinese.git
cd codex-chinese
powershell -NoProfile -ExecutionPolicy Bypass `
  -File .\scripts\ai_deploy.ps1 -Version v2.0.6
```

脚本会构建、测试、安装并生成 JSON 报告。

默认不会结束当前 Codex。若用户要求立即生效：

```powershell
Start-Process explorer.exe `
  -ArgumentList ('"' + "$env:LOCALAPPDATA\Programs\codex-chinese\codex-chinese-tray.exe" + '"')
```

这会重启一次 Codex。若 AI 运行在 Codex 内，当前工具调用可能中断，必须先告知用户。

## 2. 前置条件

| 工具 | 要求 | 缺失时 |
|---|---|---|
| Git | 能克隆 GitHub 仓库 | 用系统包管理器安装 Git |
| Go | 1.21+ | `winget install GoLang.Go` |
| Node.js | 18+，用于 prelude 测试 | `winget install OpenJS.NodeJS.LTS` |
| Codex | 已安装并至少启动过一次 | 先启动 Codex |

Windows 安装器不需要 Visual C++ 运行库；Go 产物为静态编译。

## 3. `ai_deploy.ps1` 做了什么

1. 检查 Go 版本。
2. 执行 `build.ps1`，生成 6 个 CLI 和 2 个托盘产物。
3. 执行 `go vet ./...`。
4. 检测 Node；存在时运行 `test/prelude.test.mjs`。
5. 检查 8 个产物存在且小于 6 MB。
6. 生成 `dist/SHA256SUMS`。
7. 调用 `install_windows.ps1 -AssetDir dist -Mode tray`。
8. 校验安装文件哈希和登录启动项。
9. 输出 `deploy-report.json`。

查看报告：

```powershell
Get-Content .\deploy-report.json -Raw
```

必须包含：

```json
{
  "ok": true,
  "version": "v2.0.6",
  "artifact_count": 8
}
```

## 4. 手动等价步骤

```powershell
$env:VERSION = "v2.0.6"
.\build.ps1

go vet ./...
node .\test\prelude.test.mjs

powershell -NoProfile -ExecutionPolicy Bypass `
  -File .\scripts\install_windows.ps1 `
  -Version v2.0.6 `
  -AssetDir .\dist `
  -Mode tray
```

8 个产物：

```text
codex-chinese-windows-amd64.exe
codex-chinese-windows-arm64.exe
codex-chinese-darwin-amd64
codex-chinese-darwin-arm64
codex-chinese-linux-amd64
codex-chinese-linux-arm64
codex-chinese-tray-windows-amd64.exe
codex-chinese-tray-windows-arm64.exe
```

## 5. 安装结果

```text
%LOCALAPPDATA%\Programs\codex-chinese\codex-chinese.exe
%LOCALAPPDATA%\Programs\codex-chinese\codex-chinese-tray.exe
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
