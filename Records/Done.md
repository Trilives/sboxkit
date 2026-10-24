# 已完成

## 2026-10-25：v0.4.0 稳定版

- 按需提权、dpkg 自更新、下载摘要校验、sing-box 非 root 运行与一键安装脚本，验收记录见下一节。
- 发布门禁：`gofmt`、`go vet`、`go test ./...`、`go test -race ./...`、带 sing-box v1.14.4 的真实内核用例、
  安装脚本测试、i18n 覆盖检查在 `machine.md` 主机通过，`goreleaser check` 与快照构建通过；提交的代码与主机上
  验证的 0.4.0-rc4 源码逐文件一致（此后只改了文档、记录与 `VERSION`）。
- 发布验证：推送 `v0.4.0` 标签后 release 与 CI 工作流成功；10 个资产（新增 `install.sh`）均与 `checksums.txt`
  一致；`/releases/latest` 与预览渠道（`/releases` 首项）都指向 v0.4.0，`releases/latest/download/install.sh`
  可取用。在 `machine.md` 主机用发布的脚本 `install.sh --user --version 0.4.0` 装到临时目录：SHA-256 校验通过，
  `sboxkit version` 为 0.4.0、随包内核 v1.14.4；解包（未安装）发布的 amd64 deb：摘要一致、数据段为
  `data.tar.gz`、版本 0.4.0、二进制含 `LoadCredential` 单元模板。临时目录与 `~/.local/bin` 链接已清理。
- 发布说明：release 工作流写入的是提交信息而不是标签说明——actions/checkout 最后以
  `+<sha>:refs/tags/<tag>` 取回触发的标签，附注标签被换成轻量标签（在主机上按任务日志中的 git 命令复现）。
  v0.4.0 的发布说明已改为标签内容；工作流改为先强制取回标签对象。v0.3.1–v0.3.3 的发布说明同样受影响，未改动。

## 2026-10-25：按需提权、dpkg 自更新、下载摘要校验、sing-box 非 root 运行与一键安装脚本（v0.4.0）

- 按需提权（用户选定「Only when changing」）：系统模式下交互菜单先以当前用户打开，查看状态、信息、日志、
  诊断、网络测试与实时切换节点无需密码；第一次选择改动操作时先 `sudo -v`，再以
  `sudo -- /usr/bin/sboxkit --resume=<入口>` 重新执行并直接进入该操作，此后整个会话保持 root；授权失败或
  取消时留在原菜单。状态缺失、不归 root、不可读、服务未安装、版本变化需同步运行时资源或自愈需修复时，启动即
  提权并说明原因。改动状态的子命令启动即提权，`nettest` / `diagnose` / `version` 以当前用户运行。状态目录
  归 `root:<管理员组>`（sudo / wheel / admin），目录 setgid，凭据文件 0640、目录 0750，TUI 日志裁剪后保持
  0640；状态目录没能收归 root 时 `healthcheck` 直接失败退出，不以 root 读取其中文件；系统模式下拒绝以 root
  使用指向普通用户属主目录的非默认 `SBOXKIT_HOME`。
- dpkg 自更新（`internal/selfupdate/debpkg.go`）：dpkg 安装的 `/usr/bin/sboxkit` 不再改写成指向版本库的
  符号链接。下载同版本 .deb 并按 API 摘要校验，以纯 Go 的 ar + data.tar.gz 解析解出包内二进制（体积上限
  256 MiB）试跑并做兼容自检，再 `dpkg -i`（dpkg 锁被占用时重试）；撤销更新与回退稳定版即重新安装缓存的
  .deb，缓存每次使用前重新比对 API 摘要，不符即删除并重新下载；清理时按语义版本保留当前、上一个与
  `last-stable`。旧版自更新留下的符号链接与版本库在第一次经 dpkg 安装后清理。apt 仓库经评估暂不提供
  （理由见 `docs/ARCHITECTURE.md` §8）。
- 下载摘要校验（`internal/kernel/integrity.go`）：内核与 sboxkit 发行包比对 release API 的 `digest`，
  geo 规则集比对 contents API 的 git blob 对象名；期望摘要只向 `api.github.com` 查询、从不经镜像。缓存同样
  校验；拿不到摘要时经镜像下载直接拒绝，直连只依赖 TLS。强制更新遇到与摘要相符的缓存直接复用（此前会
  重新下载且误报「丢弃无效缓存」）。
- sing-box 非 root 运行（`internal/sysd/identity.go`）：系统单元默认 `DynamicUser=yes`，配置经
  `LoadCredential` 交付，`ProtectSystem=strict`，唯一可写目录为 `StateDirectory=sboxkit-cache`，只保留
  `CAP_NET_ADMIN` / `CAP_NET_BIND_SERVICE` / `CAP_NET_RAW`；系统模式丢弃 `log.output`。配置含按进程分流、
  systemd 早于 247，或非 root 身份起不来而 root 可以时保留 root 并说明原因（后者在
  `state/runtime.identity-fallback` 记下版本）；两种身份都起不来时报错并回退到原配置。生成器只在启用 TUN
  时生成按进程直连规则（此前默认配置因 tailscale 规则总是以 root 运行）。sboxkit 重启主服务前先对主服务与
  relay `reset-failed`。
- 一键安装脚本 `scripts/install.sh`（随发行版发布并列入 `checksums.txt`）：检测架构、按 API 摘要校验（只在
  直连时退回 `checksums.txt`），把 .deb 复制到 root 独占的临时目录再次校验后交给 `dpkg -i`；`--user` 解压到
  `~/.local/share/sboxkit` 并在 `~/.local/bin` 建链接；支持 `--version`、`--mirror`（仅 https）、`--dir`、
  `SBOXKIT_MIRROR`，`GITHUB_TOKEN` 只发给 `api.github.com`；下载中断续传。CI 运行 `scripts/test-install.sh`。
- 清理：删除 31 条代码中已不存在的翻译条目；`.gitignore` 去掉 Python 时代的条目；README 预览图按当前中文
  界面重新截取（运行时管理不再含「更新」，工具新增故障诊断与更新）。
- 测试：新增 `selfupdate/debpkg_test.go`、`kernel/integrity_test.go`、`sysd/identity_test.go`、
  `flows/elevation_test.go`、`converter` 进程直连用例、`execx/log_test.go` 权限用例、`scripts/test-install.sh`
  等；`gofmt`、`go vet`、`go test ./...`、`go test -race ./...`、带 sing-box 1.14.4 的真实内核用例、安装脚本
  测试、i18n 覆盖（缺失 0、无效条目 0）在 `machine.md` 主机通过（源码与本机工作区逐文件哈希一致）；
  `goreleaser check` 与快照构建通过，`install.sh` 列入 `checksums.txt`，deb 的数据段为 `data.tar.gz`。
  主机未安装 shellcheck。
- 远程验收（`machine.md` 主机，系统模式，0.4.0 快照 deb）：
  - 安装脚本：系统模式装 v0.3.3 成功；sudo 等待密码期间替换下载文件被拒绝（「验证后包被改动，未安装」），无
    残留。`--user` 经主机代理 51 秒装好并校验 SHA-256，重复执行拒绝覆盖，随后移除。
  - 从 v0.3.3 升级：旧的用户属主状态由 root watchdog 收归为 `root:sudo`（setgid，凭据 0640）。首次运行说明
    「sboxkit 版本已变化」后提权并同步；再次运行免密进入浏览模式，普通用户实时切换节点成功。「固定节点」连续
    输错 3 次密码留在原菜单，输对后以 `--resume=runtime/pin` 直接进入固定节点。
  - 重新生成订阅后按进程规则消失，单元改为 `DynamicUser`（uid 61915，配置来自
    `/run/credentials/sing-box.service/config.json`），NRestarts 0，面板 200，节点选择保留，缓存迁到
    `/var/lib/private/sboxkit-cache`。
  - 摘要校验：直连 `assets-update` 26 分钟后摘要一致；本地真实镜像成功；镜像篡改内核被拒绝（期望与实际摘要
    均列出），只篡改 geosite 同样被拒绝，两次运行时内核与规则集都未改变、服务保持运行。
  - dpkg 自更新：按旧版自更新布局（`/usr/bin/sboxkit` → 版本库）从「工具 → 更新」进入（提权后直达
    `tools/update`），稳定渠道下载 v0.3.3 .deb 并校验 SHA-256、兼容自检后经 dpkg 安装；`/usr/bin/sboxkit`
    恢复为普通文件，版本库删除，.deb 缓存于 `downloads/packages/`，`last-stable` 为 0.3.3。重装快照后「回退
    到稳定版」直接使用校验通过的缓存；篡改缓存后改为重新下载（确认后中止：快照版保持、篡改的缓存已删除）。
  - TUN 非 root：原生配置直通（含 `log.output`）以动态用户运行，进程能力仅上述三项，`sbktest0`、策略路由与
    2022 路由表生效，访问 198.18.250.10 经 `inbound/tun` 进入，mixed 端口 204；`log.output` 被丢弃并提示，
    未创建日志文件。含 `process_name` 的配置保持 root 并说明原因。
  - 验收中发现并修复：systemd 255 在服务等待自动重启时 `systemctl restart` 不清零 `NRestarts`（实测 17 次后
    重启仍为 17），旧的健康判断要求为 0，使 root 回退被误判为同样失败；两种身份都失败时仍提示同步成功；连续
    重启使 relay 触发 `start-limit-hit`，回退后的同步报「A dependency job failed」。修复后复验：mixed 监听
    已被 sshd 占用的 22 端口（两种身份都失败）→ 报错并整体回退到原配置，主服务与 relay 运行、不记录回退；规则集
    只有 root 可读 → 非 root 失败、root 成功并记下 `0.4.0-rc4`；有记录时切换保持 root 并说明；删除记录后
    重新以动态用户运行，NRestarts 0。
  - 普通用户（清空 sudo 缓存）：信息、日志（journal 与 0640 的 TUI 日志）、故障诊断、网络测试、
    `diagnose --json` 均无需密码。非 TTY：管道浏览正常；管道选择改动操作时 sudo 无终端可读密码，留在菜单并正常
    退出；`sboxkit pause` 无终端时退出码 1、服务不变。
  - 卸载（全选）后 purge：状态、运行时、缓存目录（含 `/var/lib/private` 下的实体）、单元与 NM 钩子均删除，
    TUN 消失；仅 tmpfs 上的 `/run/sboxkit` 残留（与 v0.3.3 观察一致），手动删除。
  - 恢复：停止 tmux 与回环 HTTP 服务，删除测试目录、root 下的测试规则集与 Go 遥测目录；与验证前快照比对一致。
  - 未覆盖：篡改缓存后重新下载的完整过程（经验证的下载与 dpkg 安装已由首次自更新覆盖）；arm64 / armv7
    仅构建；用户模式服务本身（本轮只验证安装脚本的 `--user` 安装）。
- 已知限制：启用 TUN 且保留默认 `bypass_process_names`（tailscale）时仍以 root 运行；从 sudo 打开的 root
  shell 里 `su` 回普通用户时残留的 `SUDO_UID` 会干扰按需提权。见 [`Todo.md`](Todo.md)。

## 2026-10-24：系统模式特权边界收口——root 只执行、只信任 root 属主的文件（v0.4.0）

- 问题：`/var/lib/sboxkit` 经 sudo 创建后交回调用者属主，而 root 运行的 watchdog（每 2 分钟）与周更定时器
  执行经其中版本库解析的 `/usr/bin/sboxkit`、读取其中的设置并写日志（不带 `O_NOFOLLOW`）；该用户身份下的
  任意进程都能免密拿到 root。订阅原文与生成配置以 0644 存于 0755 目录，其他本地用户可读节点凭据。
- 方案（用户选定「启动时整体提权」）：系统模式下交互菜单与读写状态的子命令启动时经
  `sudo [SBOXKIT_*=…] -- <调用路径> 参数…` 以 root 重新执行（`cmd/sboxkit/privilege.go`、
  `execx.ReexecAsRoot`）；`version` / `help` / `diagnose` 保持普通用户，后者只读 `/run/sboxkit`，
  语言仅取 `SBOXKIT_LANG`。状态目录只归 root（0755；订阅 0700，生效配置 / `customize.json` / 日志 0600）。
- 迁移：每个 root 入口（含 watchdog 的 `healthcheck`）在读取状态前调用 `paths.SecureTree`：目录先改属主再列
  内容；非 root 属主、组/其他可写或多硬链接的文件以新 inode 重建（旧写句柄失效）；指向树外的符号链接与
  FIFO 等删除；已合规时只做一遍 lstat。收归失败的那次运行不读 `customize.json`、不写日志。日志以
  `O_NOFOLLOW` 打开、裁剪经临时文件改名。root 代 `SUDO_USER` 改写 `~/.bashrc` 时拒绝符号链接、硬链接与
  非本人文件（`proxyenv.writeOwnedFile`），`~/` 展开指向 `SUDO_USER`。移除「也可以直接用 sudo 启动」提示。
- 测试：新增 27 个用例（`paths/secure_test.go`、`execx/elevate_test.go`、`execx/log_test.go`、
  `proxyenv`、`flows/system_state_test.go`、`cmd/sboxkit/privilege_test.go`）；`gofmt`、`go vet`、
  `go test ./...`、`go test -race ./...`、带 sing-box 1.14.4 的真实内核用例、i18n 覆盖检查（缺失 0）在
  `machine.md` 主机通过（源码与本机工作区逐文件哈希一致）；本机只做编译与 `git diff --check`。
- 远程验收（`machine.md` 主机，系统模式，TUN 关闭，本地 Clash 订阅文件）：
  - v0.3.3 复现（发布 deb）：普通用户开启 `enable_file_log` 并把 `sboxkit.log` 换成指向 `/etc` 下新文件的链接，
    同一次 watchdog 运行由 root 在 `/etc` 创建该文件并写入日志；按自更新布局让 `/usr/bin/sboxkit` 解析进版本库
    （`ln` 一步按旧版自更新经 sudo 执行）后替换版本库二进制，watchdog 以 `uid=0(root)` 执行了用户的脚本。
  - 自更新升级路径：普通用户按旧版自更新写入新构建并切换 `current`，并预埋日志链接、FIFO 与持有
    `config.json` 写句柄的进程；新二进制的首次 watchdog 运行即把整棵树收归 root，链接与 FIFO 删除、
    `/etc` 目标未创建，`config.json` 换为新 inode（旧句柄写入落在 `(deleted)` 孤立 inode，生效配置不含注入
    内容）。此后普通用户改日志 / `current` / 版本库二进制 / `customize.json`、读订阅与配置、建 FIFO 均被拒绝。
  - `.deb` 升级路径：状态改回用户属主并预埋日志链接后 `dpkg -i` 快照包，下一次 watchdog 完成同样的收归。
  - 交互：tmux 真实 TTY 下普通用户运行 `sboxkit` 提示后请求一次 sudo，菜单以 root 运行，版本变化同步、
    工具 → 信息正常；管道非 TTY 运行正常退出；`NO_COLOR=1 SBOXKIT_LANG=zh` 经 sudo 透传（首轮把变量放在
    `--` 之后导致 sudo 将其当作命令，已修正并复验）；全新安装创建 `root:root` 状态目录。
  - 恢复：卸载（全选）并 purge，删除状态 / 运行时 / `/run/sboxkit`、测试目录、复现产物、tmux 会话与 Go 遥测
    目录，与验证前快照逐项比对一致。
- 已知限制（当时）：自更新升级窗口、收归前已被篡改的内容会被保留、内核下载未校验——均在同一版本中处理，
  见上一节。发布前又按用户选择改为「按需提权」，状态目录改为 root 属主、管理员组可读（0640 / 0750），
  本节的「启动时整体提权」与 0600 / 0700 权限已被取代。

## 2026-10-24：v0.3.3 稳定版

- sboxkit 版本变化（自更新、回退、apt 升级或换包）后首次交互运行，整体同步一次全部运行时资源（服务单元、
  bootstrap DNS relay、Web 面板、内核与规则集、运行时配置）；版本未变时不读 root 专属运行时目录、不请求
  授权，修复系统模式下每次运行都提示「检测到内置 Web 面板有更新」并要求 sudo。验收记录见下一节。
- 发布门禁：`gofmt`、`go vet`、`go test ./...`、`go test -race ./...`、带 sing-box v1.14.4 的真实内核用例、
  `git diff --check`、i18n 覆盖检查在 `machine.md` 主机通过；自 v0.3.2 起 `.goreleaser.yaml`、工作流、打包脚本
  与依赖未变，构建由 release 工作流完成。
- 发布验证：推送 `v0.3.3` 标签后 release 与 CI 工作流成功（CI 随包 sing-box v1.14.4，带真实内核跑全部测试）；
  9 个资产均与 `checksums.txt` 一致；`/releases/latest` 与预览渠道（`/releases` 首项）都指向 v0.3.3。在
  `machine.md` 主机解包（未安装）发布的 amd64 deb：`sboxkit version` 为 0.3.3、随包内核 1.14.4，二进制含版本
  变化同步提示、不再含旧的面板过期提示；临时目录已清理。

## 2026-10-24：版本变化后整体同步运行时资源（修复每次运行都提示更新 Web 面板）

- 根因：系统模式运行时目录 `/var/lib/sboxkit-runtime` 为 `root:root 0700`，交互启动时以普通用户比对
  `ui/` 内容必然读取失败，面板每次都被判为过期，于是每次运行都请求 sudo 并重写面板。
- 修复（`internal/sysd/runtime_version.go`）：`SyncAndRestart` / `Install` 成功后把当前 sboxkit 版本记入
  `state/runtime.version`（0644）；交互启动的 `sysd.ReconcileRuntime` 只比较该记录与当前版本，不同时整体
  同步一次（服务单元、bootstrap DNS relay、Web 面板、内核与规则集、运行时配置），相同则不读运行时目录、
  不请求授权。服务未安装或已暂停时不同步；同步失败不记版本，下次启动重试。移除面板专用的
  `RefreshPanelIfStale`；自更新 / 回退成功提示补充「下次运行自动同步运行时资源」。
- 测试：新增 `runtime_version_test.go`（升级后同步一次、无记录的旧部署、同版本不动、未安装 / 暂停跳过、
  失败重试、记录可读）；`gofmt`、`go vet`、`go test ./...`、`go test -race ./...`、带 sing-box 1.14.4 的真实
  内核用例、`git diff --check`、i18n 覆盖检查通过（本机与 `machine.md` 主机各跑一遍）。
- 远程验收（`machine.md` 主机，系统模式，v0.3.2 deb，TUN 关闭、合成订阅经本机回环 HTTP 提供）：
  - v0.3.2 复现：连续三次运行都提示「检测到内置 Web 面板有更新」并请求 sudo，`ui/index.html` 每次被重写；
    普通用户对 `/var/lib/sboxkit-runtime/ui` 为 `Permission denied`。
  - 换入新构建（0.3.3-dev，模拟 apt 升级）：首次运行提示「sboxkit 版本已变化（? → 0.3.3-dev）」，授权一次后
    单元、运行时配置、bootstrap DNS、面板、内核与规则集全部重新部署，主服务与 relay 重启，记录写为
    0.3.3-dev；此后两次运行（清空 sudo 缓存）直接进菜单，无提示、无授权、文件与服务启动时间不变。
  - 暂停服务后换入 0.3.3-dev2：运行不同步、服务保持停止；恢复后再运行同步一次（0.3.3-dev → 0.3.3-dev2）。
    同步时拒绝授权则记录不变，下次启动重试成功。root 写入的记录（模拟周更定时器）可被普通用户的同步替换。
  - 恢复：经 sboxkit 卸载（全选）后 purge deb，清除空状态目录、`/run/sboxkit`、测试目录、回环 HTTP 与 Go
    遥测目录；与验证前快照比对一致（防火墙哈希仅计数器变化，无 sboxkit 规则；无残留监听端口）。
  - 观察：卸载「清理所有订阅与配置」后仍留下空的 `/var/lib/sboxkit` 与 tmpfs 上的 `/run/sboxkit`。

## 2026-10-24：v0.3.2 稳定版

- 生成的 DNS 地址过滤迁移到 sing-box 1.14 响应匹配（`evaluate` + `match_response` + `respond`），修复含 hosts
  CNAME / FakeIP + `geoip-cn` 的订阅在 1.14.x 下 `check` / 启动失败；生成器自此要求 sing-box ≥ 1.14.0。
- 导入、刷新、重建、切换订阅以及内核 / 规则集 / sboxkit 更新前，先用即将部署的内核与规则集执行 `sing-box check`；
  不兼容时说明所需版本与影响并提议同时更新相关组件，拒绝或无法升级时保持原组合运行；服务同步的校验前移到
  覆盖运行时文件、停止旧服务之前。验收记录见下方两节。
- 发布门禁：`gofmt`、`go vet`、`go test -race ./...`、带随包 sing-box v1.14.4 的 `go test ./...`（含真实内核用例）、
  脚本 `bash -n`、`goreleaser check` 与三架构快照构建（tar.gz / deb / 用户模式包）通过；快照用户模式包与 deb 的
  随包内核为 v1.14.4，包内二进制含 `compat-check`。DNS 迁移提交单独检出后 `go test ./...`（1.14.1 / 1.14.3）通过。
- 发布验证：推送 `v0.3.2` 标签后 release 工作流成功（CI 随包 sing-box v1.14.4）；9 个资产均与 `checksums.txt`
  一致；`/releases/latest` 与预览渠道（`/releases` 首项）都指向 v0.3.2。删除 18 个历史预发布及其标签，
  仓库仅保留 `main` 分支与 13 个稳定版标签。

## 2026-10-24：导入、重建和更新前校验配置与内核兼容性

- 部署前闸门（`internal/compat`、`internal/deploy`）：订阅添加 / 刷新 / 重建 / 切换与「本地文件覆盖」
  导入先在内存生成候选配置，用即将部署的内核与规则集在私有临时目录 `sing-box check`（`external_ui`、
  `cache_file` 重定向到校验目录，规则集路径换成暂存文件，相对路径先转绝对）；通过后才经
  `deploy.Apply` 一个事务写入订阅原文 / 生成配置 / meta / `config.json` / active 指针并同步服务。
  失败时提示内核版本、校验输出与处理方向（旧内核不认识新写法 → 更新内核；新内核弃用 / 要求
  `match_response` → 更新 sboxkit 后重新生成）。状态文件改为同目录临时文件 + 改名的原子写入。
- 服务注册 / 同步：前置检查 → 暂存运行时配置 → 用部署后实际运行的内核校验 → 提权，全部通过后才
  覆盖运行时文件、停止或替换旧服务；这一阶段失败以 `sysd.PrecheckError` 返回，不删除既有 unit 或
  运行时文件。运行时已被改动后的失败（如重启失败）先还原状态路径，再按还原后的状态重新同步；
  重新同步也失败时一并报告。「重新初始化」不再预先卸载服务。
- 组件更新（`internal/flows/upgrade.go`、`internal/kernel/stage.go`、`internal/selfupdate/prepare.go`）：
  内核 / 规则集先暂存到 `state/downloads/staged-*`，sboxkit 新版本先解压到独立版本目录；校验
  「生成器 + 内核 + 规则集 + 生效订阅」组合（换 sboxkit 时由旧版本调用新版本的内部子命令
  `compat-check`，以 `compat.Report` JSON 交回）以及现有生效配置在新组合下能否启动。不兼容时说明
  所需版本与影响，并按方向询问是否同时更新内核或 sboxkit、是否重新生成生效订阅；无人值守、用户
  拒绝或补充组件不可用时放弃，原组合继续运行。全部通过后在一个事务里换入组件、写入重新生成的配置
  并同步服务，失败整体回退。早于该机制、无法自检的 sboxkit 版本需用户明确同意（默认否）。
- 测试：`go test ./...`、`go test -race ./...`、`go vet`、`gofmt`、`git diff --check`、i18n 覆盖检查通过；
  真实内核用例（`SING_BOX_BIN` 指向 sing-box 1.14.1 / 1.14.3）复现 v0.3.1 Hua 配置被拒、切换被拒、
  当前生成器重建通过后才部署。
- 远程验收（`machine.md` 主机，Ubuntu 24.04 用户模式，合成 Hua 订阅：hosts CNAME + FakeIP + `geoip-cn`）：
  基线为 v0.3.1 `init`（无 TUN）+ 随包内核 1.13.21，服务运行；sing-box 1.14.3 对该配置 `check` 报
  `dns rule[1]: Response Match Fields … require match_response to be enabled`，复现失败。
  - 换入新构建（0.3.2-dev）、内核仍为 1.13.21：重建、刷新、添加（设为生效）、「本地文件覆盖」导入、
    切换到含 1.14 写法配置的订阅均被拒绝并给出 `unknown DNS rule action: evaluate` 与处理方向；主服务
    PID、运行时 / 状态配置哈希、active 指针与订阅目录哈希均不变，mixed 端口持续可用。
  - `sboxkit update`（非交互）：暂存 1.14.4 后判定现有生效配置被拒、需重新生成，放弃并退出 1，原组合
    继续运行；下载失败（GitHub EOF）同样退出 1、不改动任何文件。
  - 交互（纯文本模式）：只更新规则集时说明生成器要求 ≥ 1.14.0 并提议同时更新内核；暂存 1.14.4 后
    拒绝重新生成即放弃，内核、规则集、配置均不变；补充内核查询失败（HTTP 401）时同样放弃并说明原组合不变。
  - 交互（TTY TUI）：更新内核并同意重新生成后，1.14.4 与新配置一起换入并重启；运行时配置经 1.14.4、
    状态配置经 1.14.3 真实 `check` 通过，日志无错误。随后切换、重建与导入兼容配置均先校验再部署。
  - sboxkit 自身：从 0.3.2-dev「更新到稳定版」下载 v0.3.1 并通过 SHA-256；v0.3.1 无 `compat-check`，
    询问（默认否）后拒绝，仍为 0.3.2-dev，版本目录已清理。`compat-check` 实机：对 1.13.21 报
    `该生成器要求 sing-box ≥ 1.14.0`，对 1.14.3 为 compatible（相对路径参数同样可用）。
  - 验收中发现并修复（均先补失败用例）：真实内核输出未被归类，处理方向为空；`compat-check` 相对路径
    在校验目录下解析错位；只更新规则集时提议更新 sboxkit 而非内核；补充组件不可用时未说明原组合不变。
  - 主机网络较慢：内核与 sboxkit 发行包在本机下载、校验后上传，经测试状态内的回环 `github_mirror` 提供。
  - 现场恢复：停用并删除 sing-box / bootstrap-dns 用户单元及 wants 链接，`daemon-reload`；删除测试目录、
    `$XDG_RUNTIME_DIR/sboxkit`、回环镜像进程与 tmux 会话。`~/.bashrc`、`~/.profile` 哈希、`~/.local/bin`、
    crontab 与 linger 均与验证前一致。
- 已知限制：初始化「注册服务」若重装既有服务后校验通过、但重启失败，之前步骤已提交新状态，没有
  可恢复的旧组合，只报告失败。尚无含 `compat-check` 的稳定版发布，「新版本自检通过后切换」只由单元测试
  与 `compat-check` 实机输出覆盖；实机只验证了无法自检时的拒绝路径。

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
