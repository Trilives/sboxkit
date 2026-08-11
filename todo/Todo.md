# sboxkit 开发清单

当前没有未完成事项。已完成内容与验收记录见 [`Done.md`](Done.md)。

## 执行约束

- 遵守 [`docs/ARCHITECTURE.md`](../docs/ARCHITECTURE.md) 的事务和分层约定。
- 遵守 [`docs/MODULARITY.md`](../docs/MODULARITY.md) 的规模红线。
- 订阅转换先核对 [`docs/SUBSCRIPTION_PRESERVATION.md`](../docs/SUBSCRIPTION_PRESERVATION.md)；涉及版本差异时以 sing-box 官方文档和实际随包内核校验结果为准，社区资料仅作补充。
- Go 改动通过 `gofmt`、`go test ./...`、`go vet ./...`。
- 交互改动同时验证 TTY 与非 TTY；若涉及发布资产，再验证相关脚本和 GoReleaser 配置。
