# 部署与使用（面向人类）

## 一、直接使用发行版（推荐）

1. 打开 [Releases](https://github.com/zhenglei410/codex-chinese/releases)，下载对应平台文件。
2. （可选）校验完整性：

   ```bash
   # macOS / Linux
   shasum -a 256 codex-chinese-*
   # Windows
   certutil -hashfile codex-chinese-windows-amd64.exe SHA256
   ```

   与 Release 里的 `SHA256SUMS` 对比。

3. 赋予执行权限（macOS / Linux）：

   ```bash
   chmod +x codex-chinese-darwin-arm64
   # macOS 若被 Gatekeeper 拦截：
   xattr -d com.apple.quarantine ./codex-chinese-darwin-arm64
   ```

4. **完全退出 Codex（含托盘图标）**。
5. 运行程序，然后用 `-verify` 校验，**退出码 0 才算成功**：

   ```bash
   ./codex-chinese-darwin-arm64 -verify      # 期望 PASS
   ```

   注意：程序打印「完成：Codex 已强制中文」只是过程提示，不能当作成功依据。

## 二、从源码构建

```bash
git clone https://github.com/zhenglei410/codex-chinese.git
cd codex-chinese
VERSION=v1.0.0 bash build.sh
```

Windows：

```powershell
git clone https://github.com/zhenglei410/codex-chinese.git
cd codex-chinese
$env:VERSION="v1.0.0"; .\build.ps1
```

## 三、开机自启（可选）

程序本身不常驻。若希望每次登录后自动强制中文，可自行创建启动项：

**Windows**：把快捷方式放进
`%APPDATA%\Microsoft\Windows\Start Menu\Programs\Startup`。

**macOS**：用 `launchd` 或"系统设置 → 通用 → 登录项"添加一个运行脚本的 App。

**Linux**：在 `~/.config/autostart/` 放一个 `.desktop` 文件。

> 注意：开机自启时 Codex 可能尚未启动，建议在脚本里先 `sleep` 几秒或让程序用 `-wait 60`。

## 四、卸载

无需卸载：程序不写注册表、不装服务、不改 Codex。删掉下载的文件即可。
若想恢复英文，在 Codex 设置里把语言改回即可。

## 五、故障排查

| 现象 | 处理 |
|---|---|
| 等待调试端口超时 | 完全退出 Codex（含托盘）后重试 |
| 找不到 Codex | 用 `-codex "路径"` 指定 |
| macOS 无法打开 | `xattr -d com.apple.quarantine <文件>` |
| Windows 被 SmartScreen 拦截 | 更多信息 → 仍要运行 |
| 界面仍是英文 | 先 `-doctor` 看是否找到 Codex；再确认重启了 Codex |
| 升级 Codex 后失效 | Statsig 键名可能变了，提 Issue 或按 `docs/ARCHITECTURE.md` 适配 |

更多诊断：

```bash
codex-chinese -doctor
```
