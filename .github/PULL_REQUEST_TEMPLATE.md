## 改动说明

<!-- 做了什么，为什么，修复了什么根因？ -->

## 验证

- [ ] `gofmt -l .` 无输出
- [ ] `go vet ./...` 通过
- [ ] `go test ./...` 通过
- [ ] `node test/prelude.test.mjs` 输出 `ALL PASS`
- [ ] `build.ps1` 或 `build.sh` 生成 8 个产物
- [ ] Windows + MSIX Codex 上 `-verify` 退出码 `0`

## 架构检查

- [ ] 未创建 `.bat` / `.vbs` 启动器
- [ ] 未写死 Codex 版本目录
- [ ] 未引入第三方 Go 依赖
- [ ] 保留启动器与 supervisor 全局互斥
- [ ] 优先复用已有 Codex CDP 实例
- [ ] 未在 Codex bootstrap 阶段强制刷新
