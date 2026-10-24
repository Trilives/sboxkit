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

## 任务清单

### P0：固化 bootstrap DNS 中继的 AF_NETLINK 修复并发布

**故障与根因（2026-10-20）**：订阅及节点本身可用，但启用 sing-box 后，节点域名解析返回 `SERVFAIL`。中继读取 `/proc/net/route` 后调用 Go 的 `net.InterfaceByName`；该调用需要 `AF_NETLINK`，而生成的 systemd 单元仅允许 `AF_INET AF_INET6`。实际跟踪到 `socket(AF_NETLINK, ...) = -1 EAFNOSUPPORT`，所以中继在连接上游 DNS 前就失败。停服时直接查询上游可解析该域名。

- [X] 在系统模式的 `bootstrapServiceText` 与用户模式的 `bootstrapUserServiceText` 模板中加入 `AF_NETLINK`，保留 `AF_INET AF_INET6`；更新单元文本测试。当前改动仍在工作区，尚未提交。
- [X] 在 ares 上通过 `sing-box-bootstrap-dns.service.d/override.conf` 临时放行 `AF_NETLINK` 并重启服务；本机中继已能解析节点域名，经 mixed 端口访问 Google 返回 HTTP 204、Cloudflare 返回 HTTP 200。
- [X] 运行 `gofmt`、`go test ./...`、`go vet ./...`；发布资产已检查（2026-10-20）：`goreleaser check` 与三架构快照构建通过，deb / 用户模式包含随包内核与规则集，包内二进制的两个中继单元均含 `AF_NETLINK`。
- [X] 运行时物理网卡发现测试（2026-10-20）：`sysd/bootstrap_sandbox_linux_test.go` 从生成的系统/用户单元读取 `RestrictAddressFamilies`，在子进程内以 seccomp 复现该限制后运行真实的 `/proc/net/route` 发现与 netlink 接口查询（拨号前取消，不发包）；反向对照确认旧值 `AF_INET AF_INET6` 复现 `EAFNOSUPPORT`。
- [X] 用新构建包验收中继单元与真实代理（2026-10-20，未中断本机运行中的网络）：确认系统和用户模式的中继单元都允许 `AF_NETLINK`，节点域名返回 `NOERROR`，并通过 mixed 端口完成真实代理请求。
  - 系统模式：从快照 deb 解出的二进制按生成的系统单元 `[Service]` 全部加固项（root、能力集、`ProtectSystem=strict` 等）以临时系统单元运行在 16452 端口，两个节点域名均 `NOERROR`。
  - 用户模式：同一二进制按用户单元加固项以 `systemd-run --user` 运行在 16450 端口返回 `NOERROR`；仅放行 `AF_INET AF_INET6` 时返回 `SERVFAIL`。
  - 真实代理：新构建生成的无 TUN 配置以旁路 sing-box 1.14.1 实例（mixed 17890）经上述系统单元 relay 解析节点，Google/Cloudflare 返回 204。
  - 未覆盖：deb 在干净主机上的安装/升级流程本身（本机 docker 桥接网络不可用，原机安装会中断运行中的网络），留待升级 ares 时复核。
- [ ] 发布并升级 ares 后，确认生成单元本身已包含 `AF_NETLINK`；再移除临时 override，执行 `daemon-reload` 和重启复验。若复验失败，恢复 override 并保留现有可用服务。
  - v0.3.1 已发布（2026-10-20）；ares 的升级与 override 移除由用户稍后执行。自更新后的「重新生成当前配置并重启服务？」只会重新暂存现有配置、重写单元，需选「是」才会写入 `AF_NETLINK`。

### P0：修复 TUN 模式下远程 DNS 直连超时

**故障与根因（2026-10-20）**：mixed 端口测速和访问 OpenAI/Anthropic API 正常，但 TUN 下 Codex、Claude 的域名解析超时。生成的 `dns-proxy` 使用 Google DoH，却直连 `8.8.8.8:443`；当前网络无法直连该地址。sing-box 日志反复出现 `api.openai.com`、`api.anthropic.com` 等域名的 DoH 拨号超时。

- [X] 将生成的 `dns-proxy` 的 `detour` 改为 `Proxy`，仍通过 `dns-direct` 解析 DoH 服务器域名；保持节点域名和引导中继走直连，避免代理自举循环。更新相关转换测试；`go test ./...`、`go vet ./...` 已通过。代码仍在工作区，尚未提交。
- [X] 备份并修正 ares 的当前运行配置，通过 `sing-box.service.d/50-remote-dns-detour.conf` 在主服务启动前临时校正旧版生成配置。TUN DNS 查询返回 `NOERROR`；不显式设置 HTTP 代理访问 OpenAI、Anthropic API 分别收到 HTTP 401、405，说明已到达服务端。
- [ ] 以新构建包验收订阅刷新、重建和服务重启后 `dns-proxy.detour=Proxy`，并在 TUN 下验证 Codex、Claude 的实际交互，而不只验证 mixed 端口。
  - 已完成（2026-10-20，状态副本）：新构建「订阅管理 → 重新生成配置 → Hua」后订阅与生效配置均为 `detour=Proxy`、`domain_resolver=dns-direct`，并通过 sing-box 1.14.3 真实 `check`。
  - 已完成（旁路实例）：上述配置中 `dns.final=dns-proxy`，`api.openai.com`、`api.anthropic.com`、`chatgpt.com` 经 DoH over Proxy 均 `NOERROR`；经 mixed 端口访问 OpenAI、Anthropic API 分别返回 401、405。
  - 待完成：刷新时订阅源直连返回 HTTP 403（3 次重试，原文未变），需在可拉取的网络下复验；TUN 下 Codex/Claude 实测需在 ares 升级并刷新配置后进行（会重启主服务）。
- [ ] 发布并升级 ares 后移除临时的 `50-remote-dns-detour.conf` 和 `/usr/local/libexec/sboxkit-remote-dns-detour`，重启复验；失败时恢复临时修正。与此同时处理上面的 `AF_NETLINK` 临时 override。
  - 注意：任何更新路径都不会从订阅原文重建 `config.json`；须先在「订阅管理」刷新或重新生成订阅，`dns-proxy` 才会变为 `Proxy`，否则移除临时修正后会回到直连 DoH。
  - 注意（2026-10-24）：须升级到含 DNS 响应匹配迁移的版本（见 [`Done.md`](Done.md)）后再重新生成订阅。v0.3.1 生成的 Hua 配置（hosts CNAME + FakeIP + `geoip-cn`）在 sing-box 1.14.3 下 `check` 即报 `Legacy Address Filter Fields` 并拒绝启动；ares 已以本地热修版从 Hua 原文重建并恢复服务，正式发布及移除临时 override 仍待完成。
