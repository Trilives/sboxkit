# sboxkit 开发清单

当前没有未完成事项。已完成内容与验收记录见 [`Done.md`](Done.md)。

新增工作应满足：

- 遵守 [`docs/ARCHITECTURE.md`](../docs/ARCHITECTURE.md) 的事务和分层约定。
- 遵守 [`docs/MODULARITY.md`](../docs/MODULARITY.md) 的规模红线。
- Go 改动通过 `gofmt`、`go test ./...`、`go vet ./...`。
- 交互改动同时验证 TTY 与非 TTY；发布资产验证脚本和 GoReleaser 配置。
