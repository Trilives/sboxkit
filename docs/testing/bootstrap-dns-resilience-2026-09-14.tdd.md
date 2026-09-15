# Bootstrap DNS 软故障加固 TDD 记录（2026-09-14）

## 目标与边界

本轮针对 `Records/Todo.md` 中 2026-09-11 复发事件完成软件侧加固：分层探测、首次故障
快照、跨 timer 熔断、双上游直连 relay、只读诊断，以及隔离 A/B harness。

真实 32 项故障矩阵尚未执行：当前开发环境缺少 `conntrack`，且最终验收应在可丢弃 VM
中由 root 运行。此记录不把历史故障归因于 sing-box 1.13.18 或 1.13.19。

## RED

先加入下列行为测试，并确认因目标 API/行为尚不存在而编译或断言失败：

- `internal/bootstrapdns/forwarder_test.go`、`server_test.go`
- `internal/config/bootstrap_upstreams_test.go`
- `internal/converter/bootstrap_failover_test.go`
- `internal/sysd/health_policy_test.go`、`diagnostics_test.go`
- `internal/flows/diagnostic_test.go`
- `cmd/sboxkit/diagnose_test.go`

RED checkpoint：`1870242 test: add reproducers for bootstrap DNS resilience`。

## GREEN / IMPROVE

- `internal/bootstrapdns`：有序 fallback、逐次超时、冷却/半开、UDP/TCP/DoH 直连、
  `SO_BINDTODEVICE`、loopback 双协议服务和严格 runtime JSON。
- `internal/sysd`：五层短路探测、稳定分类、持久状态/锁/原子写、10 分钟 2 次重启上限、
  指数退避、两轮健康恢复、首次有界快照和并发白名单采集。
- `internal/config` / `internal/converter`：旧单上游迁移为有序双上游；生成配置仅指向本地
  relay，节点和 route resolver 共享 bootstrap 路径。
- CLI/TUI：`sboxkit diagnose [--json]` 与工具菜单只读展示。
- systemd：主服务 `Requires/After` relay；relay `PartOf` 主服务；共享运行时和 relay 端口
  明确限定为单实例；dispatcher 复用统一 healthcheck limiter。
- 特权实验：32 项 netns harness 默认拒绝执行，只允许显式 run-id、root 与人工确认后
  在 namespace 内注入故障；无特权自测锁定安全边界。

改进阶段补充了实际 loopback 传输、server 生命周期、runtime 严格解析、路由解析、状态损坏
fail-closed、文件权限、并发快照和 health probe 辅助路径测试；TCP client 增加 10 秒读写
deadline，避免本地慢连接无限占用 worker。

## 验证结果

```text
go test -count=1 -race ./...                              PASS
go vet ./...                                              PASS
gofmt                                                     PASS
git diff --check                                          PASS
bash scripts/test-network-resilience-ab.sh                PASS
python3 scripts/network-resilience-no-response.py --help  PASS
```

真实内核 schema 检查：

- 随包 `packaging/deps/amd64/sing-box`：1.13.19；TUN × FakeIP 四组合，以及 bootstrap
  DoH/DHCP 旧标量迁移代表配置全部 `check` 通过。
- 官方 release 临时下载到 `/tmp` 的 sing-box 1.13.18：相同组合与迁移配置全部 `check` 通过。
- 以上只证明两版接受生成 schema，不等于完成 RST/黑洞/无响应/上行中断的行为 A/B。

展示验证：

- 非 TTY：`sboxkit diagnose --json` 输出稳定 JSON unavailable 结构。
- 真 TTY：tmux 中运行 `sboxkit diagnose`，输出稳定纯文本 unavailable 状态且正常退出。

覆盖率：

- 新增 `internal/bootstrapdns` 包：80.8% statements。
- `internal/sysd` 包：45.7%；其中新状态机、固定采集编排和多数辅助函数为 80–100%，但包内
  大量历史 systemd 操作及需要真实宿主的生产闭包仍未被单元测试覆盖。
- 全仓：44.2% statements。仓库历史覆盖率显著低于 80%，因此不把 changed-package 或
  changed-function 覆盖率冒充全仓 80%；若发布门禁要求全仓 80%，需另开旧代码补测任务。

## 未完成验收

- 尚未在可丢弃 VM 运行 1.13.18 / 1.13.19 × 四类故障 × TUN/FakeIP 的 32 项矩阵。
- 尚无本轮真实 pcap、socket/conntrack、恢复时间和 watchdog 重启次数证据。
- 因此 `Records/Todo.md` 的版本 A/B 和总验收继续保持未勾选，结论保持“证据不足，不能
  归因到具体 sing-box 版本”。
