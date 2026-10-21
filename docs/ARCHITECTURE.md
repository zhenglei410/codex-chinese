# 架构与原理

## 1. 问题

Codex 桌面应用内置中文语言包，但 i18n 由 Statsig 配置控制：

```text
Layer / DynamicConfig: 72216192
enable_i18n: true
locale_source: SYSTEM
```

远程配置不可用时，应用可能得到 `enable_i18n=false` 并回退英文。
只改普通设置不足以覆盖运行时回退，因此需要注入渲染进程主世界。

## 2. 总体数据流

```text
codex-chinese.exe
  │
  ├─ 全局启动锁
  ├─ 解析 Codex 路径和 MSIX 包身份
  ├─ 复用现有 CDP 实例，或选择稳定端口
  ├─ 原生 AUMID 激活 Codex
  │      --remote-debugging-port=<port>
  │      --remote-allow-origins=http://127.0.0.1:<port>
  │
  ├─ 等待 /json 页面目标
  ├─ 启动 detached supervisor
  └─ 等待 -verify 成功

supervisor
  │
  ├─ 全局 supervisor 锁
  ├─ 连接 app://-/index.html
  ├─ Page.addScriptToEvaluateOnNewDocument(prelude)
  ├─ Runtime.evaluate(prelude)   // 当前文档立即生效
  ├─ 周期健康检查
  └─ 页面/连接变化后重连
```

不强制刷新刚启动的 Electron 页面，避免中断 bootstrap。

## 3. 启动层

### 3.1 非 MSIX

按普通独立进程启动，并传入调试参数。

### 3.2 MSIX / Microsoft Store

商店版不能直接 `CreateProcess`：

- 直接执行可能返回 `Access is denied`；
- 绕过包身份启动的进程可能因缺少包身份退出。

Windows 实现从当前用户 AppModel 注册表读取
`PackageRootFolder` 和包全名，再解析 `AppxManifest.xml`：

```text
PackageFullName:
  OpenAI.Codex_<version>_<arch>_<resource>_<publisher>

PackageFamilyName:
  OpenAI.Codex_<publisher>

AUMID:
  <PackageFamilyName>!<Application Id>
```

随后通过 `IApplicationActivationManager::ActivateApplication` 启动。

## 4. 端口和并发控制

`v2.0.6` 使用三层约束避免“启动—关闭”循环：

1. `CodexChineseLauncher` 全局互斥锁：同一时间只有一个启动流程。
2. `CodexChineseSupervisor` 全局互斥锁：跨端口只允许一个 supervisor。
3. `choosePort()` 顺序：
   - 先复用当前任何受控 Codex CDP 端点；
   - 再使用上次保存的端口；
   - 最后才选择 `9229-9258` 中新的空闲端口。

这会避免以下事故：

```text
旧实例在 9230 正常注入
↓
新任务错误选择空闲的 9229
↓
新任务杀掉 9230 的 Codex
↓
9230 supervisor 又把实例重启
↓
两个 supervisor 持续互相覆盖
```

## 5. 注入 prelude

prelude 在页面主世界执行，主要步骤：

1. 设置 `navigator.language` / `navigator.languages`。
2. 拦截 `window.__STATSIG__` 赋值。
3. 包装 `getLayer`、`_getLayerImpl`、`getDynamicConfig`。
4. 对 `72216192` 强制：

```js
enable_i18n = true
locale_source = "SYSTEM"
```

5. 通过 Codex state API 将 `localeOverride` 设置为 `zh-CN`。
6. API 未就绪时按递增间隔重试。

包装器必须返回原始对象：

```js
var result = original(name, options);
patch(result);
return result;
```

不能把 `patch(result)` 的返回值作为函数返回值。

## 6. 长驻 supervisor

`Page.addScriptToEvaluateOnNewDocument` 注册与 CDP 会话绑定。启动进程退出后，
如果会话也关闭，后续页面导航就会失去补丁。

因此 CLI 启动一个分离的 supervisor：

- 保持 WebSocket；
- 每 2 秒检查注入状态；
- 连续失败后关闭旧连接并重新连接；
- Codex 退出后按模式决定继续守护还是退出。

托盘模式下 supervisor 读取 `-guard-pid`，托盘退出时 supervisor 同时退出。

## 7. 托盘守护

托盘流程：

```text
启动
↓
6 秒等待 Codex 自身登录启动稳定
↓
调用 CLI
↓
复用受控实例或重启一次
↓
创建带 guard-pid 的 supervisor
```

同一托盘只允许一个实例。托盘内也使用原子状态阻止重复执行接管流程。

## 8. 自动启动

Windows 安装器写入：

```text
HKCU\Software\Microsoft\Windows\CurrentVersion\Run
CodexChinese = "<install-dir>\codex-chinese-tray.exe"
```

只影响当前用户，不需要管理员权限。

## 9. 验证

`-verify` 读取：

```json
{
  "installed": true,
  "lang": "zh-CN",
  "i18n": "true",
  "htmlLang": "zh-CN"
}
```

成功条件：

```text
installed=true
lang 以 zh 开头
i18n != false
退出码 0
```

## 10. 安全边界

- CDP 只绑定 `127.0.0.1`。
- 不修改 Codex 安装目录。
- 不读取或上传认证信息。
- 不请求管理员权限。
- 不引入第三方 Go 依赖。
- 不使用 UPX。
