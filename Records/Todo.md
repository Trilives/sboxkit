# sboxkit 开发清单

未完成事项按优先级排列。已完成内容与验收记录于 [`Done.md`](Done.md)。

## 执行约束

- 遵守 [`docs/ARCHITECTURE.md`](../docs/ARCHITECTURE.md) 的事务和分层约定。
- 遵守 [`docs/MODULARITY.md`](../docs/MODULARITY.md) 的规模红线。
- 清单完成后移入 [`Done.md`](Done.md)。
- 订阅转换先核对 [`docs/SUBSCRIPTION_PRESERVATION.md`](../docs/SUBSCRIPTION_PRESERVATION.md)；涉及版本差异时以 sing-box 官方文档和实际随包内核校验结果为准，社区资料仅作补充。
- Go 改动通过 `gofmt`、`go test ./...`、`go vet ./...`。
- 交互改动同时验证 TTY 与非 TTY；若涉及发布资产，再验证相关脚本和 GoReleaser 配置。

## 任务清单

当前无待办事项。因故障未再出现而关闭的观察项，以及未纳入本次稳定版的诊断增强，
均以“未验收 / 未实现”的边界记录在 [`Done.md`](Done.md)；若同类故障再次出现，应从
归档记录恢复为新待办，而不是沿用未经复现的结论。
