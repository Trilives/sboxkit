# 命令参考

推荐使用交互式终端界面：

```bash
sboxkit
```

下面的命令主要用于脚本、远程维护和故障恢复。

## 帮助与版本

```bash
sboxkit help
sboxkit version
```

## 初始化

推荐交互式初始化：

```bash
sboxkit
```

非交互式初始化：

```bash
sboxkit init
sboxkit init --download --proxy http://127.0.0.1:7890
sboxkit init --no-tun
sboxkit init --no-tun --write-proxy-env
sboxkit init --no-tun --no-write-proxy-env
```

TUI 中关闭 TUN 时，会询问是否写入 Shell 全局代理变量。所有 y/n 选择都可以取消返回。

## 订阅和本地配置

添加 Clash 订阅：

```bash
sboxkit sub add --name main --source clash --url "https://example.com/sub.yaml"
```

添加 sing-box 订阅或本地配置文件：

```bash
sboxkit sub add --name remote --source sing-box --url "https://example.com/config.json"
sboxkit sub add --name local --file ~/config.yaml
sboxkit sub add --name direct --file ~/config.json --source sing-box
sboxkit sub add --name direct --file ~/config.json --source sing-box --passthrough
```

本地文件会先复制进固定状态目录，再参与转换和重建。

管理订阅：

```bash
sboxkit sub list
sboxkit sub switch --name NAME
sboxkit sub refresh --name NAME
sboxkit sub refresh --name NAME --proxy http://127.0.0.1:7890
sboxkit sub rebuild --name NAME
sboxkit sub remove --name NAME
```

切换订阅会重建活动配置。若要应用到 systemd 服务，需要再执行 `sudo sboxkit service sync`。

## 服务管理

安装并启动 systemd 服务：

```bash
sudo sboxkit service install
```

服务命令：

```bash
sudo sboxkit service install --no-start
sudo sboxkit service sync
sudo sboxkit service start
sudo sboxkit service stop
sudo sboxkit service status
sudo sboxkit service remove
```

启动或重启服务前，程序会提示 TUN 或路由变更可能截断当前 SSH 连接。

## 运行时资产

服务先运行起来后，建议通过已运行代理下载大型可选规则资产：

```bash
sboxkit update --proxy http://127.0.0.1:7890 --sync-service
```

其他形式：

```bash
sboxkit update
sboxkit update --force
sboxkit update --core
```

`update` 默认下载运行时规则资产；`--core` 会同时更新用户状态目录里的 sing-box 内核缓存。`.deb` 安装包中的 `/usr/lib/sboxkit/sing-box` 不会被覆盖。

## 配置

查看配置：

```bash
sboxkit config show
```

常用设置：

```bash
sboxkit config set --key enable_tun --value false
sboxkit config set --key lan_proxy --value true
sboxkit config set --key lan_panel --value true
sboxkit config set --key enable_file_log --value true
sboxkit config set --key log_max_mb --value 20
sboxkit config set --key direct_domain_suffixes --value example.com,example.org
```

TUI 中配置分组包括：

- 常用部署：TUN、局域网代理、WebUI、下载代理、GitHub 加速和 Token。
- 订阅与后端：subconverter 后端和 base64 本地回退。
- 日志与诊断：文件日志开关和大小上限。
- DNS 与出站：引导 DNS 和默认出站。
- TUN 与绕过：路由排除、UID 排除、进程绕过和本地域名绕过。
- 地区与分流：地区聚合组和域名后缀规则。

修改后如果需要生效到服务：

```bash
sboxkit sub rebuild --name main
sudo sboxkit service sync
```

## 日志

开启文件日志：

```bash
sboxkit config set --key enable_file_log --value true
sboxkit config set --key log_max_mb --value 20
```

日志位置：

```text
~/.local/state/sboxkit/state/logs/
```

日志默认关闭。开启后会自动删除旧日志，保持总大小不超过 `log_max_mb`，硬上限为 100 MB。

## Shell 代理环境

```bash
sboxkit proxy-env write
sboxkit proxy-env remove
```

写入的托管块会导出：

```text
http_proxy=http://127.0.0.1:7890
https_proxy=http://127.0.0.1:7890
all_proxy=socks5://127.0.0.1:7890
```

## 节点操作

```bash
sboxkit node list
sboxkit node list --api http://127.0.0.1:9090
sboxkit node list --api http://127.0.0.1:9090 --secret TOKEN
sboxkit node switch --group Proxy --name NODE
sboxkit node switch --group Proxy --name NODE --reorder
sudo sboxkit node switch --group Proxy --name NODE --reorder --sync-service
```

普通节点切换只调用运行中的 Clash API，不重启服务。`--reorder` 会把该节点移动到生成配置的选择器前列；只有再加 `--sync-service` 才会同步并重启 systemd 服务。

## WebUI

启用内置 WebUI：

```bash
sboxkit config set --key lan_panel --value true
sboxkit sub rebuild --name main
sudo sboxkit service sync
```

访问：

```text
http://127.0.0.1:9090/ui/
```

本地预览内置 UI 资源：

```bash
sboxkit ui serve --addr 127.0.0.1:8790
```

`ui serve` 只是预览工具，正常 WebUI 由运行中的 sing-box 提供。

## 定时任务和恢复

```bash
sudo sboxkit timer install --binary /usr/bin/sboxkit
sudo sboxkit timer remove

sudo sboxkit resilience install
sudo sboxkit resilience remove
```

## 诊断

网络测试：

```bash
sboxkit nettest
```

主要文件位置可在 TUI 的 `Diagnostics / 诊断工具` 中查看。

常见位置：

```text
~/.local/state/sboxkit/state/customize.json
~/.local/state/sboxkit/state/config.json
~/.local/state/sboxkit/state/subscriptions/
~/.local/state/sboxkit/state/downloads/
~/.local/state/sboxkit/state/logs/
/etc/sboxkit/sboxkit.json
/etc/sboxkit/sing-box
/usr/lib/sboxkit/sing-box
/lib/systemd/system/sboxkit.service
```

## 卸载

移除 sboxkit 管理的服务、定时器、恢复守护和 `/etc/sboxkit`：

```bash
sudo sboxkit uninstall
```

同时清理用户状态：

```bash
sudo sboxkit uninstall --purge-state
```

`sboxkit uninstall` 不会删除 `.deb` 安装的 `/usr/bin/sboxkit`。移除包本身：

```bash
sudo apt remove sboxkit
```

连同包配置文件一起移除：

```bash
sudo apt purge sboxkit
```
