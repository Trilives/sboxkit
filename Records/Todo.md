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

### P0：复发的 bootstrap DNS 软故障诊断、故障转移与 watchdog 防重启风暴

2026-09-11 在 Ares-4A 再次复现全局节点超时。现场为 sboxkit 0.2.4、运行时
sing-box 1.13.18、TUN + FakeIP，`dns-direct` 使用 `DIRECT` 上的 TCP
`223.5.5.5:53`。16:35 起日志先出现 `connect: connection refused`，随后大量
`i/o timeout` / `context deadline exceeded`；节点入口域名无法 bootstrap 解析，
因此直连探针与代理节点同时失败。watchdog 在 16:35–16:55 共触发 10 次重启，
均未立即恢复，16:57 又在没有新重启的情况下自行恢复。恢复后的物理网卡抓包确认
TCP 握手、DNS 查询与响应完整，16:59 mixed 代理 `generate_204` 返回 204（约 51 ms）。
同期未发现物理 Wi-Fi 断开事件。以上证据仍不足以断言是 sing-box 内核回归；仓库随包
内核已是 1.13.19，而现场运行时仍为 1.13.18，也需要纳入 A/B。

- [x] 给 healthcheck 增加分层判定：分别探测物理上行、bootstrap DNS、节点入口解析、
  本地 mixed 端口和端到端代理，输出稳定的故障分类，而不是把所有失败都归为“proxy dead”。
  物理上行与 bootstrap 探测必须显式绕过 TUN/代理依赖，避免探针与故障路径循环依赖。
- [x] 首次进入失败状态、首次准备重启时生成有界诊断快照，至少包含默认物理接口、
  `ip rule`/相关路由、resolver 状态、bootstrap 目标、服务/接口生命周期、socket/conntrack
  摘要和各分层探针结果；日志需限频、脱敏，并提供只读诊断入口，便于故障仍在时采证。
- [x] 为 watchdog 增加跨 timer invocation 的冷却与熔断状态：连续重启无改善时指数退避，
  默认任意 10 分钟最多重启 2 次；连续两轮端到端探测恢复后再清除失败状态。日志明确记录
  “跳过重启”的原因，避免本次每 2 分钟一次的重启风暴。
- [x] 评估并实现不依赖 `Proxy` 的 bootstrap DNS 故障转移，至少允许两个不同运营方/
  不同传输的有序上游；主上游被 TCP RST 或黑洞时自动使用次上游，且节点服务器域名、
  代理 DoH 的建立均不能反向依赖失效的业务代理。
- [ ] 用相同配置对随包 sing-box 1.13.18 与 1.13.19 做版本 A/B；分别注入 TCP RST、
  TCP 黑洞、DNS 有连接无响应和物理上行中断，记录物理网卡侧报文与 socket/conntrack，
  未取得差异证据前不把问题归因到具体内核版本。
- [ ] 验收：上述四类故障均能被正确分类；单个 bootstrap 上游失效时代理仍可用；全部
  上游失效时重启次数满足限额且诊断快照只生成一次；解除故障后无需人工重启即可恢复；
  覆盖 TUN/FakeIP 开关组合、TTY/非 TTY 状态展示，并通过真实随包内核 `check`、
  `gofmt`、`go test ./...`、`go vet ./...`。
