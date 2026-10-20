# AGENTS.md

> 本文件是给 AI 编码代理（OpenAI Codex、Claude Code、Cursor 等）的项目说明书。
> 读到本文件后，按 `docs/AI_DEPLOY.md` 执行部署，并在完成后**主动告诉用户怎么用**（见 5.1）。
>
> **遇到任何故障，先按第 6 节自愈**——采集证据、定位、改代码、自证，然后才汇报。
> 不要只把报错原样丢回给用户。用户明确要求：**AI 应当自己修，而不是等人来修。**

## 0. 先看这里：什么才算"做对了"（**必读，优先级最高**）

这个工具只做一件事：**用带 `--remote-debugging-port` 的方式启动 Codex，然后注入补丁**。
凡是绕开这一步的"聪明办法"，**全部无效**，而且通常不会报错。

| 环节 | ✅ 正确做法 | ❌ 错误做法（真实事故） |
|---|---|---|
| 启动 Codex | 直接运行 `codex-chinese*.exe`，它自己带调试端口拉起 Codex | 写一个 `launch.bat` / `launch.vbs` 再调用；或先手动开 Codex 再注入 |
| 桌面快捷方式 | TargetPath = `...\codex-chinese.exe` 或 `codex-chinese-tray.exe` | TargetPath = 脚本宿主 exe / `cmd.exe`，参数里塞 `.vbs` / `.bat` |
| 判断成功 | `codex-chinese.exe -verify` **退出码 0**，且输出 `verify: installed=true lang=zh-CN i18n=true` | 看到"完成：Codex 已强制中文"、进程在跑、快捷方式已建、终端没报错 |
| 安装方式 | `scripts/install_windows.ps1`（下发行版 + 校验 SHA256 + 建正确快捷方式） | 自己手搓包装脚本"帮用户简化一下" |
| Codex 来源 | 先跑 `-doctor` 看 `codex:` 行；商店（MSIX）版会自动从注册表定位 | 自己拼 `WindowsApps\OpenAI.Codex_<版本>__<哈希>\app\ChatGPT.exe` 写死路径 |
| 注入收尾 | 等 `-verify` 确认新页面已执行 prelude 再结束 | 注入后立刻退出（脚本随会话撤销，等于没注入） |
| 失败时 | 先按**第 6 节自愈**：采集证据 → 定位根因 → 改代码 → 自证 | 只汇报"不工作"、猜"重启试试"、反复重试、擅自改 Codex 配置 |

**两条铁律**：
1. 快捷方式的 TargetPath 必须是 `codex-chinese*.exe`，**永远不要**是 `wscript.exe`、`cmd.exe` 或 `.vbs` / `.bat`。
2. 唯一成功判据是 `-verify` 退出码 0。程序自己打印的"完成"只是过程提示，**不是判据**。

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
findcodex_windows.go  Windows 专用：从注册表定位商店（MSIX）版 Codex
findcodex_other.go    非 Windows 空实现
detach_windows.go     Windows 进程分离（DETACHED_PROCESS）
detach_unix.go        macOS/Linux 进程分离（Setsid）
console_windows.go    Windows 控制台 UTF-8
console_other.go      非 Windows 空实现
tray/main_windows.go  Windows 托盘（纯 Win32，无 cgo）
tray/main_other.go    非 Windows 占位（提示仅支持 Windows）
test/prelude.test.mjs 注入脚本(prelude)的回归测试（node，无需浏览器）
test/mock-statsig.html 端到端测试用的假 Statsig 页面
build.sh / build.ps1  交叉编译 CLI 六平台 + 托盘两平台
deploy.json           机器可读的构建/产物/校验清单
docs/AI_DEPLOY.md     给 AI 的确定性部署方案（执行入口）
docs/TROUBLESHOOTING.md 典型故障详解（快捷方式陷阱 / 商店版路径 / 黑窗 / 端口打不开）
docs/ARCHITECTURE.md  原理与数据流
scripts/ai_deploy.*   一键部署脚本（AI 可直接运行）
scripts/install_windows.ps1  Windows 安装器（下载发行版 + 建正确快捷方式）
scripts/diagnose_windows.ps1 Windows 环境诊断（找不到 Codex / 端口打不开时先跑）
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
8. **禁止伪启动器**：不得生成 `.vbs` / `.bat` 启动器，也不得创建指向
   `wscript.exe`、`cmd.exe` 的快捷方式。快捷方式的 TargetPath 必须是
   `codex-chinese*.exe`。原因：VBS 不会给 Codex 传 `--remote-debugging-port`，
   注入链路不存在，**必然不生效**。
9. **以 `-verify` 为唯一成功判据**：只有 `codex-chinese.exe -verify` 退出码为 0
   才算部署成功。进程启动、快捷方式存在、终端无报错都不算。
10. **包装函数必须原样返回原值**：prelude 里所有对 Statsig 方法的包装，
   必须是 `var r = orig(...); patch(r); return r;`，**禁止**写成
   `return patch(r)`。历史上这里写成 `return patchValue(r)`，因为 `patchValue`
   没有返回值，`getLayer("72216192")` 就变成了 `undefined`，Codex 拿不到
   i18n 开关，症状是"注入标记在、界面还是英文"。改 prelude 后必须跑
   `node test/prelude.test.mjs`。
11. **改完 prelude 必须跑测试**：`node test/prelude.test.mjs` 必须全绿；
    它会在没有 Codex 的环境里验证补丁行为，是唯一能提前发现第 10 条那类
    逻辑错误的手段。
12. **托盘必须编译成 GUI 子系统**：构建参数必须包含 `-H=windowsgui`。
    漏掉它，产物就是控制台程序，用户一启动就看到一个黑窗（真实事故）。
13. **不要在 `WindowsApps` 上用目录遍历/glob 找 exe**：普通用户对该目录
    **没有列目录权限**，`filepath.Glob` 永远返回空。商店（MSIX）版的定位
    必须走注册表 `HKCU\...\AppModel\Repository\Packages\OpenAI.Codex_*\PackageRootFolder`
    （见 `findcodex_windows.go`），再拼 `app\ChatGPT.exe`。
14. **托盘不能丢弃命令行参数**：`-codex` / `-port` 必须透传给
    `codex-chinese.exe`。历史上托盘直接忽略它们，导致"用户明明指定了路径，
    却报找不到 Codex"。

## 4. 常用命令

```bash
go vet ./...                     # 静态检查（含 tray 包）
node test/prelude.test.mjs       # 注入脚本回归测试（改 prelude 后必跑）
make test                        # 上面两项一起跑
VERSION=v1.3.0 bash build.sh     # CLI 6 平台 + 托盘 2 平台
make release VERSION=v1.3.0      # 同上
./dist/codex-chinese-<os>-<arch> -version   # 版本
./dist/codex-chinese-<os>-<arch> -doctor    # 环境诊断（0=可用, 2=未找到 Codex）
./dist/codex-chinese-tray-windows-amd64.exe # Windows 托盘（GUI 子系统，无黑窗）
./dist/codex-chinese-windows-amd64.exe -verify  # 注入结果校验（0=PASS）
```

托盘会透传 `-codex` / `-port` 给同目录的 `codex-chinese.exe`；
失败细节写入 `%TEMP%\codex-chinese-tray.log`。

Windows 安装器（下载发行版 + 校验 + 建正确的快捷方式）：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts\install_windows.ps1
```

## 5. AI 部署任务（关键）

当用户要求“部署 / 构建 / 安装这个项目”时：

1. 阅读 `docs/AI_DEPLOY.md`。
2. **优先装发行版**（不需要 Go）：Windows 运行
   `scripts/install_windows.ps1`；其他平台下载 Release 资产。
   只有在无网络或需要定制时，才走源码构建
   （`scripts/ai_deploy.sh` / `ai_deploy.ps1`）。
3. 构建路径下核对 JSON：`ok=true`、`artifacts` 数量为 **8**（6 个 CLI + 2 个托盘）。
4. 启动后用 `codex-chinese.exe -verify` 校验，**退出码必须为 0**。
5. 按 5.1 把使用说明讲给用户。

**成功判定**：`-version` 输出以 `codex-chinese ` 开头；8 个产物均存在；`go vet` 退出码 0；
`node test/prelude.test.mjs` 全绿；运行后 `-verify` 退出码 0（`PASS`，且输出形如
`verify: installed=true lang=zh-CN i18n=true`）。缺一不可。
**失败处理**：**转第 6 节自愈**——采集证据、定位根因、改代码、自证。
自愈确实走不通时，按 6.7 的结构汇报；不要猜、不要伪造 PASS、不要只丢一句"失败了"。

`-verify` 的三种失败形态要分清，别混为一谈：

| 输出 | 含义 | 处置 |
|---|---|---|
| `cdp not-ready` (退出码 2) | Codex 没带调试端口启动 / 已退出 | 先完全退出 Codex 再重跑 |
| `installed=false` (退出码 1) | 注入没生效（快捷方式绕过了 exe、或注入后立刻断连） | 检查快捷方式 TargetPath 是否为 `codex-chinese*.exe` |
| `installed=true i18n=false` (退出码 1) | 标记在但补丁没作用到 Layer，界面仍英文 | 说明用的是旧版本二进制，换用 v1.2.0+ |

### 5.1 部署完成后必须告诉用户怎么用（不可省略）

部署成功后，**不要只丢一段 JSON**。必须用用户使用的语言，按下面结构给出可操作说明：

1. **产物在哪**：列出 `dist/` 路径与对应平台的推荐文件。
2. **怎么启动**：
   - Windows 想要常驻：双击 `codex-chinese-tray-windows-amd64.exe`，右键托盘图标选「立即强制中文」。
   - Windows 一次性：运行 `codex-chinese-windows-amd64.exe`。
   - macOS / Linux：`./codex-chinese-darwin-arm64`（或对应平台文件）。
3. **首次使用前置条件**：先**完全退出 Codex（含托盘图标）**。
4. **成功标志**：`-verify` 退出码 0（输出 `PASS`，且有一行
   `verify: installed=true lang=zh-CN i18n=true`）。
   程序打印的「完成：Codex 已强制中文。」只是过程提示，**不能**当作成功依据。
5. **常用参数**：`-codex` 指定路径、`-port` 端口、`-no-launch` 只注入、`-doctor` 诊断。
6. **重要提醒**：每次重启 Codex 后需重新运行（或让托盘版常驻）。
7. **故障排查**：端口超时 → 退出 Codex 重试；macOS 被拦 → `xattr -d com.apple.quarantine`；
   Windows SmartScreen → 更多信息 → 仍要运行。

可直接复用的话术：

> 部署完成 ✅
> 产物在 `dist/`。Windows 推荐双击 `codex-chinese-tray-windows-amd64.exe`（托盘常驻），
> 右键图标选「立即强制中文」；也可以直接运行 `codex-chinese-windows-amd64.exe`。
> 使用前请先完全退出 Codex（含托盘图标）。
> 最后跑一次 `codex-chinese-windows-amd64.exe -verify`，输出 `PASS`
> 且那行显示 `installed=true lang=zh-CN i18n=true` 才算成功。
> 每次重启 Codex 后需要再执行一次；用 `-doctor` 可诊断环境。

## 6. AI 自愈手册（**用户明确要求：坏了自己修**）

用户的原话是"我要让 AI Agents 自己修复"。所以本节不是参考文档，是**作业流程**：
遇到故障 → 按本节定位 → 改代码 → 自证 → 再汇报。**禁止只把报错贴回去。**

### 6.0 什么情况触发自愈

任一命中即进入本节，不要先问用户：

- `-doctor` 退出码 2 / 打印 `codex: not-found`
- `-doctor` 打印 `cdp: not-ready`
- `-verify` 退出码非 0
- 用户描述：黑窗、报找不到 Codex、界面还是英文、托盘点了没反应

### 6.1 第一步：采集证据（**不许靠猜**）

在目标机器上把这三条的输出全部拿到手，再判断：

```bash
./dist/codex-chinese-<os>-<arch> -doctor
./dist/codex-chinese-<os>-<arch> -verify
```

```powershell
# Windows 额外跑，-TryLaunch 会真的带端口启动一次做实测
powershell -NoProfile -ExecutionPolicy Bypass -File scripts\diagnose_windows.ps1 -TryLaunch
```

托盘场景再补一份日志：`%TEMP%\codex-chinese-tray.log`。

### 6.2 决策树：证据 → 原因 → 改哪里

| 证据特征 | 根因 | 改这个文件/函数 |
|---|---|---|
| `codex: not-found`，但 `Get-AppxPackage -Name *Codex*` 能列出包 | 商店（MSIX）版：exe 叫 `ChatGPT.exe`，版本目录随机，且 `WindowsApps` 不可枚举 | `findcodex_windows.go`：注册表键前缀 + `rel` 列表；必要时补 `main.go` 的 `codexCandidates()` |
| `codex: not-found`，Codex 装在别的目录 | 候选表不全 | `main.go` → `codexCandidates()` 加路径 |
| `codex:` 有路径，但 `-verify` 报 `cdp not-ready`（退出码 2） | Codex 没带端口启动：残留实例吞参数 / 该版本忽略开关 | 先按 6.4 处理 |
| 托盘一启动就有黑窗 | 托盘不是 GUI 子系统 | `build.sh` / `build.ps1` 的托盘构建必须带 `-H=windowsgui` |
| 用户传了 `-codex` 仍报找不到 | 托盘把参数丢了 | `tray/main_windows.go` → `parseArgs()` / `withGlobals()` |
| `installed=false` | prelude 没在新文档里执行 | `main.go` → `waitInjected()`，确认 reload 后等到了标记才断连 |
| `installed=true` 但 `i18n=false` | Layer 名变了，或包装器丢了返回值 | `main.go` 的 prelude：`"72216192"` 与 `patchValue` 的 `return r` |
| `-verify` PASS 但界面英文 | `localeOverride` 没写进去 / 语言包未落盘 | `main.go` 的 prelude 末段（`localeOverride` + 单次 reload） |

### 6.3 四类标准修复动作

#### A. 适配新版 Codex（Layer 名变了）

1. 在 Codex 里开 DevTools，搜 `enable_i18n`，找到当前 Layer / DynamicConfig 名。
2. 全量替换 `main.go` 中**所有**出现处（共 5 处，漏一处就会半失效）：
   prelude 的 `wrap()` 判断、`_getLayerImpl` 判断、`patchClient()` 里两次调用、
   以及 `verifyExpr` 里的 `getLayer("...")`。
3. 同步改 `test/prelude.test.mjs`（里面也硬编码了该名字）。
4. 跑 `node test/prelude.test.mjs`，必须 ALL PASS。

#### B. 新增 Codex 安装来源

- 普通安装：在 `main.go` → `codexCandidates()` 里追加，**放在商店候选之前**。
- 商店变体：改 `findcodex_windows.go` 的 `appModelPackagesKey` 与包名前缀
  `openai.codex_`，以及 `rel` 列表（`app\ChatGPT.exe` 等）。
- **绝对不要**新增对 `C:\Program Files\WindowsApps` 的目录遍历或 `filepath.Glob`
  ——普通用户没有列目录权限，写进去也是死代码。

#### C. 托盘 / 构建问题

- 黑窗：确认构建带 `-H=windowsgui`，再用 PE 头验证子系统：
  ```bash
  node -e "const b=require('fs').readFileSync('dist/codex-chinese-tray-windows-amd64.exe');const p=b.readUInt32LE(0x3c);console.log('subsystem='+b.readUInt16LE(p+24+68),'(2=GUI 正确)')"
  ```
- 参数透传：`parseArgs()` 负责宽容解析（GUI 程序不能用 `flag`，未知参数会直接退出），
  `withGlobals()` 负责附加到 CLI 参数。
- 托盘没有控制台，任何失败都要 `logf()` 落盘，否则用户永远看不到原因。

#### D. 注入链路问题

- **脚本随会话撤销**：`Page.addScriptToEvaluateOnNewDocument` 注册的脚本与 CDP 会话
  同生命周期，reload 后立刻断开等于没注入。必须用 `waitInjected()` 轮询到标记再断。
- **包装器必须原样返回**：`var r = orig(...); patch(r); return r;`，
  写成 `return patch(r)` 会把 `Layer` 变成 `undefined`。

### 6.4 最硬的骨头：Codex 拒绝开调试端口

判定（看 `-TryLaunch` 的输出）：

| `cdp_after` | `still_running` | 结论 | 动作 |
|---|---|---|---|
| not-ready | false | 进程秒退 → **单实例锁**把参数转交给已存在实例 | 结束**全部** `ChatGPT.exe` / `Codex.exe` 后重试（商店版常有十几个进程） |
| not-ready | true | 进程活着但不开端口 → 该版本忽略/屏蔽了开关 | 依次试 `--remote-debugging-pipe`、`--inspect=9229`；仍不行则判定不支持 |
| ready | — | 成功 | 继续注入 |

`--user-data-dir` 隔离对 MSIX 无效（已实测），别在这上面浪费时间。

判定为"该版本不支持"时，**不要硬造成功**：如实告诉用户，并说明这意味着
该 Codex 版本无法用 CDP 方案汉化，需要换思路（例如等官方语言设置）。

### 6.5 改完必须自证（缺一不可，顺序照做）

```bash
gofmt -l .                      # 必须为空
go vet ./...
node test/prelude.test.mjs      # 必须 ALL PASS
VERSION=vX.Y.Z bash build.sh    # 8 个产物
./dist/codex-chinese-<os>-<arch> -doctor
./dist/codex-chinese-<os>-<arch> -verify    # 退出码 0
```

改过托盘还要验 PE subsystem = 2（见 6.3-C）。

**自证不通过就不算修好。** 不要用"应该可以了"作为结论。

### 6.6 自愈的权限边界

可以做（不必额外请示）：

- 改源码、文档、脚本、测试；本地构建；本地 `git commit`。
- 在 `dist/` 里写产物。

不能做（除非用户明确要求）：

- 改用户环境、注册表、Codex 安装目录。
- **结束用户正在运行的应用**——先问一句。
- `git push`、打 tag、建 Release。
- 引入第三方依赖、加 UPX、改用 asar preload 之类的替代方案。

### 6.7 自愈失败时怎么写汇报

不要编，按这个结构给：

1. 现象（原始输出，不要转述）。
2. 已采集的证据（三条命令的输出）。
3. 已尝试的修复动作与结果。
4. 你的判断：卡在哪一环、为什么。
5. 需要用户做的最小动作（一条命令 / 一个确认）。

## 7. 发布流程（仅在有授权时）

```bash
git tag v1.1.0
git push origin v1.1.0      # 触发 .github/workflows/release.yml
```

Release 工作流会自动构建 CLI 六平台 + 托盘两平台、生成 `SHA256SUMS` 并创建 GitHub Release。
