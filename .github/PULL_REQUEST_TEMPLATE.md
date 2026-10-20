## 改动说明

<!-- 这个 PR 做了什么，为什么 -->

## 检查清单

- [ ] `go vet ./...` 通过
- [ ] `VERSION=dev bash build.sh` 能出 6 个产物
- [ ] 未引入第三方 Go 依赖（`go.mod` 无 `require`）
- [ ] 单个产物体积仍在 3–4 MB 量级
- [ ] 未修改 Codex 安装文件 / 未使用 UPX
- [ ] 如有平台相关代码，放在带 `//go:build` 的文件里
