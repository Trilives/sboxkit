# sboxkit

面向 Linux 的 sing-box 终端优先部署工具。

`sboxkit` 以 TUI 优先的方式安装和管理 sing-box 本地代理服务。Debian
安装包会在一个安装器里带上两个独立二进制文件：`sboxkit` 控制器和上游
`sing-box` 内核。内核不会链接进本项目，仓库本身也不会提交上游 sing-box
二进制。

## 打包内容

`.deb` 包内包含：

```text
/usr/bin/sboxkit
/usr/lib/sboxkit/sing-box
/usr/share/sboxkit/base-rules/minimal.json
/lib/systemd/system/sboxkit.service
/usr/share/doc/sboxkit/copyright
/usr/share/doc/sboxkit/SING_BOX_SOURCE.txt
```

便携式 `.tar.gz` 包包含同样的两个二进制文件，外加安装脚本。

## 安装

从 GitHub Releases 下载对应架构的安装包：

```bash
sudo apt install ./sboxkit_0.1.1_amd64.deb
```

便携包安装：

```bash
tar -xzf sboxkit_0.1.1_amd64_portable.tar.gz
cd sboxkit_0.1.1_amd64
sudo ./install.sh
```

## 推荐用法

启动交互式终端界面：

```bash
sboxkit
```

TUI 是主要入口。它会引导初次初始化、订阅导入、TUN 模式选择、可选的
shell 代理环境写入、服务安装、更新、节点切换和诊断。

脚本化用法见 [命令参考](docs/COMMANDS.md)。

## 首次运行模型

首次初始化刻意做得很轻：

1. 配置本地偏好，并导入一个订阅或本地 sing-box 配置。
2. 使用内置的最小规则文件生成可工作的 sing-box 配置。
3. 启动服务。
4. 在需要时，通过已运行的代理下载更大的可选规则资产。

这样可以让全新安装在代理可用前就启动起来，不需要先下载外部大型规则集。

## 运行目录

用户数据存放在固定状态目录中，不受当前工作目录影响：

```text
$XDG_STATE_HOME/sboxkit/state/
~/.local/state/sboxkit/state/   # when XDG_STATE_HOME is not set
```

可以通过 `SBOXKIT_ROOT=/path/to/root` 覆盖，也可以对单个命令传入 `--root DIR`。

系统服务运行时文件会放到：

```text
/etc/sboxkit/
├── sing-box
├── sboxkit.json
├── sboxkit.cache.db
├── ruleset/
├── ui/
└── healthcheck.sh
```

## 订阅来源

`sboxkit` 可以从远程 URL 或本地配置文件导入订阅：

- `clash`：本地解析 Clash YAML，并转换为 sing-box 配置。
- `sing-box`：直接使用 sing-box JSON；如果关闭 passthrough 模式，则会重建本地策略组。
- `base64`：优先使用配置的 subconverter 后端，必要时可回退到本地 Shadowsocks 解析。

本地文件在转换前会先复制到固定状态目录。

## 可选 WebUI

WebUI 默认关闭。开启后，sing-box 会从本仓库内置的本地资源目录
`/etc/sboxkit/ui` 提供一个轻量 WebUI。它通过 Clash API 查看出站组并切换
选择器节点，且不需要重启服务。

WebUI 只是辅助界面，终端 UI 仍然是推荐的操作方式。

## 法律与第三方资产

- `sboxkit` 由本仓库构建。
- `sing-box` 作为独立的上游二进制文件随发布包分发。
- 上游 sing-box 的源码引用会写入包文档。
- 大型第三方规则集只会在用户明确请求时下载。
- 不下载第三方 WebUI 仪表盘。

打包说明见 [Third-Party Assets](docs/THIRD_PARTY_ASSETS.md)。

## 从源码构建

```bash
go test ./...
go vet ./...
go build -o sboxkit ./cmd/sboxkit
```

仓库也支持使用 `.tools/go` 下的隔离本地 Go 工具链：

```bash
make test GO=.tools/go/bin/go
make vet GO=.tools/go/bin/go
make build GO=.tools/go/bin/go
```

## 更多文档

- [命令参考](docs/COMMANDS.md)
- [架构说明](ARCHITECTURE.md)
- [第三方资产](docs/THIRD_PARTY_ASSETS.md)
