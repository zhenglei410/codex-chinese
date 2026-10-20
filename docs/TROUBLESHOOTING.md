# 故障排查

## 典型症状：桌面快捷方式指向 `wscript.exe` + `.vbs`，界面仍是英文

如果你看到类似这样的快捷方式：

```
C:\Windows\System32\wscript.exe "C:\...\launch-cn.vbs"
```

**这是错误部署，必然不生效。** 请按下面处理。

### 为什么一定不生效

本工具的原理是：启动 Codex 时带上 `--remote-debugging-port=9229`，
再通过 CDP 往渲染进程注入补丁。

一个 `.vbs` 启动器通常只做"打开 Codex"这一件事，**不会带调试端口**；
即使它调用了 `codex-chinese.exe`，只要 Codex 已经在运行（或先于注入启动），
调试端口也不会打开。没有 CDP 连接，就没有注入，界面当然还是英文。

另外，`wscript.exe` 启动的脚本不显示任何输出，失败时你完全看不到原因。

### 正确做法

1. **删掉那个快捷方式**（右键 → 删除）。
2. 用官方安装器（推荐）：

   ```powershell
   powershell -NoProfile -ExecutionPolicy Bypass -File scripts\install_windows.ps1
   ```

   它会下载官方 Release、校验 SHA256，并创建一个**直接指向
   `codex-chinese-tray.exe`** 的快捷方式（不是 wscript）。

   离线时：`... -AssetDir .\dist`。
3. **完全退出 Codex**（含托盘图标）。
4. 双击桌面的「Codex 中文」，右键托盘图标 →「立即强制中文」。
5. 验证：

   ```powershell
   & "$env:LOCALAPPDATA\Programs\codex-chinese\codex-chinese.exe" -verify
   ```

   期望输出 `PASS`（退出码 0）。

### 如何判断一个快捷方式是否正确

右键快捷方式 → 属性 → 「目标」必须以 `codex-chinese.exe` 或
`codex-chinese-tray.exe` 结尾，**不能**是 `wscript.exe` / `cmd.exe` / `.bat` / `.vbs`。

---

## `-verify` 的各种结果

| 输出 | 退出码 | 含义 | 处理 |
|---|---|---|---|
| `PASS` | 0 | 已注入、语言为中文、`enable_i18n=true` | 完成 |
| `FAIL: 未检测到注入标记或语言不是中文` | 1 | Codex 在跑，但没被注入 | 确认是用本工具启动的 Codex；`-verify` 前不要重启 Codex |
| `FAIL: enable_i18n 仍为 false，补丁未覆盖 Statsig 图层` | 1 | 标记在，但开关没改到 | 先升级到 v1.2.0+（见下节）；仍失败再按 ARCHITECTURE 第 6 节重新定位 Layer 名 |
| `FAIL: Codex 未以调试端口启动，或已完全退出` | 2 | 没有 CDP 端口 | 完全退出 Codex，用 CLI/托盘重新启动 |

## 商店（MSIX）版 Codex：报「找不到 Codex」

**症状**：托盘右键「立即强制中文」弹出「强制中文失败 / 找不到 Codex」；
`-doctor` 显示 `codex: not-found`。

**原因**：微软商店版 Codex 和传统安装版差别很大——

| 项目 | 传统安装版 | 商店（MSIX）版 |
|---|---|---|
| 可执行文件名 | `Codex.exe` | **`ChatGPT.exe`** |
| 安装位置 | `%LOCALAPPDATA%\Programs\Codex\` | `C:\Program Files\WindowsApps\OpenAI.Codex_<版本>_<架构>__<哈希>\app\` |
| 版本目录 | 固定 | **随机，每次更新都变** |
| 应用执行别名 | 有 | **通常没有** |

而且 `C:\Program Files\WindowsApps` 对普通用户**不允许列目录**，
所以"扫一遍目录找 exe"这种写法在这里必然失败——只有已知完整路径才能读取和执行。

**v1.3.0 起的做法**：从注册表读取安装目录，不依赖列目录权限：

```
HKCU\Software\Classes\Local Settings\Software\Microsoft\Windows\CurrentVersion\
  AppModel\Repository\Packages\OpenAI.Codex_<版本>_<架构>__<哈希>\PackageRootFolder
```

拿到根目录后拼 `app\ChatGPT.exe` 即为真实路径。`-doctor` 会直接打印它：

```
codex: C:\Program Files\WindowsApps\OpenAI.Codex_26.1013.4894.0_x64__2p2nqsd0c76g0\app\ChatGPT.exe
```

**仍然找不到时的三种指定方式**（优先级从高到低）：

```powershell
# 1. 命令行参数（托盘也支持，v1.3.0 起会透传给 CLI）
codex-chinese-windows-amd64.exe -codex "C:\Program Files\WindowsApps\OpenAI.Codex_<版本>_x64__<哈希>\app\ChatGPT.exe"

# 2. 环境变量
$env:CODEX_CHINESE_CODEX = "C:\Program Files\WindowsApps\OpenAI.Codex_<版本>_x64__<哈希>\app\ChatGPT.exe"

# 3. 直接给安装目录，程序会自己找里面的 ChatGPT.exe / Codex.exe
codex-chinese-windows-amd64.exe -codex "C:\Program Files\WindowsApps\OpenAI.Codex_<版本>_x64__<哈希>"
```

> 版本号可以从 `Get-AppxPackage -Name *Codex*` 的 `InstallLocation` 字段拿到。

## 托盘启动后弹出一个黑色窗口（v1.3.0 已修）

黑窗是**托盘自己**的控制台窗口，不是 Codex。原因是托盘被编译成了
控制台子系统程序（PE subsystem = 3）。

v1.3.0 起托盘用 `-H=windowsgui` 构建（PE subsystem = 2），启动不再有黑窗。
如果你用的是旧版，换 v1.3.0+ 的 `codex-chinese-tray-*.exe` 即可。

自检方法（PowerShell）：

```powershell
$b = [IO.File]::ReadAllBytes(".\codex-chinese-tray-windows-amd64.exe")
$pe = [BitConverter]::ToUInt32($b, 0x3c)
[BitConverter]::ToUInt16($b, $pe + 24 + 68)   # 2 = GUI（正常），3 = 控制台（会有黑窗）
```

## `cdp: not-ready`，但 Codex 明明在运行

**最常见原因：Codex 没有完全退出。**

Electron 应用是单实例的：当已有 Codex 在跑时，你再带
`--remote-debugging-port` 启动一个新进程，新进程只会把请求转交给已有实例
然后**自己退出**，调试端口根本不会打开。

商店版尤其容易踩坑——任务管理器里通常有**十几个 `ChatGPT.exe` 进程**
（主进程、GPU 进程、渲染进程等），关掉主窗口不等于全部退出。

正确操作：

1. 关闭 Codex 窗口。
2. 任务管理器 → 详细信息 → 结束所有 `ChatGPT.exe`（以及 `Codex.exe`）。
3. 再运行 `codex-chinese.exe`，或者托盘「立即强制中文」。
4. `-verify` 确认。

如果确认已经全部退出、`-doctor` 也能打印出 codex 路径，但端口仍打不开，
说明该版本的 Codex 不接受命令行开关，请跑下面这个诊断脚本把结果贴回来。

## 一键诊断（推荐先跑这个）

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts\diagnose_windows.ps1
# 想直接验证"能不能带端口启动"：
powershell -NoProfile -ExecutionPolicy Bypass -File scripts\diagnose_windows.ps1 -TryLaunch
```

它会一次性输出：系统信息、商店包与真实路径、别名、正在运行的进程、
调试端口状态，以及（加 `-TryLaunch` 时）真实启动测试的结果。
把整段输出贴到 issue 里即可定位。

## `installed=true` 但 `i18n=false`（v1.2.0 已修）

这个症状的含义是"**补丁挂上了，但界面还是英文**"。历史上它有两个原因：

### 原因一（v1.2.0 修复）：包装函数吃掉了返回值

prelude 会把 `client.getLayer` 包装一层来打补丁。早期写法是：

```js
return n === "72216192" ? patchValue(r) : r;   // ❌
```

而 `patchValue()` 没有 `return`，于是这一句返回 `undefined`。Codex 调用
`getLayer("72216192")` 拿到 `undefined`，i18n 初始化直接落空，界面保持英文。

正确写法是**先打补丁、再原样返回原对象**：

```js
var r = orig(n, o);
if (n === "72216192") patchValue(r);
return r;                                       // ✅
```

如果你用的是 v1.1.0 或更早的二进制，**升级即可解决**。

### 原因二（罕见）：Codex 升级后 Layer 名变了

当前 Layer / DynamicConfig 名是 `"72216192"`。Codex 换版本后如果这个 ID 变了，
补丁就打不到正确对象。重新定位方法见 `docs/ARCHITECTURE.md` 第 6 节。

### 如何自证

不用打开 Codex 就能验证补丁逻辑是否正确：

```bash
node test/prelude.test.mjs      # 期望 ALL PASS (11 项)
```

它会从 `main.go` 抽出 prelude，在一个假 Statsig 环境里跑，专门断言
`getLayer("72216192")` **必须返回对象而不是 undefined**、且 `enable_i18n` 变为 `true`。
CI 每次提交都会跑这个测试。

## 其他常见问题

| 现象 | 处理 |
|---|---|
| 等待调试端口超时 | Codex 还在后台（含托盘），完全退出后重试 |
| 找不到 Codex | `-codex "完整路径"` |
| macOS 无法打开 | `xattr -d com.apple.quarantine <文件>` |
| Windows SmartScreen 拦截 | 更多信息 → 仍要运行 |
| 每次重启 Codex 都变回英文 | 正常：注入是运行时的，重启后需再执行一次；让托盘版常驻即可 |
