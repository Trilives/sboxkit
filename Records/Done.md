# 已完成

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
