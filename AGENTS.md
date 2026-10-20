# AGENTS.md

> 本文件是给 AI 编码代理（OpenAI Codex、Claude Code、Cursor 等）的项目说明书。
> 读到本文件后，按 `docs/AI_DEPLOY.md` 执行部署，并在完成后**主动告诉用户怎么用**（见 5.1）。

## 1. 项目是什么

`codex-chinese` 是一个**单文件、零第三方依赖、跨平台**的 Go 小程序，用于强制 Codex 桌面应用
使用其内置 `zh-CN` 语言包，避免 Statsig / VPN 不通时回退英文。

它**只做一件事**：通过 Chrome DevTools Protocol 往 Codex 渲染进程注入一段 prelude，
在运行时给 Statsig 的 i18n 开关（Layer/DynamicConfig `"72216192"`）打补丁。

提供两个形态：

| 形态 | 平台 | 说明 |
|---|---|---|
| `codex-chinese`（CLI） | Windows / macOS / Linux | 命令行运行，输出结果 |
| `codex-chinese-tray`（托盘） | 仅 Windows | 常驻托盘，右键菜单「立即强制中文 / 环境诊断 / 退出」 |

### 它不会做的事（重要边界）
- 不修改 Codex 的安装文件、`app.asar` 或配置文件。
- 不联网、不上传任何数据。
- 不是 OpenAI 官方项目。

## 2. 仓库结构

```
main.go               核心 CLI：找 Codex、启动、HTTP、WebSocket、CDP、prelude
detach_windows.go     Windows 进程分离（DETACHED_PROCESS）
detach_unix.go        macOS/Linux 进程分离（Setsid）
console_windows.go    Windows 控制台 UTF-8
console_other.go      非 Windows 空实现
tray/main_windows.go  Windows 托盘（纯 Win32，无 cgo）
tray/main_other.go    非 Windows 占位（提示仅支持 Windows）
build.sh / build.ps1  交叉编译 CLI 六平台 + 托盘两平台
deploy.json           机器可读的构建/产物/校验清单
docs/AI_DEPLOY.md     给 AI 的确定性部署方案（执行入口）
docs/ARCHITECTURE.md  原理与数据流
scripts/ai_deploy.*   一键部署脚本（AI 可直接运行）
.github/workflows/    CI 与 Release
```

## 3. 硬性约束（不要违反）

1. **零第三方依赖**：`go.mod` 不允许出现 `require`。HTTP/WebSocket 已手写，禁止引入
   `net/http`、`crypto/tls`、`gorilla/websocket`、`systray` 等。
2. **控制体积**：单个 CLI 产物 3–4 MB；超过 6 MB 视为回归。
3. **托盘只用 Win32**：`tray/` 通过 `syscall` 调 user32/shell32，不得引入 cgo 或 GUI 库。
4. **不改用户环境**：除 `dist/` 外不写用户目录；不修改 Codex 安装目录。
5. **不用 UPX**：未签名压缩 exe 会被杀软误报。
6. **平台代码用 build tag**：新增平台相关代码必须加 `//go:build`。
7. **不擅自发布**：没有用户明确授权，不执行 `git push`、不创建 Release。

## 4. 常用命令

```bash
go vet ./...                     # 静态检查（含 tray 包）
VERSION=v1.1.0 bash build.sh     # CLI 6 平台 + 托盘 2 平台
make release VERSION=v1.1.0      # 同上
./dist/codex-chinese-<os>-<arch> -version   # 版本
./dist/codex-chinese-<os>-<arch> -doctor    # 环境诊断（0=可用, 2=未找到 Codex）
./dist/codex-chinese-tray-windows-amd64.exe # Windows 托盘
```

## 5. AI 部署任务（关键）

当用户要求“部署 / 构建 / 安装这个项目”时：

1. 阅读 `docs/AI_DEPLOY.md`。
2. 直接运行 `scripts/ai_deploy.sh`（Windows 用 `scripts/ai_deploy.ps1`）。
3. 核对 JSON：`ok=true`、`artifacts` 数量为 **8**（6 个 CLI + 2 个托盘）、
   `doctor_exit` 为 0 或 2。
4. 按 5.1 把使用说明讲给用户。

**成功判定**：`-version` 输出以 `codex-chinese ` 开头；8 个产物均存在；`go vet` 退出码 0。
**失败处理**：按 `docs/AI_DEPLOY.md` 的“故障对照表”处理；无法解决时原样返回错误，不要猜测。

### 5.1 部署完成后必须告诉用户怎么用（不可省略）

部署成功后，**不要只丢一段 JSON**。必须用用户使用的语言，按下面结构给出可操作说明：

1. **产物在哪**：列出 `dist/` 路径与对应平台的推荐文件。
2. **怎么启动**：
   - Windows 想要常驻：双击 `codex-chinese-tray-windows-amd64.exe`，右键托盘图标选「立即强制中文」。
   - Windows 一次性：运行 `codex-chinese-windows-amd64.exe`。
   - macOS / Linux：`./codex-chinese-darwin-arm64`（或对应平台文件）。
3. **首次使用前置条件**：先**完全退出 Codex（含托盘图标）**。
4. **成功标志**：终端出现 `完成：Codex 已强制中文。`。
5. **常用参数**：`-codex` 指定路径、`-port` 端口、`-no-launch` 只注入、`-doctor` 诊断。
6. **重要提醒**：每次重启 Codex 后需重新运行（或让托盘版常驻）。
7. **故障排查**：端口超时 → 退出 Codex 重试；macOS 被拦 → `xattr -d com.apple.quarantine`；
   Windows SmartScreen → 更多信息 → 仍要运行。

可直接复用的话术：

> 部署完成 ✅
> 产物在 `dist/`。Windows 推荐双击 `codex-chinese-tray-windows-amd64.exe`（托盘常驻），
> 右键图标选「立即强制中文」；也可以直接运行 `codex-chinese-windows-amd64.exe`。
> 使用前请先完全退出 Codex（含托盘图标）。看到「完成：Codex 已强制中文」即成功。
> 每次重启 Codex 后需要再执行一次；用 `-doctor` 可诊断环境。

## 6. 发布流程（仅在有授权时）

```bash
git tag v1.1.0
git push origin v1.1.0      # 触发 .github/workflows/release.yml
```

Release 工作流会自动构建 CLI 六平台 + 托盘两平台、生成 `SHA256SUMS` 并创建 GitHub Release。
