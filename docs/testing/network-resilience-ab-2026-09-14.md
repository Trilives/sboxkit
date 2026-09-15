# sing-box 1.13.18 / 1.13.19 网络自愈 A/B 实验（已取消）

状态：用户于 2026-09-15 决定停止版本 A/B 与后续特权故障注入；本文及脚本仅作为历史
实验设计留档，不再属于 Todo 或 v0.2.5 发布验收。

本文描述 `scripts/network-resilience-ab.sh` 的安全边界、输入契约、证据目录和待填写的
实验记录。脚本用于比较相同 sing-box 配置在两个内核版本下遇到 bootstrap DNS 故障时
的可观测行为，不应单凭一次结果把问题归因于某个版本。

这不是当前 sboxkit 整体自愈链路的验收脚本：它只启动两个版本的 sing-box，复现历史上
由内核直接访问 bootstrap DNS 的路径，不启动新增的 loopback relay、healthcheck 或
watchdog。relay fallback、分层分类、重启限额、单次快照和解除故障后恢复必须另做真实
systemd/产品级故障注入；该实验范围现已取消，不能据此声称取得上述证据。

## 安全边界

脚本默认拒绝执行。必须显式选择模式：

- `dry-run`：普通用户验证路径并打印 32 项矩阵，不调用 `ip`，不创建输出目录。
- `verify`：普通用户执行两个内核的 `version`，并用每个内核 `check` 四份配置。
- `prepare`：要求 root 和 `AB_CONFIRM_ISOLATED=yes`，临时创建两个 namespace，验证 veth
  连通后立即清理；不启动 sing-box，不注入故障。
- `run`：要求 root、人工确认和一个不存在的绝对输出目录；逐项创建隔离拓扑、采证并清理。
- `cleanup`：要求 root 和人工确认；只删除当前 `AB_RUN_ID` 对应、以 `sboxkit-ab-` 开头的
  client/upstream namespace，并要求 `/run` 中 root-only 所有权标记记录的 namespace inode
  仍与当前对象一致；标记缺失或 inode 不同就拒绝删除。

主动模式不会改变宿主默认路由、策略路由、sysctl 或宿主 nftables/iptables。故障 nft
规则只存在于 upstream namespace。veth 两端在创建后立即移入 namespace；每个矩阵项结束
都会删除 namespace。脚本不会删除或覆盖证据目录。

不要在生产主机上首次试跑。应使用可丢弃的 Linux VM；VM 快照是额外保护，不替代脚本的
作用域检查。

## 输入契约

必须显式提供两个本机可执行的 Linux 二进制：

```bash
export SING_BOX_11318_BIN=/absolute/path/to/sing-box-1.13.18
export SING_BOX_11319_BIN=/absolute/path/to/sing-box-1.13.19
```

`verify` 会检查 `sing-box version` 输出包含期望版本。脚本不会下载二进制；应从受信任来源
单独下载并在实验记录中填写 SHA-256。`run` 还要求从可信发布渠道独立取得并设置
`SING_BOX_11318_SHA256`、`SING_BOX_11319_SHA256`，执行前逐一匹配；不要对不可信二进制
使用它自己提供的散列值。`run` 校验原文件后会把 core 复制到 root-owned 0700 临时目录，
再次核对散列并只执行 0500 副本，以关闭用户可写源文件的替换窗口。core 仍以 root 在宿主
mount/PID namespace 中执行，所以必须只在可丢弃 VM 使用受信任的官方二进制；网络 namespace
不是文件系统沙箱。`run` 因此另行要求 `AB_CONFIRM_TRUSTED_ROOT_CODE=yes`，明确确认这条
非网络信任边界。

`AB_CONFIG_DIR` 必须包含以下四份非空 JSON：

```text
tun-on-fakeip-on.json
tun-on-fakeip-off.json
tun-off-fakeip-on.json
tun-off-fakeip-off.json
```

这四份配置用于复现历史上由 sing-box 内核直接访问 bootstrap DNS 的路径。每份配置必须：

1. 使用相同节点、路由、mixed 端口和 bootstrap DNS 参数，仅改变文件名声明的 TUN/FakeIP
   开关。
2. 将 bootstrap DNS 的 IP 字面量设为隔离 upstream 地址 `198.18.0.2`；如覆盖
   `AB_UPSTREAM_IP`，四份配置同步使用覆盖值。
3. 不引用宿主 DNS、宿主网关或公网 bootstrap 地址。
4. `tun-on` 文件包含 `type: tun` inbound，`tun-off` 不包含；`fakeip-on` 包含
   `type: fakeip` DNS server，`fakeip-off` 不包含。

`verify` 会解析 JSON 检查以上开关和隔离 upstream 引用，再由两个内核分别执行 `check`。
它不会证明节点凭据有效，也不会访问节点。

运行标识只接受 1–24 位小写字母、数字或连字符：

```bash
export AB_CONFIG_DIR=/absolute/path/to/ab-configs
export AB_RUN_ID=20260914-a
export SING_BOX_11318_SHA256=<64-hex-from-trusted-release>
export SING_BOX_11319_SHA256=<64-hex-from-trusted-release>
export AB_CONFIRM_TRUSTED_ROOT_CODE=yes
```

可选参数及默认值：

| 变量 | 默认值 | 含义 |
|---|---|---|
| `AB_CLIENT_IP` | `198.18.0.1` | client veth 地址 |
| `AB_UPSTREAM_IP` | `198.18.0.2` | upstream/bootstrap 地址 |
| `AB_MIXED_PORT` | `7890` | mixed HTTP proxy 端口 |
| `AB_PROBE_URL` | `http://connectivitycheck.gstatic.com/generate_204` | 经 mixed 触发的请求 |
| `AB_SETTLE_SECONDS` | `2` | 内核启动后的等待时间 |
| `AB_PROBE_TIMEOUT` | `8` | 单次 curl 上限，秒 |

隔离拓扑没有 NAT 或公网路由，因此默认探测 URL 仅用于触发节点域名解析和出站建立，不应
期待得到 HTTP 204。实验比较的是两个版本在同一故障下的日志、DNS 报文、socket 和
conntrack 差异。脚本拒绝带 userinfo、query 或 fragment 的探测 URL，避免证据日志携带凭证。

## 执行顺序

先运行仓库自测和无特权验证：

```bash
bash -n scripts/network-resilience-ab.sh
bash -n scripts/test-network-resilience-ab.sh
bash scripts/test-network-resilience-ab.sh

scripts/network-resilience-ab.sh dry-run
scripts/network-resilience-ab.sh verify
```

`dry-run` 应打印 `matrix_cases=32`：

```text
2 个内核版本 × 4 类故障 × 2 个 TUN 状态 × 2 个 FakeIP 状态
```

在可丢弃 VM 中安装以下工具：`ip`、`nft`、`tcpdump`、`ss`、`conntrack`、`curl`、
`timeout`、`sha256sum`、`python3`、`ping`。然后先验证拓扑生命周期：

```bash
sudo --preserve-env=SING_BOX_11318_BIN,SING_BOX_11319_BIN,AB_CONFIG_DIR,AB_RUN_ID \
  env AB_CONFIRM_ISOLATED=yes scripts/network-resilience-ab.sh prepare
```

确认 prepare 清理了两个 namespace 后再执行完整矩阵：

```bash
export AB_OUTPUT_DIR=/tmp/sboxkit-ab-20260914-a
sudo --preserve-env=SING_BOX_11318_BIN,SING_BOX_11319_BIN,SING_BOX_11318_SHA256,SING_BOX_11319_SHA256,AB_CONFIG_DIR,AB_RUN_ID,AB_OUTPUT_DIR,AB_CONFIRM_TRUSTED_ROOT_CODE \
  env AB_CONFIRM_ISOLATED=yes scripts/network-resilience-ab.sh run
```

若进程被强制杀死且 trap 没有运行，可人工确认 run-id 后执行限定清理：

```bash
sudo --preserve-env=SING_BOX_11318_BIN,SING_BOX_11319_BIN,AB_CONFIG_DIR,AB_RUN_ID \
  env AB_CONFIRM_ISOLATED=yes scripts/network-resilience-ab.sh cleanup
```

清理前应先执行 `ip netns list`，确认目标名称与本次 run-id 完全一致。

## 故障模型

| 故障 | namespace 内注入方式 | 报文侧预期证据 |
|---|---|---|
| `tcp-rst` | upstream nft 对 TCP/53 `reject with tcp reset` | SYN 后出现 RST |
| `tcp-blackhole` | upstream nft 对 TCP/53 `drop` | SYN 重传，无响应 |
| `dns-no-response` | Python fixture 接受 TCP/UDP 查询但不回复 | TCP 建连及查询完整，无 DNS 响应 |
| `uplink-down` | client veth 设置为 down | client link 为 DOWN，无有效上行报文 |

这些预期只用于检查故障注入是否成功，不等同于 sing-box 行为结论。若 pcap 不满足对应特征，
该矩阵项应标记为 `INVALID_INJECTION` 并重跑，而不是纳入版本比较。

## 证据目录

每项保存在：

```text
<output>/version-<version>/fault-<fault>/tun-<on|off>/fakeip-<on|off>/
```

文件含义：

| 文件 | 内容 |
|---|---|
| `metadata.txt` | 版本、故障、开关、二进制/配置 SHA-256、namespace 与探测参数 |
| `sing-box.log` | 对应内核的完整运行日志 |
| `packets.pcap` / `tcpdump.log` | client namespace 的 bootstrap DNS 报文；不捕获 mixed 流量，snaplen 限 4096 |
| `before-snapshot.txt` | 故障注入后、内核启动前的 link/rule/route/ss/conntrack/nft 状态 |
| `after-snapshot.txt` | 探测后的同类状态 |
| `probe.txt` | mixed curl 输出和退出码；内核未启动时记录 `not-run` |
| `result.txt` | 内核是否保持运行及完成时间 |
| `dns-fixture.log` | 仅 `dns-no-response`：接受/持有/丢弃查询的记录 |

顶层 `manifest.txt` 固定记录矩阵大小和隔离地址。配置文件只引用路径与散列，不复制订阅
凭据；脚本使用 `umask 077` 创建私有证据。归档前仍须人工检查 `sing-box.log` 是否包含敏感
节点信息并按需脱敏。

## A/B 证据模板

### 执行元数据

- 状态：`NOT_RUN` / `PARTIAL` / `COMPLETE`
- 操作者：`<name>`
- 开始/结束时间：`<ISO-8601>`
- VM/镜像：`<distribution and snapshot id>`
- Git revision：`<commit>`
- 输出目录 SHA-256 manifest：`<path or hash>`
- 1.13.18 来源与 SHA-256：`<source>` / `<hash>`
- 1.13.19 来源与 SHA-256：`<source>` / `<hash>`
- 四份配置 SHA-256：`<list>`
- `dry-run` 结果：`PASS` / `<failure>`
- `verify` 结果：`PASS` / `<failure>`
- `prepare` 结果：`NOT_RUN` / `PASS` / `<failure>`
- `run` 结果：`NOT_RUN` / `COMPLETE` / `<completed count>/32`
- namespace 清理确认：`NOT_RUN` / `PASS` / `<remaining namespaces>`

### 矩阵判读

每个单元填写 `SAME`、`DIFFERENT`、`INVALID_INJECTION` 或 `NOT_RUN`，并链接两版证据目录。

| 故障 | TUN | FakeIP | 注入有效 | 1.13.18 vs 1.13.19 | 证据/备注 |
|---|---|---|---|---|---|
| TCP RST | on | on | `<status>` | `<verdict>` | `<paths>` |
| TCP RST | on | off | `<status>` | `<verdict>` | `<paths>` |
| TCP RST | off | on | `<status>` | `<verdict>` | `<paths>` |
| TCP RST | off | off | `<status>` | `<verdict>` | `<paths>` |
| TCP 黑洞 | on | on | `<status>` | `<verdict>` | `<paths>` |
| TCP 黑洞 | on | off | `<status>` | `<verdict>` | `<paths>` |
| TCP 黑洞 | off | on | `<status>` | `<verdict>` | `<paths>` |
| TCP 黑洞 | off | off | `<status>` | `<verdict>` | `<paths>` |
| DNS 建连无响应 | on | on | `<status>` | `<verdict>` | `<paths>` |
| DNS 建连无响应 | on | off | `<status>` | `<verdict>` | `<paths>` |
| DNS 建连无响应 | off | on | `<status>` | `<verdict>` | `<paths>` |
| DNS 建连无响应 | off | off | `<status>` | `<verdict>` | `<paths>` |
| 物理上行中断 | on | on | `<status>` | `<verdict>` | `<paths>` |
| 物理上行中断 | on | off | `<status>` | `<verdict>` | `<paths>` |
| 物理上行中断 | off | on | `<status>` | `<verdict>` | `<paths>` |
| 物理上行中断 | off | off | `<status>` | `<verdict>` | `<paths>` |

### 结论门槛

只有同时满足以下条件，才能写“观察到版本差异”：

1. 对应 pcap/fixture/link 状态证明两版的故障注入等价且有效。
2. 两版使用完全相同的配置 SHA-256、probe 参数和 VM 内核/工具版本。
3. 差异可在全新 namespace 中至少重复两次。
4. 日志时间线与 socket/conntrack 证据一致，不只是错误文案不同。

否则结论保持：`证据不足，不能归因于 sing-box 1.13.18 或 1.13.19`。

## 本次实现验证记录

### 用户目标

- 运维人员能够在不触碰宿主网络配置的前提下预览并验证实验矩阵。
- 经明确授权的 root 操作者能够只在隔离 namespace 中注入四类故障并保存证据。
- 评审人员能够区分“工具已验证”与“真实特权实验已执行”。

### RED / GREEN

- RED：加入 `scripts/test-network-resilience-ab.sh` 后，因 harness 尚不存在而失败；输出为
  `default.out does not contain: refusing to touch networking`。
- GREEN：实现 harness 与 no-response fixture 后，执行
  `bash scripts/test-network-resilience-ab.sh`，输出
  `network resilience A/B harness self-test: PASS`。
- 语法验证：`bash -n scripts/network-resilience-ab.sh` 与
  `bash -n scripts/test-network-resilience-ab.sh` 均退出 0。
- fixture CLI 验证：`python3 scripts/network-resilience-no-response.py --help` 退出 0。

### 当前已知缺口

- 本次没有请求 root 权限，没有执行 `prepare` 或 `run`，因此没有真实 pcap、ss 或 conntrack
  证据；上方矩阵仍是待填写模板。
- 当前开发环境缺少 `conntrack` 命令；这是 `prepare/run` 的硬依赖，脚本会在创建 namespace
  之前拒绝执行。
- root-only 路径只能在安装完整依赖的可丢弃 Linux VM 中完成最终集成验证。
- 未创建 TDD checkpoint commit；共享工作树中存在其他并行任务修改，提交由主代理统一处理。
