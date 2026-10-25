# sboxkit 开发清单

未完成事项按优先级排列。已完成内容与验收记录于 [`Done.md`](Done.md)。

## 执行约束

- 遵守 [`docs/ARCHITECTURE.md`](../docs/ARCHITECTURE.md) 的事务和分层约定。
- 遵守 [`docs/MODULARITY.md`](../docs/MODULARITY.md) 的规模红线。
- 清单完成后移入 [`Done.md`](Done.md)。
- 订阅转换先核对 [`docs/SUBSCRIPTION_PRESERVATION.md`](../docs/SUBSCRIPTION_PRESERVATION.md)；涉及版本差异时以 sing-box 官方文档和实际随包内核校验结果为准，社区资料仅作补充。
- Go 改动通过 `gofmt`、`go test ./...`、`go vet ./...`。
- 交互改动同时验证 TTY 与非 TTY；若涉及发布资产，再验证相关脚本和 GoReleaser 配置。
- 验证统一在 [`machine.md`](../machine.md) 指定的远程主机执行，禁止本机验证，完成后恢复现场。
- 只在用户要求时发布版本，且只发布预览版（带预发布后缀的标签，如 `v0.4.1-beta.1`）；稳定版须经用户确认后
  才发布。
- 每次发布稳定版：先更新所有相关文件、删除过时内容、更新 README 截图，再发布正式版；随后只保留默认分支，
  清理全部预览版的标签与 Release，并确认预览渠道的最新版本（`/releases` 列表第一项）就是该稳定版。

## 任务清单

暂无未完成事项。
