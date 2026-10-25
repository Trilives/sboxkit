# 架构与设计

sboxkit 是部署 / 管理 sing-box 的 Go 终端应用，姊妹项目 clashdock（管理 mihomo /
Clash.Meta）的同作者重写版。本文记录核心设计决策与分层，以及与 clashdock 的关键差异。

## 1. 核心理念：真实协议转换（而非 mihomo 版的"直用订阅 + 最小改写"）

clashdock 管理的 mihomo 原生解析 Clash 配置，因此不做协议转换、不重建分流，只
覆写部署必需字段。sing-box **不能**解析 Clash YAML，机场订阅（Clash / base64
通用节点链接）必须先转换成 sing-box 的 JSON schema 才能使用；sing-box 原生订阅
链接则可选择直接信任或按同一套规则重建。这一步全部交给 `internal/converter`：

- **节点转换**：`ClashToSingBox` 解析 Clash YAML 的 `proxies` 列表，把每个节点
  转成 sing-box outbound（覆盖 vmess/vless/trojan/ss/hysteria2/tuic/socks/http）。
- **原生订阅**：`SingBoxDirect` 支持两种模式——`customize=false` 只信任用户配置、
  仅补齐面板/控制器字段（passthrough）；`customize=true` 提取节点 outbound 重新
  走一遍与 Clash 来源一致的生成管线（分组/TUN/DNS 与其它订阅一致）。
- **分组与路由现场生成**：`BuildSingBoxConfig` 统一构造 inbounds（mixed 入站 +
  可选 TUN）、DNS、route 规则、outbounds 分组（主选择组 `Proxy` + 地区自动测速
  聚合组 SG-Auto/HK-Auto + AI/Streaming 分流组 + Auto/DIRECT/BLOCK），以及
  `experimental.clash_api`。AI / 流媒体 / 地区自动测速组不再像 mihomo 版那样是
  可选的"叠加层"（overlay/regiongroups 独立步骤）——sing-box 路径每次都是完整
  重新生成，这些分组本就是生成管线的一部分，开关只影响是否包含对应分组/规则。
- **base64 通用节点链接**：先经 `internal/subscription` 的分享链接解析器（或
  云端 subconverter）转成 Clash 风格的 proxy 字典，再喂给 `ClashToSingBox`——
  这一步与后端无关，clashdock 版的解析器逻辑原样保留。
- **订阅必要配置保留**：转换不会只摘节点；Clash 的 hosts/fake-ip-filter 与
  sing-box 自定义重建路径中的等价 DNS 语义会合并进生成配置，passthrough 则
  保留未知字段。具体白名单和优先级见 [SUBSCRIPTION_PRESERVATION.md](SUBSCRIPTION_PRESERVATION.md)。

`internal/subscription/detect.go` 相应地是三态识别（`clash` / `sing-box` /
`base64`），而不是 mihomo 版的两态（`clash` / `base64`）。

## 2. 配置的表示：真正的 JSON（而非 mihomo 版"JSON 即 YAML"技巧）

clashdock 的生效配置 `config.yaml` 内容其实是 JSON（利用"JSON 是合法 YAML"这
一点，省掉 YAML dumper，让 mihomo 直接解析）。sing-box 原生就吃 JSON，不需要
这个技巧：生效配置就是货真价实的 `config.json`，用标准库 `encoding/json`
读写，不再需要 YAML 往返。`internal/configfile`（读取时用 yaml.v3 解析）仍然
保留，只是现在的角色变成"顺带也能读 Clash YAML 夹具"，不是生产路径的必需品。

## 3. 状态与运行时分离

| 位置 | 内容 |
|---|---|
| `/var/lib/sboxkit`（固定工作目录，`SBOXKIT_HOME` 可覆盖） | 状态数据：内核、geo 规则集、`subscriptions/<name>/{meta.json,raw.*,config.json}`、`active` 指针、`customize.json`、记录运行时资源部署版本的 `runtime.version`、系统模式 .deb 缓存 `downloads/packages/`（非 dpkg 安装时为自更新版本库 `sboxkit-versions/`）。属主 root、属组为管理员组，目录 setgid（0755 / 订阅目录 0750），生效配置、`customize.json`、日志 0640：只有 root 可写，管理员组可读，root 定时器与管理会话共享同一份数据（权限模型见下文）；旧版留下的 `ui/` 不再作为运行来源 |
| `/var/lib/sboxkit-runtime` | root 态自包含运行时（0755，配置 0600）：内核 + 配置 + geo 规则集 + 面板资源，以及仅含 IP 字面量上游的 `bootstrap-dns.json`（服务与用户目录解耦） |
| `/var/lib/sboxkit-cache` | 主服务的 `StateDirectory`（`cache.db`）；`DynamicUser` 时 systemd 把它放在 `/var/lib/private/` 下并交给动态用户 |
| `/run/sboxkit` | watchdog 跨 invocation 状态与当前 incident 的首份脱敏诊断快照；原子写入并由文件锁串行化 |
| `/etc/systemd/system` | `sing-box` 主服务、bootstrap DNS relay、watchdog 与自动更新单元 |

**用户模式**（第二种主流用法；可执行文件旁有 `sboxkit.user-mode`，`paths.Detect` 沿
`os.Args[0]` 的符号链接链查找，自更新后仍能找回包目录；`SBOXKIT_USER_MODE=1|0` 可强制）
改用 systemd 用户单元，`sysd.unitScope` 统一承载两种模式的差异：

| 系统模式 | 用户模式 |
|---|---|
| `/var/lib/sboxkit` | `<包目录>/.sboxkit` |
| `/var/lib/sboxkit-runtime` | `<state>/runtime` |
| `/run/sboxkit` | `$XDG_RUNTIME_DIR/sboxkit`（缺失时 `<state>/run`） |
| 管理界面以调用用户打开，首个改动操作经 sudo 整体提权；`/etc/systemd/system` + `systemctl` | 普通用户运行，`~/.config/systemd/user` + `systemctl --user`，仅 TUN 时 sudo |
| unit 的 `AmbientCapabilities`，默认 `DynamicUser` | 运行时内核的文件能力（`setcap`） |

用户单元显式写入 `SBOXKIT_USER_MODE=1` 与 `SBOXKIT_HOME`，watchdog / 更新定时器 /
relay 与交互会话看到同一份数据。用户服务本身没有 CAP_NET_ADMIN：暂存的运行时配置
需要它（`tun` 入站或 `routing_mark` / `default_mark`）而运行时内核尚无对应文件能力时，
`sysd.ensureNetCapabilities` 经 sudo `setcap` 授予与系统 unit 相同的三项能力（nosuid
挂载或缺少 setcap 时直接报错）；内容未变的内核不会重新复制，以免丢失能力、反复提权。
运行时目录为 0700，其他用户无法执行带能力的内核。带文件能力的 sing-box 同样无权向
systemd-resolved 登记 TUN 的 DNS（见下文「TUN 的系统 DNS」），用户单元又不能以 root 执行钩子：
`sysd.ensureUserTunDNS`（`sysd/tundns_user.go`）因此经同一次 sudo 为每个 TUN 链路写入系统单元
`sboxkit-tun-dns-<uid>-<链路>.service`，`WantedBy` / `BindsTo` 链路的设备单元
`sys-subsystem-net-devices-<链路>.device`，链路每次出现都以 root 执行三条 `resolvectl`
（路径须 root 独占，否则只提示不安装）；`ConditionPathExists=/sys/class/net/<链路>/tun_flags`
使同名的非 TUN 链路不被改写，systemd 244 起失败后每秒重试（设备单元可能先于
systemd-resolved 认出新链路），次数受默认启动频率限制。只处理无需转义即可作单元名的链路名
（不含 `-`），内容未变时不再提权；关闭 TUN 或改了接口名后，同步时移除不再需要的单元
（无人值守时留到下次），卸载用户服务时全部移除。防火墙与 NetworkManager 钩子跳过
（watchdog 兜底），注册服务时尽力开启 linger，并仅在由 sboxkit 开启时于状态目录留标记，
卸载服务时据此 `disable-linger`（`sysd/linger.go`）。

`sysd.deployRuntimeAssets`（`sysd/runtime_deploy.go`）是安装与 `SyncAndRestart` 共用的
内核/geo 部署路径，因此 `sboxkit update`、每周定时器与「工具 → 更新」下载的新资产都会
进入运行中的服务（两种模式）。用户模式 + TUN 时新内核先以 `.new` 暂存并 `setcap`，再原子
改名；`execx.CanElevate` 判定无法提权（无终端且 sudo 需要密码）时保留旧内核、只更新规则与
配置，`sysd.WarnPendingCore` 在进入菜单时提示「重载服务」。

**Web 面板**：sing-box 没有官方面板可下载（不像 mihomo 有 metacubexd），因此
sboxkit 自己写了一个极简面板（`internal/uiassets`，go:embed 打进二进制，无需
联网下载）。面板走 sing-box 内置的 `experimental.clash_api.external_ui` 机制，
与 mihomo 版一致：`http://host:9090/ui/`，不额外占用端口、不需要独立的反向代理
进程——面板页面里的 JS 直接调用同源的 sing-box Clash API（`/proxies`、
`/proxies/{group}`、`/proxies/{name}/delay`）。`lan_panel` 定制字段只决定
`external_controller` 绑定 `127.0.0.1` 还是 `0.0.0.0`，不影响面板是否可用。
服务安装、重载和自更新会先在私有临时目录物化二进制内嵌资源，再经提权操作替换
`/var/lib/sboxkit-runtime/ui`；订阅重建不再改写共享状态目录，避免 root 周更把
`state/ui` 变成普通用户不可写并导致后续刷新失败。

**运行时资源随版本同步**：服务单元、bootstrap DNS relay、面板、内核与规则集、运行时
配置都由部署它们的 sboxkit 版本生成。每次 `SyncAndRestart` / `Install` 成功后把当前版本
记入 `state/runtime.version`（0644）。交互启动时
`sysd.RuntimeNeedsReconcile` 只比较这条记录与当前版本：不同（自更新、apt 升级或换包后的首次
运行，或尚无记录的旧部署）时系统模式启动即提权，由 `sysd.ReconcileRuntime` 整体
`SyncAndRestart` 一次；相同则什么都不做，也不请求授权。服务未安装或已暂停时不同步（避免
顺带拉起暂停的服务）；同步失败不记版本，下次启动重试。

**系统模式权限模型**：root 运行的服务与定时器只执行、只读取 root 属主的文件。旧版本把
`/var/lib/sboxkit` 交给调用用户，而 watchdog（每 2 分钟）与周更定时器以 root 执行经其中
版本库解析的 `/usr/bin/sboxkit`、读取其中的设置与日志——该用户身份下的任意进程都能借此
免密拿到 root。现在：

- 改动状态或服务的子命令（`init` / `modify` / `update` / `pause` / `uninstall` 等，见
  `cmd/sboxkit/privilege.go`）在系统模式下启动时经 `execx.ReexecAsRoot` 以
  `sudo [SBOXKIT_*=…] -- <调用路径> 参数…` 整体重新执行；之后的特权操作直接执行，
  不再中途逐条 sudo。`nettest` / `diagnose` / `version` 以调用用户运行。经 sudo 重新执行后
  仍不是 root（sudoers 的 `runas_default` 指向他人等）时报错而不循环：判断依据是 `SUDO_COMMAND`
  正是 sboxkit（`execx.reexecutedBySudo`）。从 sudo 打开的 root shell 里 `su` 回普通用户残留的
  `SUDO_*` 记的是那个 shell，照常提权；不设 `SUDO_COMMAND` 的实现按已经过 sudo 处理。
- 交互菜单先以调用用户打开（只读状态、不写日志、不做事务快照）：状态、连接信息、节点热切换
  （Clash API）、网络测试、诊断与日志不需要密码。改动入口调用 `flows.RequireRoot(entry)`：
  先 `sudo -v`（失败或取消时留在原菜单），再以 `--resume=<入口>` 经 sudo 重新执行；root
  进程把入口拆成菜单键路径，由 `flows.MenuSelect` 逐层消费，直达该操作，本次会话随后保持
  root。`flows.LaunchNeedsRoot` 决定哪些情形启动即提权：状态缺失（首次使用）、状态不归 root
  （旧版本）、当前用户读不到状态（不在管理员组）、服务未安装、运行时版本待同步、网络自愈待修复。
- 状态树属主 root、属组为管理员组（`paths.AdminGroup`：`sudo` → `wheel` → `admin`，都没有时
  为 root 组），目录 setgid 使新文件继承属组；含凭据的文件 0640、订阅目录 0750
  （`Paths.SecretFileMode` / `SecretDirMode`，用户模式为 0600 / 0700）。管理员组本可 sudo，
  组读权限不扩大可见范围，却让日常浏览免去密码。
- root 入口（含 watchdog 的 `healthcheck`）先调用 `paths.SecureTree` 把整棵状态树收归
  root：目录先改属主再列内容；非 root 属主、组/其他可写或多硬链接的文件以新 inode 重建，
  旧写句柄随之失效；只差属组的 root 文件原地改组；指向树外的符号链接与 FIFO 等特殊文件删除。
  已合规时只做一遍 lstat。收归失败时该次运行不写日志；日志以 `O_NOFOLLOW` 打开、裁剪经
  临时文件改名，不经路径重写。
- 主服务默认以 `DynamicUser=yes` 运行（`sysd/identity.go`）：配置经
  `LoadCredential=config.json:<运行时配置>` 由 systemd 以 root 读取后交付，`ExecStart` 读
  `${CREDENTIALS_DIRECTORY}/config.json`；`ProtectSystem=strict` 等加固下唯一可写的是
  `StateDirectory=sboxkit-cache`，只保留 `CAP_NET_ADMIN` / `CAP_NET_BIND_SERVICE` /
  `CAP_NET_RAW`。订阅或原生配置里的文件路径因此无法借 root 读写任意文件；系统模式还会丢弃
  `log.output`（日志统一进 journal）。以下情形保留 `User=root` 并提示原因：配置含
  `process_name` / `process_path` / `process_path_regex`（识别其他用户的进程要读其 `/proc`）、
  systemd 早于 247（无 `LoadCredential`）、本版本切换后服务 4 秒内未保持 active 或期间发生
  自动重启（只比较重启后这段时间的 `NRestarts`：等待自动重启时 `systemctl restart` 不清零它），
  而改用 root 后能稳定运行（`verifyIdentitySwitch` 在 `state/runtime.identity-fallback` 记下
  版本，版本变化后再试）。root 同样起不来说明故障与身份无关：恢复非特权单元、不记录，并报错让
  这次部署回退到原配置。
  `bypass_process_names` 默认为空，启用 TUN 的默认部署因此同样以非 root 运行：Tailscale 给
  自身流量打 fwmark，由其优先级 5210–5270 的策略路由走主路由表，先于 TUN 自 9000 起的规则；
  tailnet 网段已在 `route_exclude_address` 中排除。旧部署的 `customize.json` 保存了旧默认值
  （tailscale、tailscaled），升级不改写，清空后即不再需要 root。
  bootstrap DNS relay 仍以 root 运行并自带沙箱。
- TUN 的系统 DNS（`sysd/tundns.go`、`internal/tundns`）：sing-box 以 root 运行时，TUN 启动后经
  `org.freedesktop.resolve1` 把链路 DNS 设为 TUN 网段内的下一个地址（`172.19.0.1/30` →
  `172.19.0.2`）、路由域 `~.`、默认 DNS 路由；以 `DynamicUser` 运行时 systemd-resolved 拒绝这些
  调用且 sing-box 不报错，系统解析仍走物理网卡上的 DNS，单元却保持 active。非特权单元因此为每个
  TUN 入站加一行 `ExecStartPost=+/usr/bin/sboxkit tun-dns --dev <链路> --dns <地址>`：`+` 使其以
  root、不受沙箱限制执行；systemd-resolved 未运行（也不在启动中）时直接退出，否则等链路启用后依次
  执行 `resolvectl dns` / `domain ~.` / `default-route yes`，失败重试至多 1 分钟（短于默认
  `TimeoutStartSec`，单次调用限时 10 秒），仍失败则以非零退出，使服务启动失败。`systemctl restart`
  会等 `ExecStartPost` 结束，钩子失败表现为重启命令本身报错；切换身份时 `verifyIdentitySwitch` 把这种
  报错与未保持 active 同样处理，改回 root 重试。DNS 地址按 sing-tun 的规则
  推导（首个 IPv4、IPv6 地址各自的下一个地址）；TUN 入站未显式指定合法的 `interface_name` 或
  地址段内没有下一个地址时，改用 root 由 sing-box 自行登记；含多个 TUN 入站时同样改用 root（每个
  钩子依次各等至多 1 分钟，合计可能超出启动时限）。接口名可能出自原样信任的原生订阅，钩子与
  watchdog 因此只登记内核导出 `tun_flags` 的 TUN 设备，不改写物理网卡等其他链路的 DNS。
  链路随 sing-box 退出而消失，systemd-resolved 同时丢弃这些设置，无需撤销。`SyncAndRestart` 在
  非特权单元内容有变（从 root 切换，或 TUN 钩子变化）时都做一次 `verifyIdentitySwitch`。钩子与 sing-box 自己都只在启动时登记
  一次，系统模式的 watchdog 因此每轮先核对各 TUN 链路的 `resolvectl dns|domain|default-route`，
  缺失（例如被 `resolvectl revert` 或 NetworkManager 改动）时补登记而不重启 sing-box
  （`tundns.Ensure`，日志 `classification=tun_dns action=reregistered`），不论 sing-box 以哪种身份
  运行。用户模式见本节前文。
- 系统单元以 root 执行写入其中的 sboxkit（bootstrap relay、watchdog、周更定时器、TUN DNS 钩子），
  路径取启动 sboxkit 的调用路径（`StableExecutablePath`）。`sudo ./sboxkit` 从家目录启动时，该用户
  之后替换这个文件即可借这些单元以 root 运行任意程序；`sysd.serviceExecutable`
  （`sysd/service_exe.go`）因此要求路径本身、解析符号链接后的文件以及两者的每一级上级目录都归
  root 且组和其他人不可写，否则拒绝写入单元并提示改用 deb 或 `/usr/local/bin`。检查在
  `prepareRuntime` 阶段完成，以 `PrecheckError` 返回，运行时与服务保持原样、事务不重同步；用户
  单元以用户自身运行，不做此检查。
- root 代 `SUDO_USER` 写入其家目录（`~/.bashrc` 代理变量）时不跟随符号链接，只改写该用户
  自己的普通文件（`proxyenv.writeOwnedFile`）；`~/` 展开同样指向 `SUDO_USER`。

运行时目录、bootstrap relay 端口及 watchdog 单元是共享单例，因此服务变更入口只接受
主服务名 `sing-box`；不会伪装支持多个名称后让实例互相覆盖配置或争抢端口。

## 4. 交互契约：esc 保存 / ^R 回退 + 事务

与 clashdock 版完全一致（TUI 交互层不因内核类型而变）。TUI（`internal/tui`，
Bubble Tea）提供四类阻塞式提示：Select / MultiSelect / Ask / Confirm。每次调用
运行一个内联 `tea.Program`，因此流程层保持命令式结构。键位契约：

- **esc → ErrSaveExit**（保存并返回上层）
- **^R → ErrCancelled**（回退并返回）；`errors.Is(ErrSaveExit, ErrCancelled)` 成立
- 数字键跳选；菜单重入时光标停在上次选中项（`Initial`）
- 非 TTY 自动回退编号列表 + 文本输入，脚本可喂答案

事务（`internal/txn`）承载回退语义：`更改配置` 会话进入时快照
config.json / active / customize.json / subscriptions/，esc 提交、^R 整体还原并把
运行中的服务重新对齐；`初始化`（`flows.Init`）拆成多个各自独立的事务（部署设置 /
添加订阅 / 注册服务 / 网络自愈 / 每周更新定时器），每个事务只登记并回退自己范围内
的 undo（删订阅 / 卸服务 / 撤防火墙），某一步取消或出错只回退它自己，不会连带撤销
更早已经提交的步骤——例如服务一旦注册启动成功，后续「顺带更新内核/geo 数据」
失败只会警告，不会把已经跑起来的服务卸载回退。系统类操作（更新内核 / 重启服务等）
标注「※即时」，不参与会话回退（各自在内部事务里原子完成，见下）。

**部署前的兼容性闸门**（`internal/compat` + `internal/deploy`）：生成、导入或升级得到的
候选配置，都先在私有临时目录（0700，含节点凭据，校验后删除）里用**准备实际部署**的
sing-box 内核与规则集执行 `sing-box check`，通过后才写入状态目录并同步服务。拒绝信息
带内核版本与处理方向（已弃用 / 移除的写法 → 更新 sboxkit 后重新生成；内核尚不支持的
写法 → 更新内核）。

- 订阅添加 / 刷新 / 重建 / 切换与「本地文件覆盖」导入：候选配置在内存中生成，用 state
  现用内核校验；通过后经 `deploy.Apply` 一个事务写入订阅原文 / 生成配置 / meta /
  config.json / active 指针并 `sysd.SyncAndRestart`。任一步失败都还原这些路径；同步失败
  发生在运行时被改动之后时，再按还原后的状态重新同步，运行时文件与服务回到改动前的组合。
  首次初始化下载内核前无法校验，推迟到注册服务时校验。校验失败时按内核输出给出处理方向：
  旧内核不认识新写法（`unknown DNS rule action` 等）提示更新内核，新内核弃用 / 要求新写法
  （`require match_response` 等）提示更新 sboxkit 后重新生成。
- `sysd.Install` / `SyncAndRestart`：前置检查 → 暂存运行时配置 → 用部署后服务实际运行的
  内核（用户模式 + TUN 推迟切换内核时仍是运行时旧内核）校验 → 提权，全部通过后才覆盖
  运行时文件、停止或替换旧服务。这一阶段的失败以 `sysd.PrecheckError` 返回，调用方据此
  不回退运行时；初始化「注册服务」的回退只卸载本次新建且已部署的服务。
- 内核 / 规则集更新（菜单、`sboxkit update` 定时器、`assets-update`、初始化后的自动更新）：
  最新版先暂存到 `state/downloads/staged-*`，同时校验现有生效配置与「当前生成器 + 新内核 +
  新规则集 + 生效订阅」，通过后才原子换入状态目录。生效配置不被接受、而重新生成的配置
  可通过时，交互模式询问是否一并重新生成；生成器本身不兼容时说明所需版本与影响，并询问
  是否同时更新相关组件：内核低于生成器要求（`compat.Report.NeedsNewerKernel`）时提议更新
  内核（只更新规则集时也一样），否则提议更新 sboxkit。无人值守、用户拒绝或补充组件不可用
  （如下载失败）时放弃更新，原组合继续运行。
- sboxkit 自更新 / 回退稳定版：见第 8 节。

主菜单原「更改配置」按「改动是否需要重启服务生效」拆成两个入口：`flows.ModifyConfig`
（订阅管理 + 定制层字段分组编辑，写的是 sing-box 运行配置本身）与 `flows.ModifyRuntime`
（节点切换 / 内核更新 / sboxkit 自更新 / 服务设置 / 网络自愈 / 更新定时器，均即时生效），
两者共享同一套 `modifySession` 快照 + 回退骨架。定制层不再有单独的「编辑定制层」中间层：
`config.DeploymentFields`（部署设置）与 `config.OverlayFields`（AI / 流媒体 / 地区自动
测速组）两个字段分组直接是 `ModifyConfig` 菜单下的平级项（`flows.EditFieldGroup`），退出
即保存本组已做的修改，外层会话的文件快照负责整体回退。交互式主菜单检测到主服务未注册
（`sysd.IsInstalled` 判定，已停止但单元文件仍在也算已注册）时会询问是否现在进行初始化，
而不是自动强制进入；检测到未注册时先触发语言选择（`flows.PickLanguage`），再问是否
初始化——语言选择本身不属于 `Init` 流程的一部分，因此也不受其回退语义影响。各级菜单选项按常用
程度排列（日常操作在前，卸载类低频/破坏性操作在后）；长提示语按终端宽度自动换行
（`tui.wrapText`）；菜单序号统一按整份菜单长度决定风格（`tui.numFor`：≤20 项整份带圈
数字，否则整份普通数字），不会出现同一菜单内前面带圈、后面变阿拉伯数字的情况。

**界面语言**（`internal/i18n`）：默认英文启动，主菜单「Language / 语言」可切中文，
写回 `customize.json` 的 `language` 字段（`SBOXKIT_LANG=en|zh` 环境变量可覆盖，
优先级更高）。源码里的中文原文本身就是翻译表的 key（`i18n.T(zh string) string`），
中文模式下原样返回，英文模式下查表翻译；地区/分组匹配关键词等「数据」而非界面文案
一律不翻译，以免破坏对真实订阅内容的匹配。翻译只在函数体内的使用点调用，绝不在包级
`var`/`const` 初始化里调用（那会在语言设置生效前把结果烤死成英文）。

## 5. 下载通道：直连优先 → 代理兜底

`internal/fetchx`：先探测直连（google generate_204），可达则显式绕过一切代理
（避免下载被静默隧道进本地 sing-box → 机场节点）；不可达才走配置的 `download_proxy`。
支持重试、Range 续传、gzip/tar 完整性校验、GitHub 镜像前缀与 API Token。

**下载完整性**（`kernel/integrity.go`）：期望摘要一律直接向 `api.github.com` 查询，从不经
镜像——镜像只负责传输。发行资产（sing-box 内核、sboxkit 发行包）比对 release API 的
`digest`（`sha256:<hex>`）；`raw.githubusercontent.com` 上的 geo 规则集比对 contents API
给出的 git blob 对象名（`sha1("blob <len>\0" + 内容)`）。命中的缓存同样校验，不符即重新下载；
与摘要相符的缓存就是这次要下载的内容，强制更新（`assets-update`）也直接复用。
拿不到摘要时：经镜像下载直接拒绝；直连 GitHub 时只依赖 TLS 继续（sboxkit 发行包再退回同一
发行版的 `checksums.txt`）。系统模式缓存的 .deb（回滚、撤销更新时使用）每次使用前同样比对
API 摘要。一键安装脚本 `scripts/install.sh` 遵循同一规则，并把 .deb 复制到 root 独占的临时
目录再次校验后才交给 dpkg（下载目录属于调用用户，sudo 等待密码期间文件可能被替换）。

## 6. 离线种子（.deb / 用户模式安装包）

.deb 与用户模式安装包内置同一份 sing-box 内核与基础 geo 规则集（`geosite-cn.srs` +
`geoip-cn.srs`），装包/解压即可离线初始化：`kernel.SeedFromSystem` 在 state 缺失
对应文件时从 `/usr/libexec/sboxkit` / `/usr/share/sboxkit/ruleset`（用户模式为包内
`core/` / `ruleset/`）复制接管。用户模式初始化后不再自动联网更新；只含二进制的
`sboxkit_*.tar.gz` 精简包保持小体积，专供自更新下载。与 mihomo 版
的三件套（geoip.metadb/geosite.dat/country.mmdb）不同，sing-box 用 sing-geosite
/ sing-geoip 项目发布的 `.srs` 规则集，一国家一份文件，无 MaxMind EULA 之类的
新鲜度义务，可以放心冻结进安装包；仍支持在线『更新 geo 数据』获取最新版本。
发布流水线每次都强制重新下载 sing-box 最新稳定版，并执行二进制版本一致性校验；
内核版本文件随 deb 打包，首次接管时会准确写入 state，而非仅标记为 `bundled`。
随后使用该内核对默认 TUN、FakeIP 与 hosts 代表配置（含、不含随包 geo 规则集）执行
`sing-box check`，同时运行全部 Go 测试和静态检查；任何配置破坏性变更都会在创建
GitHub Release 前阻断流水线。

## 7. 自愈与守护

- `sing-box-bootstrap-dns.service` 是只监听 `127.0.0.1:6450` 的 UDP/TCP relay。
  它按 `customize.json` 中的有序 IP 字面量上游直连转发，默认 AliDNS TCP →
  Cloudflare DoH；每次拨号动态选择非 TUN 默认物理接口，并通过
  `SO_BINDTODEVICE` 绕过 TUN/代理循环。失败上游进入短冷却，随后半开恢复。
  sing-box 的 `dns-direct`、节点域名及 route resolver 只依赖该 loopback relay。
  relay 单元（系统/用户）的 `RestrictAddressFamilies` 必须包含 `AF_NETLINK`：Go 的
  接口查询走 netlink，缺少时每次转发都在拨号前失败并返回 SERVFAIL；
  `sysd/bootstrap_sandbox_linux_test.go` 以 seccomp 复现该限制并运行真实发现路径。
  relay 以 `PartOf` 跟随主服务启停：身份回退、事务回退这类连续重启会让它撞上 systemd 的
  启动频率限制，主服务随之因 `Requires` 起不来；sboxkit 重启主服务前因此先对两者
  `reset-failed`（watchdog 的重启仍受自身冷却 / 熔断约束，不做此清理）。
- NetworkManager dispatcher 钩子：真实网卡 up / 连通性变化时防抖触发
  `sboxkit healthcheck --event network-change`（忽略 tun 自身），不再绕过统一限流器直接重启。
- watchdog 定时器 + `sboxkit healthcheck` 先补登记丢失的 TUN 系统 DNS（系统模式，见第 3 节），
  再依次判断物理上行、bootstrap DNS、当前运行时
  selector 实际选中链路的节点入口解析、mixed 端口和端到端代理，并输出稳定故障分类。
  未使用节点不会触发误重启；物理上行故障不重启；其它失败采用跨 timer
  invocation 的指数退避和滑动窗口熔断，默认 10 分钟最多重启 2 次，连续两轮健康后复位。
- 每个故障 incident 只保存第一份有界、脱敏快照；固定白名单命令并发且各自限时/限量。
  `sboxkit diagnose [--json]` 与工具菜单只读取已有状态和快照，不触发探测、修复或重启。
  状态与快照刻意保持本机普通用户只读（0644），内容只含脱敏后的网络拓扑、分类和计数；
  这是无需 sudo 即可诊断的产品取舍，多用户主机应视其为本地可见信息。
- `customize.json` 的 `enable_resilience` 保存用户意图；完整性检查分别观察 dispatcher、
  watchdog service、watchdog timer 及 timer 的 enabled/active 状态。启用时，初始化、
  服务安装/重载、更新和交互启动会幂等补齐缺失或陈旧组件；显式禁用时不自动安装。
- watchdog unit 保留用户调用的稳定可执行路径（包括 `/usr/bin/sboxkit` 这类更新符号
  链接），不会绑定即将被版本清理删除的真实目标；修复时保留已配置的探测间隔。系统模式下该路径
  须 root 独占（见第 3 节），从不可信位置启动时修复失败并提示，主服务不受影响。
- 暂停先停止 watchdog 与更新 timer，再停止主服务；启动先恢复主服务，再按当前偏好
  reconcile watchdog，历史残留或显式禁用的 timer 不会被直接拉起。

## 8. sboxkit 自更新

**系统模式（dpkg 安装的 `/usr/bin/sboxkit`，`selfupdate/debpkg.go`）**：不再改写 dpkg 管理的
文件，而是下载同版本的 `sboxkit_<version>_linux_<arch>.deb`（按 release API 摘要校验），用
纯 Go 解析 ar + `data.tar.gz` 取出 `usr/bin/sboxkit` 试跑并做兼容性校验（同下），再经
`dpkg -i` 安装（dpkg 锁被占用时重试）；回滚即重新安装上一版本的 .deb（缓存优先，否则下载并
校验）。nfpm 把预发布后缀按 Debian 习惯写成 `~`（`0.4.1-beta.1` 记为 `0.4.1~beta.1`，排在
`0.4.1` 之前），读取 dpkg 记录的版本时换回发行版标签的写法，安装脚本判断「已安装」时反向换算。.deb 缓存在 `downloads/packages/`，只保留当前、上一个与 `last-stable` 记录的版本。
旧版本把 `/usr/bin/sboxkit` 接管成指向版本库的符号链接，dpkg 安装后它重新成为普通文件，
此时旧的 `sboxkit-versions/` 随之删除。包管理器的记录因此始终与实际文件一致，`apt remove`
/ 后续 `.deb` 升级照常工作。暂不提供 apt 仓库：签名仓库需要在 CI 中长期保管签名私钥并托管
索引，而 dpkg 自更新加 API 摘要已给出同等的完整性保证。

**其余情形（用户模式、非 dpkg 安装）**：版本化目录 `<state>/sboxkit-versions/<version>/sboxkit` +
`current` 符号链接方案，与 `internal/kernel` 更新 sing-box 内核/geo 数据完全独立
（更新的是 sboxkit 自身这个程序）。流程：查询最新 GitHub release → 下载对应架构的
`sboxkit_<version>_linux_<arch>.tar.gz` → 按 release API 摘要校验 SHA-256（见第 5 节）→ 解压到
独立版本目录 → 试跑新二进制（`sboxkit version`）确认能执行 → 兼容性校验 → 原子重写
`current` 符号链接 → 再次试跑确认成功；启动校验失败则回退 `current` 指向。兼容性校验由
旧版本调用新版本的内部子命令 `compat-check` 完成：新生成器为生效订阅重新生成配置，并以
即将使用的内核与规则集执行 `sing-box check`，结论以 `compat.Report`（JSON，字段只增不改）
交回。不兼容时说明所需版本与影响并询问是否同时更新内核，内核暂存后连同现有生效配置一起
复验；早于该机制、无法自检的版本需用户明确同意（默认否）。全部通过后在同一事务里切换
`current`、换入内核，并可选写入新生成器的配置后同步服务；用户拒绝或升级不可用时放弃，
旧版本与旧组合保持不变。回退稳定版走同一校验。首次自更新时若正在
运行的可执行文件（一般是 apt 装的 `/usr/bin/sboxkit`）还不是托管符号链接，会先把它
原样迁移进版本目录作为基线版本，再把该路径替换成指向 `current` 的符号链接（这一步
需要 root，走 `execx.RunRoot`；此后的更新只需重写 `current`，不再需要碰 `/usr/bin`）。
只保留 `current` 指向的版本、紧邻的上一个版本、以及 `last-stable` 记录的稳定版
（如果三者不同），其余版本目录清理掉。

两条更新渠道：稳定版查 GitHub `/releases/latest`（该接口天然排除 prerelease/draft）；
预览版查 `/releases` 列表第一项（不论是否标了 prerelease，即仓库里创建时间最新的
发行版）。`.goreleaser.yaml` 配了 `release.prerelease: auto`：tag 带 semver 预发布
后缀（如 `v0.1.7-beta.1`）会被 GoReleaser 自动标记为 GitHub prerelease，不占用
"Latest release"、也不会被 `/releases/latest` 返回。每次稳定渠道更新成功都会把
`current` 同时记到 `<state>/sboxkit-versions/last-stable`，供切到预览版后想
回退的用户一键切回（`PrepareStable` + 同上的兼容性校验）。稳定渠道查到的版本低于当前版本
（从预览版切回）时提示「切换到稳定版」而不是「发现新版本」；版本比较按数值，数值相同时正式版
高于其预发布版本（`selfupdate.Older`）。

## 9. 包结构

```
cmd/sboxkit           入口：子命令 + TUI 主菜单
internal/errs         ErrSaveExit / ErrCancelled 导航哨兵
internal/execx        日志、子进程、sudo
internal/paths        状态目录解析（SBOXKIT_HOME > 用户模式标记 > 固定目录）
internal/config       customize.json：类型化 Config struct，默认值 / 读写 / 字段元数据 / 脱敏
internal/bootstrapdns loopback UDP/TCP relay：物理接口直连、UDP/TCP/DoH 有序故障转移
internal/converter    Clash / sing-box 原生 / base64 → sing-box 配置的核心转换器
internal/txn          事务：BackupFile / Snapshot / TrackPath / AddUndo，LIFO 回滚
internal/compat       部署前兼容性校验：私有目录内以指定内核 / 规则集执行 sing-box check；Report 契约
internal/deploy       状态目录改动 + 服务同步的单一事务：失败还原状态，运行时已改动时按旧状态重新同步
internal/tui          Bubble Tea 四类提示 + 非 TTY 回退
internal/fetchx       HTTP 下载通道
internal/kernel       sing-box 内核 / geo 规则集下载、暂存后原子换入 + deb 种子接管
internal/uiassets     内置 Web 面板静态资源（go:embed，无需联网下载）
internal/selfupdate   sboxkit 自更新：系统模式经 dpkg 安装 .deb；其余为版本化目录 + 原子符号链接切换
internal/subscription 订阅域：fetch / detect（三态）/ b64（分享链接解析）/ manager
internal/sysd         主服务/relay/watchdog/更新单元，分层健康探测与持久熔断状态
internal/tundns       `sboxkit tun-dns`：代非 root 的 sing-box 向 systemd-resolved 登记 TUN 链路 DNS
internal/clashapi     Clash API：切组 / 并发测延迟（协议与 mihomo 版共用）
internal/firewall     ufw > firewalld > nft > iptables 探测与放行
internal/proxyenv     ~/.bashrc 代理变量标记块
internal/i18n         中英文界面文案（默认英文，源码中文原文即翻译表 key）
internal/flows        流程编排（init / modifyconfig / modifyruntime / tools（nettest+文件位置）/
                      uninstall / nodeselect（节点切换/固定节点）/ 定制层字段分组编辑 / 自更新）
```

## 10. 测试策略

- 协议转换层（`internal/converter`）以 **golden 对拍**锁定行为：
  `testdata/converter/` 下的 Clash YAML 夹具与期望输出，Go 测试要求语义等价
  （JSON 归一化后 DeepEqual）
- txn / config / paths / tui 渲染 / b64 分享链接解析为常规单元测试（`go test ./...`）
- TUI 交互用 tmux `send-keys` / `capture-pane` 逐屏对拍验证

## 11. 模块化约束

新增或修改代码时遵守 [docs/MODULARITY.md](docs/MODULARITY.md)，该约束覆盖**整个仓库**
而不仅是 Go：普通 Go 文件目标 200-400 行、流程文件目标 150-300 行，Web 前端脚本/样式、
文档、脚本、测试夹具各有对应红线；超过软上限时优先按职责拆分（Go 同包新文件、前端
`app.<concern>.js`），避免把交互流程、系统操作、下载逻辑、数据转换或前端行为继续堆到单个文件中。
