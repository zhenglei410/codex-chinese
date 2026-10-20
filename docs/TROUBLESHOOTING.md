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
