# 原理与架构

## 1. 问题

Codex 桌面应用内置了 `zh-CN` 语言包，但"是否启用"由 Statsig 特性开关控制：

- Layer / DynamicConfig 名：`72216192`
- 关键键：`enable_i18n`（是否启用 i18n）、`locale_source`（语言来源）

当 Statsig 或 VPN 不可达时，这两个键取不到服务端下发的值，应用按默认（false）处理，
于是即使设置里选了中文，界面仍回退英文。

**结论：只改配置文件解决不了问题**——配置文件能指定语言，但关不掉"开关取不到就回退"的逻辑。
必须在运行时让那个开关返回 `true`。

## 2. 为什么必须是"主世界注入"

| 方案 | 结果 | 原因 |
|---|---|---|
| 改配置文件 / 设置项 | ❌ | 无法覆盖 Statsig 的运行时回退 |
| 往 `app.asar` 塞 preload | ❌ | `contextIsolation` 下 preload 在隔离世界，访问不到页面的 `window.__STATSIG__` |
| 浏览器扩展 content script | ❌ | 打包 Electron 默认不加载外部扩展；且默认在隔离世界 |
| **CDP `Page.addScriptToEvaluateOnNewDocument`** | ✅ | 在**主世界**、**页面脚本之前**执行，能拦截 Statsig 初始化 |

## 3. 数据流

```
┌──────────────┐   1. spawn --remote-debugging-port=9229
│ codex-chinese│ ────────────────────────────────────────► Codex (Electron)
│   (Go)       │
│              │   2. GET /json  (HTTP, 127.0.0.1)
│              │ ◄────────────────────────────────────────  [{type:"page", webSocketDebuggerUrl}]
│              │
│              │   3. WebSocket 连接 webSocketDebuggerUrl
│              │   4. Page.enable
│              │   5. Page.addScriptToEvaluateOnNewDocument { source: prelude }
│              │   6. Page.reload
└──────────────┘
                              │
                              ▼ 页面重新加载，prelude 先于应用脚本执行
                   ┌─────────────────────────────────────┐
                   │ prelude (主世界)                     │
                   │ ① navigator.language = "zh-CN"       │
                   │ ② 拦截 window.__STATSIG__ 赋值        │
                   │ ③ wrap getLayer/getDynamicConfig     │
                   │    → "72216192" 返回                  │
                   │      enable_i18n: true                │
                   │      locale_source: "SYSTEM"          │
                   │ ④ localeOverride = "zh-CN" → reload   │
                   └─────────────────────────────────────┘
```

## 4. prelude 的三个关键点

### 4.1 拦截 Statsig 赋值

Statsig 初始化时会执行 `window.__STATSIG__ = ...`。prelude 用
`Object.defineProperty(window, "__STATSIG__", { set })` 抢在它之前装好 setter，
一旦赋值就立刻给客户端打补丁。

若 Statsig 在 prelude 之前就已存在，则直接补一次；同时每 50ms 轮询、最多 5 秒，
覆盖"加载慢"的情况。

### 4.2 同时 patch Layer 和 DynamicConfig

应用可能通过 `useLayer("72216192").get("enable_i18n", false)` 读取，
而 Layer 的 memo 缓存键恒为 `NoValues`，只补 DynamicConfig 不生效。
因此两条通道都要包：

- `client.getLayer` / `client._getLayerImpl`
- `client.getDynamicConfig`

并同时改写 `__value` 与 `value`，防止应用绕过 `get` 直接读字段。

### 4.3 写官方设置并只刷新一次

通过应用自身的 state API 把 `localeOverride` 设为 `zh-CN`，然后 `location.reload()`。
用 `sessionStorage` 打标记，避免"设置未生效 → 再刷新"的死循环。

这就是"需重启 Codex 才能完整生效"的来源——刷新后语言才真正落下来。

## 5. 体积工程

3.5 MB 的来源：

| 措施 | 作用 |
|---|---|
| `CGO_ENABLED=0` | 静态链接，Linux 不依赖 glibc，交叉编译无需工具链 |
| 不用 `net/http` | 手写 localhost HTTP（按 Content-Length/chunked 精确读取） |
| 不用 `crypto/tls` | 只连本机明文，不需要 TLS |
| 手写 WebSocket | 握手 + 掩码帧 + ping/pong，约 250 行，替代第三方库 |
| `-trimpath -buildvcs=false` | 去掉本地路径与 VCS 信息 |
| `-ldflags "-s -w"` | 去掉符号表与 DWARF |

未使用 UPX：未签名压缩 exe 容易被杀软误报，对开源分发是负收益。

## 6. 适配新版 Codex

若失效，按顺序排查：

1. 打开 Codex DevTools，确认 `window.__STATSIG__` 是否存在。
2. 找到 i18n 对应的 Layer / DynamicConfig 名（搜索 `enable_i18n`）。
3. 更新 `main.go` 中 `prelude` 里的 `"72216192"`。
4. 重新构建并验证。

## 7. 安全边界

- 只连 `127.0.0.1`，不发起外部请求。
- 不改 Codex 安装文件、不写注册表、不常驻。
- 需要 Codex 以 `--remote-debugging-port` 启动；该端口仅回环可达。
