# 已完成

## 2026-10-24：DNS 地址过滤迁移到 sing-box 1.14 响应匹配

- 故障：ares 的 Hua 配置在 sing-box 1.14.3 下 `check` / 启动即 `FATAL initialize dns router:
  validate dns rule[7]: Legacy Address Filter Fields ...`。根因：hosts CNAME 与 FakeIP catch-all
  规则使用 `query_type`，使 1.14 关闭旧版 DNS 模式；此时 rule[7] `rule_set: geoip-cn`（纯 IP
  规则集）与 hosts 的 `ip_accept_any` 规则缺少 `match_response`，被启动校验拒绝（见 1.14.3 源码
  `dns/router.go` 的 `validateLegacyDNSModeDisabledDefaultRule`）。
- 修复：两处地址过滤改为 `evaluate`（仅 A/AAAA/HTTPS，与旧写法生效范围一致）+
  `match_response` + `respond`：命中时直接返回预查询应答，失败或不命中继续后续规则。
  行为差异（新模式固有，无法保留）：旧写法在 `dns-direct` 出错时直接返回失败；现在继续匹配，
  relay 整体不可用时未列入 geosite-cn 的国内域名改由 FakeIP / `dns-proxy` 应答。
- 发布门禁缺口：`TestGeneratedConfigsPassReleaseSingBoxCheck` 此前只在空目录生成配置，缺少
  规则集时生成器不输出 geosite/geoip 规则，FakeIP + `geoip-cn` 组合从未被真实内核检查。现每个
  用例在有、无随包规则集下各查一遍，并新增 hosts、FakeIP + hosts 用例；修复前 7 个用例在
  1.14.3 下失败，修复后 1.14.1 / 1.14.3 全部通过。
- 验证（未触及运行中的服务）：用 ares 状态副本（Hua 原文 + `customize.json` + 规则集）以新代码
  重新生成，1.14.3 `check` 通过；旁路实例（无 TUN、独立端口）实测国内 IP 域名经
  `evaluate(dns-direct)` → `respond` 返回真实地址、非国内域名落入 FakeIP、geosite-cn / AI 域名
  路由不变、TXT 查询不触发预查询、hosts IP / CNAME 命中正确，日志无弃用告警。`gofmt`、`go vet`、
  `go test ./...`、`go test -race ./...`、带 1.14.3 内核的 `go test ./...` 与 `git diff --check` 通过。
- ares 生产恢复：备份原状态后，用修复后的代码从 Hua 本地订阅原文重建配置，保留 90 个节点；
  sing-box 1.14.3 对状态配置及运行时配置的 `check` 均通过。部署本地热修版
  `0.3.1-local-dnsfix.20261024` 并重新注册系统服务；主服务、bootstrap DNS、watchdog timer
  均已运行。bootstrap DNS 与 TUN DNS 查询正常；不显式指定代理时，OpenAI / Anthropic API
  分别返回 HTTP 401 / 405，证明网络可达，尚未验证登录后的 Codex / Claude 会话。
  旧版可执行文件的符号链接及状态备份保留用于回退；正式发布仍待完成。

## 2026-10-20：v0.3.1 稳定版

- bootstrap DNS relay 的系统/用户单元放行 `AF_NETLINK`，修复启用服务后节点域名 `SERVFAIL`；远程 DoH
  `dns-proxy` 改经 `Proxy`，修复 TUN 下境外域名解析超时（订阅刷新或重新生成后生效）。
- 初始化表单新增下载代理开关、文本项直接输入改写，并修复标签对齐、提示截断与非 TTY 校验重问。
- 发布前验收不中断本机运行中的网络：新构建 relay 以生成的系统单元加固项作为临时系统单元运行，节点域名
  `NOERROR`；新构建生成的无 TUN 配置以旁路实例经 mixed 端口访问 Google/Cloudflare 204、OpenAI 401、
  Anthropic 405，境外域名经 DoH over Proxy `NOERROR`。本机 docker 桥接网络不可用，未做容器内干净安装。
- 发布门禁：`gofmt`、`go vet`、`go test -race ./...`、带随包 sing-box 的 `go test ./...`、脚本 `bash -n`、
  `goreleaser check` 与三架构快照构建（tar.gz / deb / 用户模式包）通过。
- 发布后复核：release 与 CI 工作流通过；9 个资产与 `checksums.txt` 一致，随包内核刷新为 sing-box v1.14.3；
  deb 与自更新精简包中的 `sboxkit` 完全相同，版本为 0.3.1 且两个 relay 单元均含 `AF_NETLINK`；稳定渠道
  `/releases/latest` 返回 v0.3.1。发布产物同样以临时系统单元 relay（`NOERROR`）与旁路实例（Google 204、
  OpenAI 401、Anthropic 405、DoH over Proxy `NOERROR`、1.14.3 `check` 通过）复验，未触及运行中的服务。

## 2026-10-20：初始化表单直接输入与下载代理开关

- 初始化表单新增「使用下载代理」开关：关闭时下载代理地址禁用、「使用代理拉取」不可选，提交时清空
  已保存地址；开启时地址为必填（仅在提交时检查），并由 `config.ParseDownloadProxy` 校验
  http(s) / socks5(h) 代理地址，「配置变更」编辑同一字段时同样校验，避免格式错误的代理被拉取逻辑
  静默忽略。
- 文本项无需先按 Enter：首个可打印按键或粘贴直接改写原值（订阅名称等预填示例随之消失），
  Backspace / Ctrl+U 与 Enter 原位编辑，Esc 恢复本次修改前的值；Space 仍只用于勾选。
- 修复既有问题：表单标签按显示宽度对齐（中英文混排标签此前错位）；过长的校验提示在框内换行而非
  截断；非 TTY 回退中校验失败会提示原因，并以上次通过的值作为默认值重问，不再把被拒的输入当默认值。
- 便携版附带第三方资产、订阅示例按 Enter 消失两项此前已分别由 v0.3.0 用户模式安装包与
  2026-08-11 表单优化完成，本次只复核：快照用户模式包含 `core/`、`ruleset/` 与 `licenses/`，
  解压后离线接管 3 个种子文件。
- 验证：`gofmt`、`go vet ./...`、`go test -race ./...`、带随包 sing-box 的 `go test ./...`、
  `git diff --check`、脚本 `bash -n`、`goreleaser check` 与三架构快照构建；真实 tmux TTY（80 列）
  与非 TTY 管道分别走查开关、直接改写、必填提交、Esc 恢复与校验重问。

## 2026-09-30：v0.3.0 稳定版

- 用户模式成为第二种主流用法：`sboxkit-user_*.tar.gz` 随包内核与基础规则集，服务以
  systemd 用户单元运行，仅启用 TUN 时经 sudo `setcap` 一次；原「便携版」资产与脚本随之更名。
- 更新路径把新内核与 geo 规则部署进运行中的服务（两种模式）；用户模式卸载撤销 sboxkit 开启的
  linger，并补齐 root 误用拦截、状态目录 0700、系统服务运行时拒绝注册等防护。
- 发布门禁：`gofmt`、`go vet`、`go test -race ./...`、脚本 `bash -n`、`goreleaser check` 与三架构
  快照构建（tar.gz / deb / 用户模式包，含各架构内核）通过。

## 2026-09-30：用户模式全面复查

- 拒绝以 root 操作他人的用户模式安装包（sudo/su 启动会写入 root 属主文件并连到 root 的用户
  实例）；用户模式不再提示「直接用 sudo 启动」。
- 用户模式状态目录收紧为 0700（订阅原文与生成配置含节点凭据，旧安装在下次启动时收紧）。
- 系统模式 `sing-box.service` 运行中时拒绝注册用户服务（否则端口冲突导致用户服务反复重启），
  仅安装未运行时维持提示；本机（系统服务运行中）实测注册被拒、未写入任何用户单元、系统服务不受影响。
- 用户级各项服务操作（暂停/启动/重启/删除/定时器/自愈）先确认 systemd 用户实例可达，给出明确
  提示而不是裸露的 bus 错误。
- 措辞：卸载清单在用户模式显示「网络自愈（watchdog）」、结束后提示删除安装包目录；暂停提示区分
  linger；安装失败回退时不再误报「服务已删除」。
- 未改（记录）：日志为易失存储（无 `/var/log/journal`）时 `journalctl --user` 读不到用户服务
  日志；系统模式注册时不检测该用户的用户模式服务；系统模式状态目录与订阅文件仍为 0755/0644。

## 2026-09-30：用户模式（第二种主流用法）与随包内核

- 原「便携版」更名为**用户模式**，与 .deb 的系统模式并列为两种主流用法：安装包
  `sboxkit-user_*.tar.gz`、标记 `sboxkit.user-mode`、环境变量 `SBOXKIT_USER_MODE=1|0`、
  脚本 `scripts/user-{update,test}.sh`；`portable-update` 子命令更名 `assets-update`，旧名保留兼容。
- `paths.Detect` 沿 `os.Args[0]` 符号链接链查找标记（自更新后仍能找回包目录），状态默认落在
  包目录 `.sboxkit/`，运行时在 `<state>/runtime`；`sysd.unitScope` 统一系统/用户两种模式：
  用户模式下主服务、bootstrap DNS relay、watchdog 与每周更新写入 `~/.config/systemd/user`
  并经 `systemctl --user` 管理，单元显式写入 `SBOXKIT_USER_MODE=1` / `SBOXKIT_HOME`，
  watchdog 状态在 `$XDG_RUNTIME_DIR/sboxkit`，注册服务时尽力开启 linger；仅当 linger 由 sboxkit
  开启时在状态目录记标记，卸载服务时据此 `loginctl disable-linger`。
- 用户模式只在需要网络管理能力时提权：运行时配置含 `tun` 入站或 `routing_mark` /
  `default_mark` 且运行时内核缺少能力时，经 sudo `setcap cap_net_admin,cap_net_bind_service,
  cap_net_raw+ep`；内容未变的内核不再重新复制以保留能力，nosuid 挂载或缺少 setcap 时明确报错。
  其余操作（含自更新、状态目录创建）不回退 sudo；防火墙与 NM 钩子跳过，检测到系统模式同名
  服务时提示端口冲突。系统模式行为与命令保持不变。
- GoReleaser 新增用户模式安装包：随包 `core/sing-box`、版本文件、`ruleset/*.srs` 与
  `licenses/`，与 deb 共用 `fetch-deb-deps.sh` 预下载资产；初始化从包内种子离线接管并跳过初始化后
  的自动下载。精简 `sboxkit_*.tar.gz` 只含二进制，继续作为自更新资产。
- 每周更新定时器的 `ExecStart` 与 watchdog 一致改用稳定调用路径（系统模式同样生效），避免自更新
  清理旧版本后指向失效的版本目录。
- 验证：`gofmt`、`go test ./...`、`go vet ./...`、`git diff --check`、脚本 `bash -n`、
  `goreleaser check` 与三架构快照构建及包结构检查。systemd 容器（sudo 记录日志）中以普通用户：
  TUN 关闭时初始化、运行、pause/resume/重启全程零 sudo；开启 TUN 后「重载服务」恰好一次 sudo
  `setcap`，sing-box 以 UID 1001、CapEff=0x3400 运行并创建 singbox 设备与策略路由，随后重装/重启
  不再提权。上一轮另以 root 回归 deb 系统模式 init。容器无外网，未验证真实节点联网与 linger 自动开启。
- 修复既有问题：更新路径（`sboxkit update` / 每周定时器 / 工具→更新 / `user-update.sh`）的
  `SyncAndRestart` 原先不把新内核与 geo 复制到运行时，需下次重载服务才生效（两种模式相同）。现与
  安装共用 `deployRuntimeAssets`；用户模式 + TUN 下新内核以 `.new` 暂存并 setcap 后原子替换，
  无人值守无法 sudo 时保留旧内核、仍更新规则与配置，日志与下次进入菜单时提示「重载服务」。
  卸载「systemd 服务」时撤销 linger：`ensureLinger` 仅在由 sboxkit 开启时于状态目录记
  `linger-enabled-by-sboxkit`，卸载据此 `loginctl disable-linger <user>`；启停均显式带用户名，
  不在登录会话内调用（ENXIO）也能成功。
- 复验（systemd 容器，polkit 放行 set-self-linger，linger 初始为关）：TUN 初始化一次 sudo
  `setcap`（作用于 `sing-box.new`）；重载服务开启 linger 并写标记；state 内核变化后在无终端且 sudo
  需密码时同步，保留带能力的旧内核、服务与 TUN 正常，并在菜单提示；sudo 可用时同步恰好一次
  `setcap`，运行中进程即新内核（CapEff=0x3400），提示消失；内核未变的同步零 sudo；卸载后
  Linger=no、标记删除。系统模式（root）同步同样把新内核部署进 `/var/lib/sboxkit-runtime` 并重启。

## 2026-09-15：v0.2.5 稳定版

- 发布 bootstrap DNS 双上游故障转移、五层健康分类、watchdog 持久熔断与只读故障诊断。
- 修复 DNS 错误响应阻断 fallback、未使用节点误触发健康检查、并发探测排队，以及诊断快照
  在恢复/崩溃边界沿用旧证据的问题。
- 发布门禁覆盖竞态测试、vet、真实 sing-box 配置检查、A/B harness 安全自测和独立安全/
  代码复审。用户决定停止版本 A/B 与特权故障矩阵，相关事项从 Todo 关闭；这表示范围取消，
  不表示已经取得内核版本差异或真实故障注入证据。
- 发布依赖刷新到 sing-box v1.14.1；三架构 tar.gz/deb 快照构建及包结构校验通过。

## 2026-09-14：复发 bootstrap DNS 软故障的软件侧加固（后续 A/B 已取消）

- healthcheck 改为物理上行 → bootstrap DNS → 节点域名解析 → mixed 端口 → 端到端代理的
  短路分层探测，日志与诊断使用稳定故障分类；物理/bootstrap 套接字不走业务代理并绑定
  非 TUN 默认接口。
- 新增每 incident 一次的脱敏有界快照和 `sboxkit diagnose [--json]` 只读入口；采集固定
  白名单的接口、规则/路由、resolver、systemd 生命周期、socket、conntrack 与探针结果。
- watchdog 状态持久化到 `/run/sboxkit`，默认 10 分钟最多重启 2 次、指数退避、物理断网
  不重启、连续两轮健康才清状态；状态损坏时保守熔断，NetworkManager 事件复用同一限流器。
- 新增只监听 loopback 的 bootstrap DNS relay，支持 UDP/TCP/DoH 有序上游、逐次超时、
  冷却与半开恢复；默认使用不同运营方和传输，主 systemd unit 显式依赖 relay。
- relay 包测试覆盖率达到 80.8%；TUN × FakeIP 四组合通过 sing-box 1.13.18 与随包
  1.13.19 的真实
  `check`。1.13.18/1.13.19 × 四故障 × TUN/FakeIP 的 32 项隔离 netns harness 与证据模板
  已完成安全自测；特权矩阵未执行，并于 2026-09-15 按用户决定取消，不再保留为 Todo。
  RED/GREEN、覆盖率和验收边界见
  [`../docs/testing/bootstrap-dns-resilience-2026-09-14.tdd.md`](../docs/testing/bootstrap-dns-resilience-2026-09-14.tdd.md)。

## 2026-08-22：v0.2.4 稳定版收口

- 将 `v0.2.4-beta.1` 至 `v0.2.4-beta.3` 的 DNS bootstrap、网络自愈、Web 面板权限修复，以及本次主选择组限定功能收口为 `v0.2.4` 稳定版。
- 正式版节点切换只操作识别出的主 selector；外部配置可补充有序识别关键词，未识别时不再猜测其它 selector。
- 官方最新稳定 sing-box 标签核对为 `v1.13.19`；重新下载 amd64、arm64、armv7 资产后，模块版本与现有打包缓存逐架构一致。
- 发布门禁通过：带真实 sing-box 的 `go test ./...`、`go vet ./...`、静态版本构建、脚本语法、GoReleaser 配置检查，以及三架构 `tar.gz` / `.deb` 快照构建。
- 真实故障未再出现而无法执行的隔离网络/systemd 验收，以及本次未实现的诊断增强，按下节边界关闭归档，不计为稳定版验收通过。

## 2026-08-22：故障观察与诊断增强归档（非验收通过）

本节按发布决定关闭，不表示真实故障注入已经通过，也不把未复现的问题归因为 sing-box。
若同类故障再次出现，应重新建项并采集当次证据。

- **bootstrap DNS 故障注入**：代码侧已在 `v0.2.4-beta.2` 完成直连 TCP/DoH/DHCP bootstrap、删除未使用的 `dns-dnspod`、旧配置兼容、输入校验、依赖测试与真实内核 `check`；因故障未再出现，未执行真实隔离网络 UDP/TCP/DoH 黑洞 A/B，本项按未验收归档。
- **网络自愈真实 systemd 故障验收**：安装状态模型、幂等 reconcile、显式禁用持久化、稳定执行路径、状态展示与回滚测试已经完成；因现场软故障未再出现，未完成真实主机的单次恢复和防重启风暴故障注入，本项按未验收归档。
- **sing-box 1.13.18 UDP DNS 长连接失效复现**：现有证据只能证明当时 `dns-direct` 超时且重启后恢复，没有故障前物理网卡抓包、socket/conntrack 与版本 A/B；问题未能复现，不作上游回归结论，按证据不足归档。
- **DNS 软故障可观测性与诊断增强**：原计划包含 healthcheck 分类、DNS 超时限频汇总和只读诊断入口；这些增强未在本次稳定版实现，按本次范围决定取消并归档，后续只有在重新建项后才实施。

## 2026-08-21：节点切换限定主选择组

- “节点切换”与“固定节点”只展示识别出的主 selector 分组成员，不再在主组缺失时静默退化为成员最多的其它 selector。
- `customize.json` 新增有序的 `main_group_keywords`；优先精确匹配 `default_outbound`，再按关键词识别外部配置中的主选择组。
- 未识别到主组时列出现有 selector，并允许直接输入识别关键词；留空或输入不能匹配当前 selector 时不改配置，随后明确提示“未识别到主选择组，无法切换节点”。
- 新增精确主组、关键词识别、不猜测其它分组、直接输入持久化、无效输入不写盘，以及只收集主组成员的测试。
- 验证通过：TTY 与非 TTY 直接输入、`gofmt`、`go test ./...`、`go vet ./...`、`git diff --check`；TDD 记录见 [`../docs/testing/node-main-group-selection-2026-08-21.tdd.md`](../docs/testing/node-main-group-selection-2026-08-21.tdd.md)。

## 2026-08-21：v0.2.4-beta.3 Web 面板权限修复

- 本机验收定位到 root 周更会重建 `/var/lib/sboxkit/ui` 并把其属主变成 `root:root`，导致普通用户刷新或重建订阅时报 `unlinkat ... permission denied`。
- 面板运行来源改为二进制内嵌资源直接暂存到 `/var/lib/sboxkit-runtime/ui`；服务同步和订阅转换均不再改写共享的 `state/ui`，旧 root-owned 目录可原样保留且不影响运行。
- 新增缺失/只读 state UI、提权替换失败和访问提示测试；变更函数 `syncUIRuntimeWith` 的定向语句覆盖率为 90.9%。
- 本机 `Hua` 订阅已在无提权模式成功重建，生成配置默认使用直连 TCP bootstrap，移除了 `dns-dnspod`，并通过已部署 sing-box v1.13.18 的真实 `check`；运行时同步仍需交互式 sudo 后完成。

## 2026-08-21：DNS 与网络自愈 v0.2.4-beta.2 预览版加固

- bootstrap DNS 默认从 UDP 改为直连 TCP，并支持 UDP、TCP、直连 DoH 与 DHCP；DoH 只接受 IP 地址、始终走 `DIRECT`，不会通过 `Proxy` 或反向依赖业务 DNS，旧 `customize.json` 会安全补齐或修复新增字段。
- 删除从未被引用的 `dns-dnspod`；节点域名、直连域名、代理 DoH 与 bootstrap 的依赖关系新增测试，TCP、DoH、DHCP、默认 TUN 与 FakeIP 代表配置均通过随包 sing-box v1.13.19 的真实 `check`。
- 网络自愈新增 dispatcher/service/timer 的完整、陈旧、enabled/active 状态模型；初始化、启动、重载、更新会幂等修复缺失或过期组件，同时持久尊重用户明确禁用，并保留自定义探测间隔。
- watchdog 使用稳定的调用路径，避免自更新清理旧版本后留下失效 `ExecStart`；systemd unit 参数经过校验和引用，暂停主服务不会在后台重新拉起 watchdog，失败的安装/卸载会回滚用户偏好。
- 信息页展示三项自愈组件状态；切换/固定节点前优先读取 Clash API 的当前节点，API 不可用时以持久化 selector 首项作为明确标注的本地兜底。
- TDD 的 RED/GREEN 证据、覆盖率和未完成的真实主机验收记录于 [`../docs/testing/network-resilience-preview-2026-08-20.tdd.md`](../docs/testing/network-resilience-preview-2026-08-20.tdd.md)。

## 2026-08-11：初始化表单优化与稳定版内核兼容门禁

- 初始化表单进入空值文本字段时立即隐藏示例提示；编辑态按上/下方向键会校验并直接退出编辑、同步移动焦点，无需额外按一次 Enter。
- 超过 32 个字符的文本在浏览态仅显示前 12 个、六个星号和后 8 个字符；编辑与最终提交始终保留完整原值。
- 发布依赖脚本改为每次强制下载 amd64、arm64、armv7 的最新稳定 sing-box，并通过 Go 构建信息跨架构核对模块版本；deb 携带准确的 `sing-box.version`，首次接管不再只记录 `bundled`。
- 发布流水线在创建 Release 前，使用刚下载的 amd64 内核对默认 TUN 与 FakeIP 代表配置执行真实 `sing-box check`，并运行 `go test ./...`、`go vet ./...`，发现破坏性配置变更时直接阻止发布。
- 实测三个架构均重新下载 sing-box v1.13.18；完整 Go 测试、vet、真实 tmux TTY、脚本语法、GoReleaser 配置检查和三架构快照构建通过；amd64 deb 内核、版本文件与 sboxkit 可执行文件复核一致。

## 2026-08-11：订阅本地重建入口与 Hua 节点解析修复

- 订阅管理新增“重新生成配置”：可选择任意已有订阅，直接从保存的 `raw.*` 重新生成 sing-box 配置，不重新拉取订阅链接，用于应用新版转换规则。
- 定位 Hua 全部节点超时的根因：90 个节点依赖源 `hosts` 的三条域名别名，但节点显式 `domain_resolver` 会绕过 DNS rules，导致已生成的 `predefined` CNAME 无法参与节点自身解析，三个原始别名均错误落到同一入口地址。
- Clash 节点服务器命中源 `hosts` 域名别名时，生成 outbound 前应用别名链，再通过直连引导 DNS 解析；TLS 节点没有显式 SNI 时保留原始域名为 `server_name`。
- 无 TUN A/B 验证：香港、日本、摩尔多瓦三组代表节点使用原始别名均在 15–20 秒内超时，应用订阅映射后均立即请求成功；新版转换器生成的 Hua 90 个节点全部命中正确目标。
- 本机订阅、生效和运行时配置已更新并保留 `.pre-host-alias-fix` 备份；服务保持停止，未创建 TUN 或修改路由。
- 验证通过：真实 tmux TTY 完成“订阅管理 → 重新生成配置 → Hua”，随包 sing-box 1.13.18 `check`、`gofmt`、`go test ./...`、`go vet ./...`、`git diff --check`。

## 2026-08-11：初始化交互与 FakeIP 保留合并

- 单页表单改为浏览/文本编辑双状态：浏览态输入不会修改文本，Enter 进入并确认编辑，Esc 放弃当前字段修改；“开始初始化”成为可选中的末行，只有在该行按 Enter 才校验并提交。
- 初始化订阅类型缩短为 `Clash`、`sing-box`、`base64`、`本地文件`；界面标签与稳定的 `source_type` 值分离，添加订阅菜单继续保留长说明。
- `customize.json` 新增 `fake_ip_filter` 追加列表并接入“分流增强”；源订阅排除规则始终作为基线，用户条目按类型合并、保序去重，刷新及连续重建不会重复叠加。
- Clash 转换生成的代理节点显式使用直连引导 DNS 解析服务器域名；源 `hosts`、FakeIP 地址段和排除规则继续保留，外部 `RULE-SET` / `GEOSITE` 条目继续明确告警。
- 更新订阅保留文档，并增加表单状态机、稳定来源映射、配置往返、节点域名解析、源/用户规则合并及连续两次重建测试。
- 验证通过：tmux 真实 TTY 浏览/编辑/按钮提交、管道式非 TTY 初始化、`gofmt`、`go test ./...`、`go vet ./...`、`git diff --check`，以及随包 sing-box 1.13.14 的 `check`。

## 2026-08-09：初始化单页表单

- 在 `internal/tui` 增加可复用单页表单：文本直接输入、Space 勾选、左右键切换单值选项、Enter 校验提交、Esc 取消。
- 初始化一次收集下载代理、mixed 端口、TUN、局域网、防火墙、shell 代理、直连 UID/进程名和订阅信息，再交给原有独立事务执行。
- placeholder 只展示不写值；UID/进程、shell、防火墙和 sing-box 重建字段按其它选项动态启停。
- tmux 真实 TTY 逐键验证文本、勾选、左右选择、条件字段和提交；`NO_COLOR` 管道验证非 TTY 初始化可脚本化完成。

## 2026-08-09：重新初始化与服务重载

- 配置变更新增“重新初始化”：一次确认后只移除服务/运行时，保留状态、订阅原文和定制配置，再进入初始化表单。
- 服务设置保留普通重启，并新增“重载服务”：先从本地订阅原文重建，再由 `sysd.Install` 暂存和校验候选运行时，最后替换并启动服务。
- 转换或 `sing-box check` 失败发生在旧单元被替换前，并按“订阅生成 / 服务安装”阶段报告错误。

## 2026-08-09：订阅必要配置与 fake IP

- Clash/Mihomo 转换保留 `hosts`、fake-ip 模式、地址范围和 `fake-ip-filter` 精确/后缀/通配符语义；外部 RULE-SET/GEOSITE 条目明确警告而非静默丢弃。
- sing-box 统一重建保留现代 fakeip server、域名排除规则和 hosts predefined；passthrough 通过原始 JSON 保留未知顶层与嵌套字段。
- 刷新/重建始终从 `raw.*` 原文重新提取，连续两次重建测试确认 fake IP 语义不丢失。
- fake IP 使用 sing-box 1.12+ server 格式；生成配置通过随包 sing-box 1.13.14 的 `check`。
- 规则白名单、合并优先级和版本依据记录在 `docs/SUBSCRIPTION_PRESERVATION.md`。

## 2026-08-09：工具与便携模式

- 更新入口从运行时管理移动到工具；工具现在包含网络测试、内核/TUI 日志、文件位置、连接信息、内核/geo/sboxkit 更新和双语使用说明。
- 日志查看先选择 `journalctl` 内核日志或 `<state>/sboxkit.log`，展示后等待按键；文件位置页列出两者。
- 修复启动时读取错误日志字段的问题，文件日志现在按 `enable_file_log` 和配置的大小上限启用。
- tar.gz 便携包加入 `portable-update.sh` / `portable-test.sh`；新增不会操作 systemd 的 `portable-update` 子命令。
- 用户可写目录中的 sboxkit 自更新使用原子符号链接直接接管，无需 sudo；`/usr/bin` 等不可写安装仍安全回退 sudo。
- amd64/arm64 静态交叉构建、amd64 无 root 烟雾测试、脚本 `bash -n` 和 `goreleaser check` 均通过；README/命令参考已对齐中英文实际行为。

## 2026-08-09：mixed 代理端口单一配置源

- `customize.json` 新增 `mixed_port`，默认 `7890`，交互输入限制为 `1-65535`；旧配置缺少字段或手工写入越界值时安全回退默认值。
- converter、初始化提示、防火墙、shell 代理变量、网络测试、watchdog 默认值和访问提示统一使用该配置；修改已有端口时同步托管的 shell 代理块与局域网防火墙规则。
- “信息”工具改为读取 sing-box 的 `inbounds` 和 `experimental.clash_api`，不再误读 Clash 顶层 `mixed-port`、`tun`、`secret` 字段。
- 增加配置兼容、端口校验、converter、运行配置读取和代理环境变量测试。
- 验证通过：`gofmt`、`go test ./...`、`go vet ./...`、`git diff --check`。
