# 贡献指南

感谢你愿意改进这个项目。

## 仓库地址

https://github.com/zhenglei410/codex-chinese

## 开发环境

- Go ≥ 1.21
- 无第三方依赖（硬性要求）

## 本地验证

```bash
go vet ./...
VERSION=dev bash build.sh
./dist/codex-chinese-<os>-<arch> -version
```

## 代码约束

1. **零第三方依赖**：`go.mod` 不允许出现 `require`。禁止引入 `net/http`、`crypto/tls`、
   第三方 WebSocket / JSON 库。
2. **控制体积**：单个产物 3–4 MB；超过 6 MB 视为回归。
3. **平台代码分离**：平台相关代码放进 `*_windows.go` / `*_unix.go`，并加 `//go:build`。
4. **不碰用户环境**：除 `dist/` 外不写用户目录，不修改 Codex 安装目录。
5. **不用 UPX**。

## 提交 PR

1. Fork 并新建分支。
2. 按上面的清单自测。
3. 在 PR 描述里说明改动动机与验证方式。
4. CI 会在 Windows / macOS / Linux 三平台跑 `go vet` + 构建 + 版本冒烟测试。

## 适配新版 Codex

如果 Codex 升级后失效，通常是 Statsig 的 Layer/DC 名或键名变了。
需要定位新的名字并更新 `main.go` 里的 `prelude`，同时在 `docs/ARCHITECTURE.md` 记录。
