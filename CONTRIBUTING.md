# 贡献指南

## 开发环境

- Go 1.21+
- Node.js 18+（prelude 测试）
- Windows 上验证 MSIX 启动

## 修改后必须运行

```bash
gofmt -l .
go vet ./...
go test ./...
node test/prelude.test.mjs
VERSION=dev bash build.sh
```

Windows：

```powershell
$env:VERSION="dev"
.\build.ps1
```

## 架构约束

- `go.mod` 不允许出现 `require`。
- 跨平台代码必须使用 build tag。
- Windows 托盘必须编译为 GUI 子系统。
- 不得创建 `.bat` / `.vbs` 启动器。
- 不得写死 Codex 版本目录。
- 不得在 `WindowsApps` 中递归遍历。
- 必须保留全局 launcher/supervisor 互斥锁。
- 必须优先复用已有 Codex CDP 实例。
- 必须保留长驻 supervisor。
- prelude 包装器必须返回原始对象。

## 提交 PR

1. 新建分支。
2. 完成自测。
3. 描述修改动机、根因和验证证据。
4. 涉及注入逻辑时附上 `-verify` 输出。

如果没有真实 Windows + Codex 验证，不要声称端到端成功。

## 许可证

提交内容默认按 AGPL-3.0 授权。
