# 已完成

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
