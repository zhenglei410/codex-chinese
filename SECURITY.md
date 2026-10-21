# 安全说明

## 报告问题

请通过 GitHub Security Advisories 私密报告。不要在公开 Issue 中粘贴：

- API Key；
- 登录令牌；
- 对话内容；
- 完整用户路径中的敏感信息。

## 运行时边界

- CDP 只连接 `127.0.0.1`。
- 不修改 Codex 安装目录、`app.asar` 或认证文件。
- 不读取、记录、上传 API Key、令牌或对话。
- 不请求管理员权限。
- 不引入第三方 Go 依赖。
- 不联网下载运行时脚本。

## 环境变更

Windows 安装器会执行以下用户级变更：

- 复制程序到 `%LOCALAPPDATA%\Programs\codex-chinese`；
- 创建桌面和开始菜单快捷方式；
- 写入当前用户登录启动项 `CodexChinese`。

卸载时会删除以上内容。

## CDP 风险

Codex 会以调试端口启动。端口仅绑定回环地址，但同机其他进程理论上可以连接。
不需要中文功能时，退出托盘并正常启动 Codex。

## 供应链

- `go.mod` 必须保持零 `require`。
- 发布产物附带 `SHA256SUMS`。
- 推荐从 GitHub Actions 构建的 Release 获取二进制。
