# sboxkit

`sboxkit` 是面向 Linux 的 sing-box 终端部署工具。推荐入口是交互式 TUI：

```bash
sboxkit
```

它用于导入订阅或本地配置、生成 sing-box 配置、安装 systemd 服务、切换节点、更新规则资产、诊断网络状态，并维护固定的用户状态目录。

## 发布形式

0.1.3 发布两类 Linux 包：

- `.deb`：推荐安装方式，一个包内包含 `sboxkit` TUI/CLI 和独立的上游 `sing-box` 内核二进制。
- `.tar.gz`：便携包，包含 `sboxkit`、`sing-box`、最小基础规则和 `install.sh`。

两个二进制文件保持分离，`sboxkit` 不链接 sing-box 源码。仓库源码不会提交 sing-box 二进制，发布包会记录上游来源和版本。

`.deb` 主要内容：

```text
/usr/bin/sboxkit
/usr/lib/sboxkit/sing-box
/usr/share/sboxkit/base-rules/minimal.json
/lib/systemd/system/sboxkit.service
/usr/share/doc/sboxkit/README.md
/usr/share/doc/sboxkit/docs/COMMANDS.md
/usr/share/doc/sboxkit/SING_BOX_SOURCE.txt
```

## 安装

从 GitHub Releases 下载对应架构的安装包：

```bash
sudo apt install ./sboxkit_0.1.3_amd64.deb
```

也可以安装 arm64 包：

```bash
sudo apt install ./sboxkit_0.1.3_arm64.deb
```

便携包：

```bash
tar -xzf sboxkit_0.1.3_amd64_portable.tar.gz
cd sboxkit_0.1.3_amd64
sudo ./install.sh
```

## 推荐运行方式

直接运行：

```bash
sboxkit
```

TUI 主界面聚合为六类：

1. 无需重启配置：节点切换、Shell 代理环境、查看配置。
2. 需重启配置：首次初始化、订阅、本地配置、规则资产、定时任务和恢复。
3. 诊断工具：网络测试和主要文件位置。
4. 服务控制：启动、停止、同步、安装、移除 systemd 服务。
5. 语言：默认英文，可切换中文，偏好会持久化。
6. 卸载：移除系统集成，可选清理用户状态。

如果启动时检测不到 `sboxkit.service` 集成，包括停止状态的服务也会识别，程序会先进入首次设置流程。首次设置会先询问界面语言，也允许跳过初始化。

## 首次运行逻辑

首次初始化会尽量保证先跑起来，再下载大型资产：

1. 选择语言。
2. 选择是否初始化。
3. 选择订阅来源，例如 Clash、sing-box 或本地配置文件。
4. 输入配置名称、订阅 URL 或本地 `config.yaml`/`config.json` 路径。
5. 选择是否启用 TUN；如果不启用，会询问是否写入 Shell 全局代理变量。
6. 使用内置最小基础规则生成可运行配置。
7. 可选安装并启动服务。启动前会提示 TUN 或路由变更可能截断当前 SSH 连接。
8. 服务运行后，再可选通过代理下载大型规则资产并同步重启。

如果规则资产下载失败，不会影响已经初始化并启动的服务。

## 固定工作目录

用户状态固定存放，不受当前 shell 所在目录影响：

```text
$XDG_STATE_HOME/sboxkit/state/
~/.local/state/sboxkit/state/   # 未设置 XDG_STATE_HOME 时
```

可通过 `SBOXKIT_ROOT=/path/to/root` 或命令参数 `--root DIR` 覆盖。

常见用户状态文件：

```text
state/customize.json
state/config.json
state/active.json
state/subscriptions/
state/downloads/
state/logs/
state/ruleset/
state/ui/
```

systemd 服务运行目录：

```text
/etc/sboxkit/
├── sing-box
├── sboxkit.json
├── sboxkit.cache.db
├── ruleset/
├── ui/
└── healthcheck.sh
```

## 配置来源

支持远程订阅和本地配置：

- `clash`：本地解析 Clash YAML，并转换为 sing-box 配置。
- `sing-box`：直接使用 sing-box JSON；关闭 passthrough 时会重建本地策略组。
- `base64`：优先使用 subconverter 后端，必要时可启用本地 Shadowsocks 应急解析。
- 本地文件：指定 `config.yaml` 或 `config.json` 路径后，程序会复制到固定状态目录再使用。

切换订阅会重建当前配置。节点切换默认只通过运行中的 Clash API 生效，不重启服务；只有选择调整节点顺序并同步服务时才会重启。

## 日志

文件日志默认关闭。开启后，`sboxkit` 会把 stderr 记录到固定状态目录：

```text
~/.local/state/sboxkit/state/logs/
```

日志会按大小上限自动删除旧文件，默认 10 MB，硬上限 100 MB。

## 可选 WebUI

WebUI 默认关闭。开启后，sing-box 会从本项目内置的本地资源提供轻量 WebUI，用于查看出站组和切换选择器节点。它不下载外部大型 UI。

WebUI 是辅助功能，终端 TUI 仍然是推荐操作方式。

## 卸载

卸载系统集成：

```bash
sudo sboxkit uninstall
```

同时清理用户状态：

```bash
sudo sboxkit uninstall --purge-state
```

如果是 `.deb` 安装，移除包本身：

```bash
sudo apt remove sboxkit
sudo apt purge sboxkit
```

## 法律与第三方资产

- `sboxkit` 由本仓库构建。
- `sing-box` 作为独立上游二进制随发布包分发。
- 发布包会写入 sing-box 上游源码与版本来源。
- 大型第三方规则集只在用户明确请求时下载。
- WebUI 使用本仓库内置资源，不下载第三方仪表盘。

更多说明见 [第三方资产](docs/THIRD_PARTY_ASSETS.md)。

## 从源码构建

```bash
go test ./...
go vet ./...
go build -o sboxkit ./cmd/sboxkit
```

使用仓库内的隔离 Go 工具链：

```bash
make test GO=.tools/go/bin/go
make vet GO=.tools/go/bin/go
make build GO=.tools/go/bin/go
```

## 文档

- [命令参考](docs/COMMANDS.md)
- [架构说明](ARCHITECTURE.md)
- [模块化约束](docs/MODULARITY.md)
- [第三方资产](docs/THIRD_PARTY_ASSETS.md)
