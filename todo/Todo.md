# sboxkit 开发清单

未完成事项按优先级排列。已完成内容与验收记录于 [`Done.md`](Done.md)。

## 执行约束

- 遵守 [`docs/ARCHITECTURE.md`](../docs/ARCHITECTURE.md) 的事务和分层约定。
- 遵守 [`docs/MODULARITY.md`](../docs/MODULARITY.md) 的规模红线。
- 订阅转换先核对 [`docs/SUBSCRIPTION_PRESERVATION.md`](../docs/SUBSCRIPTION_PRESERVATION.md)；涉及版本差异时以 sing-box 官方文档和实际随包内核校验结果为准，社区资料仅作补充。
- Go 改动通过 `gofmt`、`go test ./...`、`go vet ./...`。
- 交互改动同时验证 TTY 与非 TTY；若涉及发布资产，再验证相关脚本和 GoReleaser 配置。

## 任务清单

### P0：完成 bootstrap DNS 故障注入验收

**背景证据**：2026-08-21 00:19 起本机出现引导 DNS 超时，00:25 后扩大为全局解析失败；重启 sing-box 后立即恢复。故障版本中，所有代理节点的 `domain_resolver` 均指向单个 `dns-direct`（默认 `223.5.5.5:53/UDP`），同时生成了无人引用的 `dns-dnspod`；仅声明多个 DNS server 不会自动形成故障转移。

代码侧已在 `v0.2.4-beta.2` 完成直连 TCP/DoH/DHCP bootstrap、删除未使用的
`dns-dnspod`、旧配置兼容、输入校验、依赖关系测试与真实内核 `check`；完成记录见
[`Done.md`](Done.md)。剩余工作仅为真实隔离网络故障注入：

- 在隔离网络环境中做 UDP/TCP/DoH A/B：丢弃主 DNS 的响应后，系统不能无限期处于“进程存活但无法解析”的状态；恢复上游后无需人工重启即可恢复，或由下述 watchdog 在限定时间内恢复。

**验收标准**：单个 UDP/53 上游持续黑洞时，代理在预定恢复时限内自动恢复；配置中不存在未被引用且没有明确用途的“备用” DNS；`gofmt`、`go test ./...`、`go vet ./...` 和真实内核配置校验通过。

### P0：完成网络自愈真实 systemd 故障验收

**背景证据**：源码已有 `sing-box-watchdog.timer/.service` 和 NetworkManager dispatcher hook，但故障机器上三者均不存在，`systemctl` 显示 watchdog unit 为 `not-found`。因此本次“主进程仍运行、代理已失效”的软故障没有被自动拉起。

代码侧已在 `v0.2.4-beta.2` 完成安装状态模型、幂等 reconcile、显式禁用持久化、
稳定可执行路径、信息页状态与失败回滚测试；完成记录见 [`Done.md`](Done.md)。剩余工作为
有 NetworkManager 与 systemd 的真实主机集成验收：

- 做 systemd 集成验收：保持真实上行可用、阻断本地 mixed 代理探测，确认 watchdog 在重试及最小 uptime 门槛后只重启一次；真实上行断开时不得制造重启风暴。

**验收标准**：升级后的既有安装能发现并修复本次现场状态；正常初始化不会遗漏自愈组件；显式禁用被持久保留；故障恢复和防重启风暴场景均有可重复验收记录。

### P1：复现并界定 sing-box 1.13.18 UDP DNS 长连接失效

**证据边界**：当前日志能证明 `dns-direct` 持续超时且重启后恢复，但重启前没有物理网卡 UDP/53 抓包，尚不能断言最初触发是上游 DNS 黑洞、网络路径变化，还是 sing-box 复用 UDP socket 后未重建。该项必须先复现，不能把推断直接固化为修复结论。

- 使用随包 sing-box 1.13.18 构造最小配置，分别模拟丢弃 DNS 响应、上游恢复、默认接口变化和 UDP conntrack/NAT 状态变化。
- 同时记录物理接口抓包、socket/conntrack、`ip rule`/路由、systemd-resolved 状态及 debug 级 sing-box 日志，确认请求是否发出、响应是否返回、旧 UDP socket 是否被继续复用。
- 对比当前稳定版、可用的新版本及 UDP/TCP DNS transport；只有最小复现和版本 A/B 支持时，才将问题归因为上游 sing-box 回归并提交上游 issue。
- 将确认后的规避方式落实到 P0 bootstrap 设计或内核版本门禁；不得直接依赖未发布的 testing 构建进入稳定包。

**验收标准**：得到可重复的最小复现或有证据排除 sing-box socket 复用问题；结论包含抓包与版本 A/B 结果，能明确区分“外部上游故障”和“本地传输不恢复”。

### P1：增加 DNS 软故障的可观测性与诊断输出

- healthcheck 失败时区分并记录：真实上行、mixed 端口可连接性、经代理 HTTP 探测、bootstrap DNS 探测和服务 uptime；日志不得包含订阅凭据或节点密码。
- 对连续 DNS 超时做限频汇总，至少输出开始时间、持续时间、失败类型和恢复/重启结果，避免只能从大量 `context deadline exceeded` 中人工还原时间线。
- 在“工具”中增加只读诊断入口，汇总服务、watchdog、默认路由、TUN、systemd-resolved 和脱敏 DNS 拓扑；默认不改配置、不重启服务。
- 为日志脱敏、分类、限频和探测超时增加单元测试；诊断命令验证 TTY 与非 TTY 输出。

**验收标准**：再次出现同类问题时，不重启即可判断 DNS 请求是否离开物理接口、主/备 resolver 是否可达以及 watchdog 为什么执行或跳过；输出不泄露敏感字段。
