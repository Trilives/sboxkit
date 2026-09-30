# 命令参考

sboxkit 的**推荐用法是直接运行 `sboxkit` 进入交互式终端**（TUI）——
所有功能都能在菜单里完成，无需记忆任何子命令。

以下子命令用于**脚本化 / 无人值守**场景（定时器、CI、远程批量运维），
效果与交互式菜单中的对应项一致。

## 子命令

| 命令 | 说明 |
|---|---|
| `sboxkit` | **交互式主菜单（推荐）**：运行时管理 / 配置变更 / 工具 / 暂停启动 / 语言 / 卸载。工具包含网络测试、只读故障诊断、日志、文件位置、信息、全部更新入口和使用说明 |
| `sboxkit init` | 初始化（首次部署）：单页表单收集下载代理、mixed 端口、TUN/局域网、直连 UID/进程和首个订阅，再按部署设置→订阅→服务→可选增强的独立事务执行 |
| `sboxkit modify` | 配置变更会话（对应主菜单「配置变更」）：订阅管理（添加订阅四种来源 + 本地文件覆盖）/ 部署设置 / 分流增强（esc 保存，^R 回退）。节点切换/内核更新/自更新/服务设置等即时生效操作仅在交互式主菜单「运行时管理」里，无对应子命令 |
| `sboxkit nettest` | 网络测试：流媒体 / 站点 / AI 延迟（TTFB）+ OpenAI / Claude 出口 IP 落地（主菜单「工具」聚合了这个 + 主要文件位置一览） |
| `sboxkit diagnose [--json]` | 只读展示 watchdog 当前状态和首次故障快照；不运行探针、不写配置、不修复或重启服务。无状态时也输出稳定的 unavailable 结构 |
| `sboxkit pause` | 暂停主服务及全部伴生单元（watchdog / 定时器一并停止，保持开机自启） |
| `sboxkit resume` | 启动主服务及全部伴生单元 |
| `sboxkit update` | 非交互全量更新：内核 + geo 数据强制更新 → 服务同步重启（每周定时器的执行目标） |
| `sboxkit assets-update` | 只更新当前状态目录中的 sing-box 内核与 geo 数据，绝不操作 systemd、不提权（旧名 `portable-update` 仍可用；用户模式下 `sboxkit update` 也会重启用户服务） |
| `sboxkit uninstall` | 勾选式卸载：服务 / 自愈 / 定时器 / 产物 / 全部状态 |
| `sboxkit version` | 显示版本 |

非 TTY 下（管道 / 重定向）交互提示自动回退为编号列表 + 文本输入，
子命令因此也可以在脚本里喂答案：

```bash
printf '3\ny\n' | sboxkit        # 例：进入主菜单第 3 项并确认
```

## 内部子命令

由 systemd 单元调用，一般无需手动执行：

| 命令 | 说明 |
|---|---|
| `sboxkit healthcheck` | `sing-box-watchdog.service` 与 NetworkManager dispatcher 的执行目标；分层分类故障，再按持久冷却/熔断策略决定是否重启 |
| `sboxkit bootstrap-dns` | 独立 relay 单元的执行目标；只监听 loopback，按有序 IP 字面量上游直连转发 DNS |

## 环境变量

| 变量 | 说明 |
|---|---|
| `SBOXKIT_HOME` | 覆盖数据目录（默认固定为 `/var/lib/sboxkit`，用户模式默认为包目录下 `.sboxkit/`） |
| `SBOXKIT_USER_MODE=1\|0` | 强制用户模式（systemd 用户单元）/ 系统模式；默认按可执行文件旁是否有 `sboxkit.user-mode` 标记判定。用户单元会写入 `=1` |
| `SBOXKIT_LANG=en\|zh` | 强制界面语言，优先级高于 `customize.json` 里保存的 `language` 字段（默认 `en`，部分终端无法正常显示中文） |
| `GITHUB_TOKEN` / `GH_TOKEN` | GitHub API Token（提升 release 查询限额） |
| `SBOXKIT_NO_PROXY=1` | 强制下载全部走直连，禁用代理兜底 |
| `NO_COLOR` | 关闭彩色输出与 TUI 盒子（自动回退纯文本） |

## 用户模式

`sboxkit-user_<version>_linux_<arch>.tar.gz` 是第二种主流用法，解压即用，包内结构：

```
sboxkit                 可执行文件
sboxkit.user-mode       用户模式标记（删除后回到系统模式）
core/sing-box(.version) 随包内核，初始化时离线接管
ruleset/*.srs           基础 geo 规则集种子
scripts/user-*.sh       更新 / 烟雾测试
licenses/               第三方许可
```

```bash
./sboxkit                  # 初始化 → systemctl --user 后台服务
./scripts/user-update.sh   # 更新内核与 geo，部署进运行时并重启用户服务
./scripts/user-test.sh     # 无 root 烟雾测试/配置校验
```

状态在 `.sboxkit/`，运行时在 `.sboxkit/runtime/`（0700），单元写入
`~/.config/systemd/user/`，watchdog 状态写入 `$XDG_RUNTIME_DIR/sboxkit`；首次注册
服务时尝试 `loginctl enable-linger`（失败只提示）；只有 linger 由 sboxkit 开启时才在状态
目录记下 `linger-enabled-by-sboxkit`，卸载「systemd 服务」时据此执行
`loginctl disable-linger`，用户原本开着的 linger 不动。

**sudo 只在 TUN 时出现**：运行时配置含 `tun` 入站（或 `routing_mark` /
`default_mark`）且运行时内核尚无能力时，sboxkit 经 sudo 执行
`setcap cap_net_admin,cap_net_bind_service,cap_net_raw+ep <runtime>/sing-box`。能力
保存在内核文件上，之后的重启、同步与 watchdog 重启不再提权；只有新内核进入运行时
（更新、重载服务、重新初始化且内核有变化）才会再请求一次：新内核先以 `sing-box.new`
暂存并授予能力，再原子替换。无人值守的更新（每周定时器、非终端且 sudo 需要密码）拿不到
授权时保留旧内核继续运行，规则集与配置照常更新，并在日志与下次打开菜单时提示执行
「重载服务」。需要 `setcap`（Debian/Ubuntu: `libcap2-bin`），解压目录不能在 nosuid
挂载上。防火墙与 NetworkManager 钩子在用户模式下不处理。需要在已登录的用户会话
（有 `XDG_RUNTIME_DIR`）中运行，用户实例不可达时各项服务操作会直接说明原因。

用户模式必须以安装包目录的属主运行：用 sudo / root 启动会被拒绝（TUN 需要的授权由
sboxkit 单独请求）。状态目录 `.sboxkit/` 为 0700（含订阅原文与节点凭据）。系统模式的
`sing-box.service` 正在运行时拒绝注册用户服务（两者争抢相同端口），仅安装未运行时提示。

sboxkit 自身更新位于「工具 → 更新」，当二进制所在目录可写时会直接使用原子
符号链接切换，不请求 sudo；系统包安装在 `/usr/bin` 时才回退到 sudo 接管。

## systemd 单元一览

由交互流程按需安装 / 卸载（用户模式下为 `~/.config/systemd/user/` 中的同名用户单元，
不含 NetworkManager 钩子）：

| 单元 | 作用 |
|---|---|
| `sing-box.service` | 主服务（`/var/lib/sboxkit-runtime` 自包含运行时）；Web UI 走其内置的 `:9090/ui/` 路径，不再有独立面板服务 |
| `sing-box-bootstrap-dns.service` | loopback bootstrap DNS relay；主服务通过 `Requires/After` 拉起，随主服务 stop/restart |
| `sing-box-watchdog.timer/.service` | 分层网络自愈探针；默认 10 分钟最多重启两次并指数退避 |
| `sing-box-update.timer/.service` | 每周自动更新（`sboxkit update`） |
