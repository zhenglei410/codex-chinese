# AGENTS.md

> 本文件是给 AI 编码代理（OpenAI Codex、Claude Code、Cursor 等）的项目说明书。
> 读到本文件后，按 `docs/AI_DEPLOY.md` 执行部署，不要自行发明步骤。

## 1. 项目是什么

`codex-chinese` 是一个**单文件、零第三方依赖、跨平台**的 Go 小程序，用于强制 Codex 桌面应用
使用其内置 `zh-CN` 语言包，避免 Statsig / VPN 不通时回退英文。

它**只做一件事**：通过 Chrome DevTools Protocol 往 Codex 渲染进程注入一段 prelude，
在运行时给 Statsig 的 i18n 开关（Layer/DynamicConfig `"72216192"`）打补丁。

### 它不会做的事（重要边界）
- 不修改 Codex 的安装文件、`app.asar` 或配置文件。
- 不联网、不上传任何数据。
- 不替用户下单、不操作业务系统。
- 不是 OpenAI 官方项目。

## 2. 仓库结构

```
main.go               核心：找 Codex、启动、HTTP、WebSocket、CDP、prelude
detach_windows.go     Windows 进程分离（DETACHED_PROCESS）
detach_unix.go        macOS/Linux 进程分离（Setsid）
console_windows.go    Windows 控制台 UTF-8
console_other.go      非 Windows 空实现
build.sh / build.ps1  六平台交叉编译
deploy.json           机器可读的构建/产物/校验清单
docs/AI_DEPLOY.md     给 AI 的确定性部署方案（执行入口）
docs/ARCHITECTURE.md  原理与数据流
scripts/ai_deploy.*   一键部署脚本（AI 可直接运行）
.github/workflows/    CI 与 Release
```

## 3. 硬性约束（不要违反）

1. **零第三方依赖**：`go.mod` 不允许出现 `require`。HTTP/WebSocket 已手写，禁止引入 `net/http`、
   `crypto/tls`、`gorilla/websocket` 等。
2. **控制体积**：单个产物应保持在 3–4 MB；若超过 6 MB 视为回归，需查明原因。
3. **不改用户环境**：除 `dist/` 外不写用户目录；不修改 Codex 安装目录。
4. **不用 UPX**：未签名压缩 exe 会被杀软误报。
5. **平台代码用 build tag**：新增平台相关代码必须放在 `*_windows.go` / `*_unix.go` 并加
   `//go:build` 约束。
6. **不擅自发布**：没有用户明确授权，不执行 `git push`、不创建 Release。

## 4. 常用命令

```bash
go vet ./...                     # 静态检查
VERSION=v1.0.0 bash build.sh     # 六平台构建，产物在 dist/
make release VERSION=v1.0.0      # 同上
./dist/codex-chinese-<os>-<arch> -version   # 版本
./dist/codex-chinese-<os>-<arch> -doctor    # 环境诊断（exit 0=可用, 2=未找到 Codex）
```

## 5. AI 部署任务（关键）

当用户要求“部署 / 构建 / 安装这个项目”时：

1. 阅读 `docs/AI_DEPLOY.md`。
2. 直接运行 `scripts/ai_deploy.sh`（Windows 用 `scripts/ai_deploy.ps1`）。
3. 核对脚本输出的 JSON：`ok=true`、`artifacts` 数量为 6、`doctor_exit` 为 0 或 2。
4. 把产物路径与校验结果返回给用户。

**成功判定**：`-version` 输出以 `codex-chinese ` 开头；六个产物均存在；`go vet` 退出码 0。

**失败处理**：按 `docs/AI_DEPLOY.md` 的“故障对照表”处理；无法解决时原样返回错误输出，不要猜测。

## 6. 发布流程（仅在有授权时）

```bash
git tag v1.0.0
git push origin v1.0.0      # 触发 .github/workflows/release.yml
```

Release 工作流会自动交叉编译六平台、生成 `SHA256SUMS` 并创建 GitHub Release。
