<div align="center">

# codex-chinese

**强制 Codex 桌面应用使用内置 zh-CN 中文界面**
即使 Statsig / VPN 不通，也不再回退英文。

[![Go](https://img.shields.io/badge/Go-1.21+-00ADD8?logo=go)](https://go.dev)
[![Platform](https://img.shields.io/badge/platform-Windows%20%7C%20macOS%20%7C%20Linux-4B5563)](#下载)
[![Size](https://img.shields.io/badge/binary-3.5%20MB-2ea44f)](#体积)
[![License](https://img.shields.io/badge/license-AGPL--3.0-blue)](LICENSE)

</div>

---

## 为什么需要它

Codex 桌面应用的中文（`zh-CN`）语言包是内置的，但**是否启用**由 Statsig 下发的开关控制
（`Layer` / `DynamicConfig` 名 `"72216192"`，键 `enable_i18n`、`locale_source`）。
当 Statsig 或 VPN 不通时，这个开关取不到值，界面就回退成英文——即使你在设置里选了中文。

`codex-chinese` 只做一件事：在运行时把这个开关**强制打开**。

## 它怎么工作（一句话）

用 Chrome DevTools Protocol 往 Codex 渲染进程的主世界注入一段 prelude，
让它返回 `enable_i18n: true`、`locale_source: "SYSTEM"`，并把官方 `localeOverride` 设为 `zh-CN`。

> 不改安装文件、不改 `app.asar`、不联网、不上传数据。

## 下载

到 [Releases](https://github.com/zhenglei410/codex-chinese/releases) 下载对应平台：

| 平台 | 文件 |
|---|---|
| Windows x64 | `codex-chinese-windows-amd64.exe` |
| Windows ARM64 | `codex-chinese-windows-arm64.exe` |
| macOS Intel | `codex-chinese-darwin-amd64` |
| macOS Apple Silicon | `codex-chinese-darwin-arm64` |
| Linux x64 | `codex-chinese-linux-amd64` |
| Linux ARM64 | `codex-chinese-linux-arm64` |
| Windows x64（托盘） | `codex-chinese-tray-windows-amd64.exe` |
| Windows ARM64（托盘） | `codex-chinese-tray-windows-arm64.exe` |

> 托盘版需与同目录的 `codex-chinese.exe` 一起使用（它负责实际注入）。

## 使用

> **先完全退出 Codex（含托盘图标）**，再运行。

```bash
# Windows
codex-chinese-windows-amd64.exe

# macOS / Linux
chmod +x codex-chinese-darwin-arm64
./codex-chinese-darwin-arm64
```

运行后终端会显示：

```
== 强制 Codex 中文界面 ==
注入目标: app://...
已注册注入脚本（对后续所有页面生效）。
注入已生效（页面已加载）。
完成：Codex 已强制中文。
```

但**不要只看这几行就以为成功了**——它只代表流程走完。真正的判据是：

```bash
codex-chinese-windows-amd64.exe -verify
# verify: installed=true lang=zh-CN i18n=true
# PASS                        ← 退出码 0 才算成功
```

三个数字缺一不可：`installed=true`、`lang=zh-CN`、`i18n=true`。

### Windows 托盘版（常驻）

双击 `codex-chinese-tray-windows-amd64.exe`，托盘出现图标后右键：

- **立即强制中文** —— 启动/注入并刷新
- **环境诊断** —— 显示 Codex 路径与调试端口状态
- **退出**

托盘版会调用同目录的 `codex-chinese.exe`，所以两个文件要放在一起。
它常驻后，每次重启 Codex 只需再点一次「立即强制中文」。

### 参数

| 参数 | 说明 |
|---|---|
| `-codex <path>` | 手动指定 Codex 可执行文件 |
| `-port <n>` | CDP 调试端口，默认 `9229` |
| `-wait <n>` | 等端口就绪的秒数，默认 `20` |
| `-no-launch` | 只注入已开调试端口的实例 |
| `-no-reload` | 只注册脚本，不刷新 |
| `-version` | 打印版本 |
| `-doctor` | 环境诊断（`exit 0` 可用 / `exit 2` 未找到 Codex） |
| `-verify` | 校验是否真的已注入中文（`PASS` + `exit 0` 为准） |

### 常见问题

- **macOS 提示"无法打开，因为无法验证开发者"**
  `xattr -d com.apple.quarantine ./codex-chinese-darwin-arm64`
- **Windows SmartScreen 拦截**
  点"更多信息 → 仍要运行"（本程序未做代码签名）。
- **提示"等待调试端口超时"**
  Codex 还在后台运行。请完全退出（含托盘图标）后重试。
- **桌面快捷方式指向 `wscript.exe` / `.vbs`，界面仍是英文**
  这是错误部署。VBS 不会给 Codex 传调试端口，注入不可能生效。
  详见 [docs/TROUBLESHOOTING.md](docs/TROUBLESHOOTING.md)。
- **怎么确认真的生效了**
  运行 `codex-chinese.exe -verify`，输出 `PASS` 才算成功。
  完整判据是这一行：`verify: installed=true lang=zh-CN i18n=true`。
  如果显示 `installed=true` 但 `i18n=false`，说明是 v1.1.0 及更早版本
  （已修复），换 v1.2.0+ 即可。
- **每次重启 Codex 都要重跑吗？**
  是。程序不常驻、不写注册表；需要的话可自行做开机自启。

## 从源码构建

需要 Go ≥ 1.21，**零第三方依赖**。跑测试需要 Node ≥ 18（仅测试用）。

```bash
node test/prelude.test.mjs       # 注入脚本回归测试（11 项）
go vet ./...
VERSION=v1.2.0 bash build.sh     # 六个平台，产物在 dist/
# Windows:
$env:VERSION="v1.2.0"; .\build.ps1
```

实测体积：

| 目标 | 大小 |
|---|---|
| windows/amd64 | 3.6 MB |
| windows/arm64 | 3.3 MB |
| darwin/amd64 | 3.5 MB |
| darwin/arm64 | 3.3 MB |
| linux/amd64 | 3.4 MB |
| linux/arm64 | 3.3 MB |

体积来自三处优化：`CGO_ENABLED=0` 静态链接、不用 `net/http`/`crypto/tls`（手写 HTTP + WebSocket）、
`-trimpath -ldflags "-s -w"`。

## 让 AI 自己部署

本仓库内置了机器可读的部署方案：

- [`AGENTS.md`](AGENTS.md) —— 项目约定与入口（Codex 等代理会自动读取）
- [`docs/AI_DEPLOY.md`](docs/AI_DEPLOY.md) —— 确定性步骤、期望输出、故障对照表
- [`deploy.json`](deploy.json) —— 构建/产物/校验的机器可读清单
- [`scripts/ai_deploy.sh`](scripts/ai_deploy.sh) / [`ai_deploy.ps1`](scripts/ai_deploy.ps1) —— 一键执行并输出 JSON 报告

一句话给代理：

> 读 `AGENTS.md`，Windows 优先运行 `scripts/install_windows.ps1`（下载发行版、建正确的 exe 快捷方式），
> 启动后用 `codex-chinese.exe -verify` 校验，**输出 `PASS` 才算成功**。

按 `AGENTS.md` 5.1 的硬性要求，**AI 在部署完成后必须主动告诉用户怎么用**
（产物位置、启动方式、首次前置、成功标志、常用参数、重启提醒、排障），
而不是只返回一段 JSON。

## 免责声明

本项目为**非官方**社区工具，与 OpenAI 无关联。仅修改运行时行为，不修改 Codex 本体。
Codex 升级后若 Statsig 的 Layer 名或键名变化，功能可能失效——欢迎提 Issue。

## 许可证

[AGPL-3.0](LICENSE)。思路参考了 [CodexPlusPlus](https://github.com/BigPizzaV3/CodexPlusPlus)，
本实现为独立重写。
