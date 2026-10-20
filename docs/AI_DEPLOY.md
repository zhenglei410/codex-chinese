# AI 部署方案（给 Codex / 其他代理执行）

> 目标：让一个 AI 代理在**无需人类逐步指导**的情况下，把本项目从源码构建成可运行产物，并自证成功。
> 所有命令均可复制执行；步骤幂等，可重复运行。

---

## 0.0 正确 vs 错误（**先读这一节，否则极易做出"看起来成功"的错误部署**）

本工具的工作链条只有一条，断了任何一环都**不会报错**，只是界面保持英文：

```
codex-chinese.exe 启动 Codex（带 --remote-debugging-port）
        └─► 连上 CDP → 注入 prelude → reload → 等新页面执行完脚本 → 断开
                └─► -verify 退出码 0  ← 唯一可信的成功证据
```

| 环节 | ✅ 这样做才对 | ❌ 这样做一定失败 |
|---|---|---|
| 启动 | 直接运行 `codex-chinese*.exe` | 写 `.vbs` / `.bat` 启动器；先手动开 Codex 再注入 |
| 快捷方式 | 指向 `codex-chinese.exe` / `codex-chinese-tray.exe` | 指向 `wscript.exe` / `cmd.exe`，参数里放脚本文件 |
| 安装 | 跑 `scripts/install_windows.ps1` | 自己拼一套"更简单"的包装来替代它 |
| 判成功 | `-verify` 退出码 0 且 `i18n=true` | 看终端有没有打印"完成"、看进程在不在、看快捷方式建没建 |
| 出错时 | 原样贴 `-verify` 输出 + 查第 8 节 | 让用户"重启试试"、反复重跑、改 Codex 配置 |

> **如果你正打算写一个包装脚本或启动器"帮用户简化操作"——立刻停下。**
> 历史上就是这样产出了
> `wscript.exe "...\launch-cn.vbs"` 的快捷方式：VBS 不给 Codex 传调试端口，
> 注入链路根本不存在，用户看到的是"部署成功但界面还是英文"。

---

## 0. 输入与产出

- 输入：本仓库源码（已 `git clone` 或解压到本地）。
- 产出：`dist/` 下 6 个平台的二进制，以及一段 JSON 报告。
- 预计耗时：< 2 分钟（不含下载）。

---

## 0.5 推荐路径：直接安装发行版（无需 Go）

适用于绝大多数情况。**不要把这一步替换成“写一个 .vbs / .bat 启动器”**——
VBS 不会给 Codex 传 `--remote-debugging-port`，注入不可能生效。

Windows：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts\install_windows.ps1
# 离线：-AssetDir .\dist
```

安装器会：下载对应架构的 CLI + 托盘 → 校验 SHA256 → 安装到
`%LOCALAPPDATA%\Programs\codex-chinese` → 创建**直接指向 exe** 的快捷方式。

macOS / Linux：从 Release 下载对应二进制，`chmod +x` 后直接运行。

> 只有在无网络、或需要改代码时，才走下面的源码构建。

---

## 1. 环境检测

### 1.1 检查 Go

```bash
go version
```

判定：
- 输出包含 `go1.21` 及以上 → 跳过第 2 步。
- 命令不存在或版本低于 1.21 → 执行第 2 步。

### 1.2 检查工作目录

```bash
pwd
ls go.mod main.go
```

必须在包含 `go.mod` 的仓库根目录执行后续命令。

---

## 2. 安装 Go（仅在缺失或过旧时）

### 2.1 取最新版本号

```bash
curl -s "https://go.dev/VERSION?m=text" | head -1
# 例：go1.27.2
```

### 2.2 Linux

```bash
GO_VAR=$(curl -s "https://go.dev/VERSION?m=text" | head -1)
curl -fsSL "https://go.dev/dl/${GO_VAR}.linux-amd64.tar.gz" -o /tmp/go.tgz
sudo rm -rf /usr/local/go && sudo tar -C /usr/local -xzf /tmp/go.tgz
export PATH=$PATH:/usr/local/go/bin
go version
```

### 2.3 macOS

```bash
GO_VAR=$(curl -s "https://go.dev/VERSION?m=text" | head -1)
# Apple Silicon 用 darwin-arm64；Intel 用 darwin-amd64
curl -fsSL "https://go.dev/dl/${GO_VAR}.darwin-arm64.tar.gz" -o /tmp/go.tgz
sudo rm -rf /usr/local/go && sudo tar -C /usr/local -xzf /tmp/go.tgz
export PATH=$PATH:/usr/local/go/bin
go version
```

### 2.4 Windows（PowerShell）

```powershell
$go = (Invoke-RestMethod "https://go.dev/VERSION?m=text").Split("`n")[0]
$zip = "$env:TEMP\$go.windows-amd64.zip"
Invoke-WebRequest "https://go.dev/dl/$go.windows-amd64.zip" -OutFile $zip
Expand-Archive $zip -DestinationPath "C:\" -Force
$env:Path = "C:\go\bin;$env:Path"
go version
```

> 若公司网络无法直连 `go.dev`，改用镜像（如 `https://golang.google.cn/dl/`），仅在必要时使用。

---

## 3. 构建

### 3.1 一键构建（推荐）

```bash
VERSION=v1.0.0 bash build.sh
```

Windows：

```powershell
$env:VERSION="v1.0.0"; .\build.ps1
```

### 3.2 等价手动命令（单平台）

```bash
CGO_ENABLED=0 go build -trimpath -buildvcs=false \
  -ldflags "-s -w -X main.version=v1.0.0" \
  -o dist/codex-chinese .
```

**必须保留**：`CGO_ENABLED=0`、`-trimpath`、`-buildvcs=false`、`-ldflags "-s -w"`。
原因分别是：静态链接、去除本地路径、去除版本库信息、去除符号表以减小体积。

---

## 4. 校验（必须全部通过）

### 4.1 静态检查

```bash
go vet ./...
```

期望：退出码 `0`，无输出。

### 4.1.5 注入脚本回归测试

```bash
node test/prelude.test.mjs
```

期望：末行 `ALL PASS (11 项)`，退出码 `0`。

`prelude` 是内联在 `main.go` 里的 JS 字符串，`go vet` 完全看不到它的逻辑错误。
这一步在没有 Codex 的机器上也能跑，专门保护"补丁返回 undefined 导致汉化静默失效"
这类只有运行时才暴露的问题。**改动 `main.go` 里的 prelude 后必须重跑。**

### 4.2 产物存在性

```bash
ls -l dist/
```

期望恰好 8 个文件：

| 文件 | 目标 | 形态 |
|---|---|---|
| `codex-chinese-windows-amd64.exe` | Windows x64 | CLI |
| `codex-chinese-windows-arm64.exe` | Windows ARM64 | CLI |
| `codex-chinese-darwin-amd64` | macOS Intel | CLI |
| `codex-chinese-darwin-arm64` | macOS Apple Silicon | CLI |
| `codex-chinese-linux-amd64` | Linux x64 | CLI |
| `codex-chinese-linux-arm64` | Linux ARM64 | CLI |
| `codex-chinese-tray-windows-amd64.exe` | Windows x64 | 托盘 |
| `codex-chinese-tray-windows-arm64.exe` | Windows ARM64 | 托盘 |

体积应在 3.2–3.8 MB 之间；若超过 6 MB，视为回归。

### 4.3 版本自证

```bash
# 把 <os>/<arch> 换成本机平台
./dist/codex-chinese-linux-amd64 -version
```

期望输出以 `codex-chinese ` 开头，例如：

```
codex-chinese v1.0.0 linux/amd64
```

### 4.4 环境诊断

```bash
./dist/codex-chinese-linux-amd64 -doctor
```

期望输出包含 `version:`、`platform:`、`codex:`、`cdp:` 四行。

退出码含义：

| 退出码 | 含义 |
|---|---|
| `0` | 找到 Codex，或检测到可注入的 CDP 端口 |
| `2` | 既没找到 Codex，也没有调试端口（在 CI/无 Codex 环境下属正常） |

### 4.5 注入结果校验（运行后必做）

```bash
./dist/codex-chinese-<os>-<arch> -verify
```

期望输出形如：

```
verify: installed=true lang=zh-CN i18n=true
PASS
```

| 输出 | 退出码 | 含义 | 处置 |
|---|---|---|---|
| `PASS` | 0 | 已注入、语言中文、`enable_i18n=true` | 完成 |
| `installed=false` | 1 | Codex 在跑但没被注入 | 检查快捷方式是否直指 `codex-chinese*.exe` |
| `installed=true i18n=false` | 1 | 补丁没打到 Statsig 图层，界面仍英文 | 换用 v1.2.0+ 二进制 |
| `cdp not-ready` | 2 | 没有 CDP 端口 | 完全退出 Codex 后重启 |

**不能**用“进程启动了”“快捷方式存在”“终端没报错”代替这一步。

---

## 5. 运行（可选，需用户授权）

```bash
./dist/codex-chinese-linux-amd64
```

前置条件：**用户已完全退出 Codex（含托盘）**。
程序会：启动 Codex（带 `--remote-debugging-port`）→ 注入 prelude → 刷新一次。

预期输出：

```
== 强制 Codex 中文界面 ==
注入目标: app://...
已注册注入脚本（对后续所有页面生效）。
注入已生效（页面已加载）。
完成：Codex 已强制中文。
```

其中 `注入已生效（页面已加载）` 是程序等到新页面真正执行完 prelude 才打印的。
如果这里变成 `警告：页面中未检测到注入标记，请运行 -verify 复查。`，
说明注入失败，**不要**继续往下报告成功，直接跑 `-verify` 并按第 8 节排查。

程序末尾的 `完成：Codex 已强制中文。` 只是流程走完的提示，
**真正的成功判据只有 `-verify` 退出码 0**（见 4.5）。

### 5.1 Windows 托盘版

```powershell
.\dist\codex-chinese-tray-windows-amd64.exe
```

常驻托盘，右键图标：**立即强制中文 / 环境诊断 / 退出**。
托盘会把实际注入交给同目录的 `codex-chinese.exe`，因此发布/分发时两个 exe 需放在同一目录。
（开发目录下会自动回退查找 `dist\codex-chinese-windows-amd64.exe`。）

---

## 6. 输出 JSON 报告（交给调用方）

构建完成后，输出以下结构（字段名固定）：

```json
{
  "ok": true,
  "version": "v1.0.0",
  "platform": "linux/amd64",
  "go_version": "go1.27.2",
  "vet_exit": 0,
  "artifacts": [
    { "target": "windows/amd64", "path": "dist/codex-chinese-windows-amd64.exe", "bytes": 3674112 },
    { "target": "windows/arm64", "path": "dist/codex-chinese-windows-arm64.exe", "bytes": 3377152 },
    { "target": "darwin/amd64",  "path": "dist/codex-chinese-darwin-amd64",      "bytes": 3582096 },
    { "target": "darwin/arm64",  "path": "dist/codex-chinese-darwin-arm64",      "bytes": 3383138 },
    { "target": "linux/amd64",   "path": "dist/codex-chinese-linux-amd64",       "bytes": 3535008 },
    { "target": "linux/arm64",   "path": "dist/codex-chinese-linux-arm64",       "bytes": 3408032 },
    { "target": "windows/amd64", "path": "dist/codex-chinese-tray-windows-amd64.exe", "bytes": 3674112, "variant": "tray" },
    { "target": "windows/arm64", "path": "dist/codex-chinese-tray-windows-arm64.exe", "bytes": 3377152, "variant": "tray" }
  ],
  "doctor_exit": 0,
  "notes": "doctor_exit=2 表示本机未安装 Codex，属正常"
}
```

`scripts/ai_deploy.sh` 与 `scripts/ai_deploy.ps1` 会自动生成这份报告。

---

## 7. 向用户交付使用说明（必须执行）

构建与校验完成后，**必须**用用户使用的语言，把下面这些信息告诉用户。
不要只返回 JSON 或产物路径。

必须覆盖的 7 点：

1. **产物位置**：`dist/`，并指出当前平台推荐用哪个文件。
2. **启动方式**：
   - Windows 常驻：双击 `codex-chinese-tray-windows-amd64.exe`，右键托盘图标 →「立即强制中文」。
   - Windows 一次性：运行 `codex-chinese-windows-amd64.exe`。
   - macOS / Linux：`./codex-chinese-darwin-arm64`（或对应平台）。
3. **首次前置**：先完全退出 Codex（含托盘图标）。
4. **成功标志**：`-verify` 退出码 0（输出 `PASS`，且有一行
   `verify: installed=true lang=zh-CN i18n=true`）。
   程序打印的「完成：Codex 已强制中文。」只是过程提示，**不是**成功依据。
5. **常用参数**：`-codex` / `-port` / `-no-launch` / `-doctor`。
6. **重要提醒**：每次重启 Codex 后需重新执行（或让托盘版常驻）。
7. **故障排查**：端口超时 → 退出 Codex 重试；macOS 被拦 → `xattr -d com.apple.quarantine`；
   Windows SmartScreen → 更多信息 → 仍要运行。

推荐话术（可直接发给用户）：

> 部署完成 ✅
> 产物在 `dist/`。Windows 推荐双击 `codex-chinese-tray-windows-amd64.exe`（托盘常驻），
> 右键图标选「立即强制中文」；也可以直接运行 `codex-chinese-windows-amd64.exe`。
> 使用前请先完全退出 Codex（含托盘图标）。
> 最后跑 `codex-chinese-windows-amd64.exe -verify`，输出 `PASS` 且那行显示
> `installed=true lang=zh-CN i18n=true` 才算成功。
> 每次重启 Codex 后需要再执行一次；用 `-doctor` 可诊断环境。

---

## 8. 故障对照表

| 现象 | 原因 | 处理 |
|---|---|---|
| `go: command not found` | 未安装 Go 或 PATH 未生效 | 执行第 2 步，并重新 `export PATH` |
| `go.mod requires go >= 1.21` | Go 版本过旧 | 升级到 1.21+ |
| `cannot find main module` | 不在仓库根目录 | `cd` 到含 `go.mod` 的目录 |
| 交叉编译报 `cgo` 相关错误 | 未设 `CGO_ENABLED=0` | 显式设置后重试 |
| 产物体积 > 6 MB | 误引入 `net/http`/`crypto/tls` 或第三方库 | 检查 `go.mod` 与 import |
| `-doctor` 退出码 2 | 本机无 Codex | 正常；换到安装了 Codex 的机器再运行 |
| `-doctor` 显示 `codex: not-found`，但 Codex 确实装着 | 商店（MSIX）版：exe 叫 `ChatGPT.exe`，且 `WindowsApps` 不可枚举 | 用 v1.3.0+（自动从注册表定位），或 `-codex` 显式指定 |
| 托盘启动后弹出黑色窗口 | 托盘被编译成控制台子系统 | 用 v1.3.0+；自建构建必须带 `-H=windowsgui` |
| 运行时报“等待调试端口超时” | Codex 已在运行且未开调试端口（Electron 单实例会吞掉新进程的参数） | 任务管理器结束全部 `ChatGPT.exe` / `Codex.exe` 后重试 |

上表仍无法定位时，跑一键诊断，并把整段输出贴回来：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts\diagnose_windows.ps1
# 加 -TryLaunch 会真的带调试端口启动一次，验证该版本是否接受命令行开关
```

---

## 9. 禁止事项

- 不要执行 `git push`、创建 Release、修改远程仓库（除非用户明确授权）。
- 不要修改 `main.go` 里的 prelude，除非用户要求适配新版 Codex。
- 不要引入第三方 Go 模块。
- 不要用 UPX 压缩产物。
- 不要在未退出 Codex 的情况下反复重试运行（会持续超时）。
- 不要只分发托盘 exe 而漏掉同目录的 `codex-chinese.exe`（托盘依赖它完成注入）。
- 不要省略第 7 步：部署完成后必须把使用说明交给用户。
- 不要生成 `.vbs` / `.bat` 启动器，不要创建指向 `wscript.exe` / `cmd.exe` 的快捷方式。
  快捷方式必须直接指向 `codex-chinese*.exe`。
- 不要用“进程存在 / 快捷方式存在 / 没有报错”作为成功判据，必须跑 `-verify` 且退出码为 0。
