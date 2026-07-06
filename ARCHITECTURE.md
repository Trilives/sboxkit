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
| `/var/lib/sboxkit`（固定工作目录，`SBOXKIT_HOME` 可覆盖） | 状态数据：内核、内置面板资源、geo 规则集、`subscriptions/<name>/{meta.json,raw.*,config.json}`、`active` 指针、`customize.json`。首次使用经 sudo 创建并交回调用者属主，root 定时器与用户会话共享同一份数据 |
| `/var/lib/sboxkit-runtime` | root 态自包含运行时：内核 + 配置 + geo 规则集 + 面板资源的暂存副本（服务与用户目录解耦） |
| `/etc/systemd/system` | `sing-box` / `sing-box-watchdog` / `sing-box-update` 三组单元 |

**Web 面板**：sing-box 没有官方面板可下载（不像 mihomo 有 metacubexd），因此
sboxkit 自己写了一个极简面板（`internal/uiassets`，go:embed 打进二进制，无需
联网下载）。面板走 sing-box 内置的 `experimental.clash_api.external_ui` 机制，
与 mihomo 版一致：`http://host:9090/ui/`，不额外占用端口、不需要独立的反向代理
进程——面板页面里的 JS 直接调用同源的 sing-box Clash API（`/proxies`、
`/proxies/{group}`、`/proxies/{name}/delay`）。`lan_panel` 定制字段只决定
`external_controller` 绑定 `127.0.0.1` 还是 `0.0.0.0`，不影响面板是否可用。

普通用户运行，特权操作全部经 `sudo` 子进程（`internal/execx`），凭证会话内缓存。

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
标注「※即时」，不参与回退。

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

## 6. 离线种子（.deb）

.deb 内置 sing-box 内核与基础 geo 规则集（`geosite-cn.srs` + `geoip-cn.srs`），
装包即可离线初始化：`kernel.SeedFromSystem` 在 state 缺失对应文件时从
`/usr/libexec/sboxkit` / `/usr/share/sboxkit/ruleset` 复制接管。与 mihomo 版
的三件套（geoip.metadb/geosite.dat/country.mmdb）不同，sing-box 用 sing-geosite
/ sing-geoip 项目发布的 `.srs` 规则集，一国家一份文件，无 MaxMind EULA 之类的
新鲜度义务，可以放心冻结进安装包；仍支持在线『更新 geo 数据』获取最新版本。

## 7. 自愈与守护

- NetworkManager dispatcher 钩子：真实网卡 up / 连通性变化时防抖重启（忽略 tun 自身）
- watchdog 定时器 + `sboxkit healthcheck`：仅当「有上行但代理探测不通」且服务
  已运行足够久才重启，避免重启风暴
- 暂停 / 启动把伴生单元一并带上（否则 watchdog 会把刚停的主服务拉起来）

## 8. sboxkit 自更新

`internal/selfupdate`：版本化目录 `<state>/sboxkit-versions/<version>/sboxkit` +
`current` 符号链接方案，与 `internal/kernel` 更新 sing-box 内核/geo 数据完全独立
（更新的是 sboxkit 自身这个程序）。流程：查询最新 GitHub release → 下载对应架构的
`sboxkit_<version>_linux_<arch>.tar.gz` → 按 `checksums.txt` 校验 SHA-256 → 解压到
独立版本目录 → 试跑新二进制（`sboxkit version`）确认能执行 → 原子重写 `current`
符号链接 → 再次试跑确认成功；启动校验失败则回退 `current` 指向。首次自更新时若正在
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
回退的用户一键切回（`RollbackToStable`）。

## 9. 包结构

```
cmd/sboxkit           入口：子命令 + TUI 主菜单
internal/errs         ErrSaveExit / ErrCancelled 导航哨兵
internal/execx        日志、子进程、sudo
internal/paths        状态目录解析（SBOXKIT_HOME > XDG）
internal/config       customize.json：类型化 Config struct，默认值 / 读写 / 字段元数据 / 脱敏
internal/converter    Clash / sing-box 原生 / base64 → sing-box 配置的核心转换器
internal/txn          事务：BackupFile / Snapshot / TrackPath / AddUndo，LIFO 回滚
internal/tui          Bubble Tea 四类提示 + 非 TTY 回退
internal/fetchx       HTTP 下载通道
internal/kernel       sing-box 内核 / geo 规则集下载部署 + deb 种子接管
internal/uiassets     内置 Web 面板静态资源（go:embed，无需联网下载）
internal/selfupdate   sboxkit 自更新：版本化目录 + 原子符号链接切换
internal/subscription 订阅域：fetch / detect（三态）/ b64（分享链接解析）/ manager
internal/sysd         systemd 三组单元（模板 go:embed）
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
