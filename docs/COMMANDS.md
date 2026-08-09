# 命令参考

sboxkit 的**推荐用法是直接运行 `sboxkit` 进入交互式终端**（TUI）——
所有功能都能在菜单里完成，无需记忆任何子命令。

以下子命令用于**脚本化 / 无人值守**场景（定时器、CI、远程批量运维），
效果与交互式菜单中的对应项一致。

## 子命令

| 命令 | 说明 |
|---|---|
| `sboxkit` | **交互式主菜单（推荐）**：运行时管理 / 配置变更 / 工具 / 暂停启动 / 语言 / 卸载。工具包含网络测试、日志、文件位置、信息、全部更新入口和使用说明 |
| `sboxkit init` | 初始化（首次部署）：单页表单收集下载代理、mixed 端口、TUN/局域网、直连 UID/进程和首个订阅，再按部署设置→订阅→服务→可选增强的独立事务执行 |
| `sboxkit modify` | 配置变更会话（对应主菜单「配置变更」）：订阅管理（添加订阅四种来源 + 本地文件覆盖）/ 部署设置 / 分流增强（esc 保存，^R 回退）。节点切换/内核更新/自更新/服务设置等即时生效操作仅在交互式主菜单「运行时管理」里，无对应子命令 |
| `sboxkit nettest` | 网络测试：流媒体 / 站点 / AI 延迟（TTFB）+ OpenAI / Claude 出口 IP 落地（主菜单「工具」聚合了这个 + 主要文件位置一览） |
| `sboxkit pause` | 暂停主服务及全部伴生单元（watchdog / 定时器一并停止，保持开机自启） |
| `sboxkit resume` | 启动主服务及全部伴生单元 |
| `sboxkit update` | 非交互全量更新：内核 + geo 数据强制更新 → 服务同步重启（每周定时器的执行目标） |
| `sboxkit portable-update` | 便携包无 root 更新：只更新用户状态目录中的 sing-box 内核与 geo 数据，绝不操作 systemd |
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
| `sboxkit healthcheck` | `sing-box-watchdog.service` 的执行目标；有上行但本地代理探测失败时重启主服务 |

## 环境变量

| 变量 | 说明 |
|---|---|
| `SBOXKIT_HOME` | 覆盖数据目录（默认固定为 `/var/lib/sboxkit`；主要用于测试） |
| `SBOXKIT_LANG=en\|zh` | 强制界面语言，优先级高于 `customize.json` 里保存的 `language` 字段（默认 `en`，部分终端无法正常显示中文） |
| `GITHUB_TOKEN` / `GH_TOKEN` | GitHub API Token（提升 release 查询限额） |
| `SBOXKIT_NO_PROXY=1` | 强制下载全部走直连，禁用代理兜底 |
| `NO_COLOR` | 关闭彩色输出与 TUI 盒子（自动回退纯文本） |

## 便携包

解压 tar.gz 后可把状态完全放在解压目录中：

```bash
export SBOXKIT_HOME="$PWD/.sboxkit"
./sboxkit
./scripts/portable-update.sh   # 更新内核与 geo
./scripts/portable-test.sh     # 无 root 烟雾测试/配置校验
```

sboxkit 自身更新位于「工具 → 更新」，当二进制所在目录可写时会直接使用原子
符号链接切换，不请求 sudo；系统包安装在 `/usr/bin` 时才回退到 sudo 接管。

## systemd 单元一览

由交互流程按需安装 / 卸载：

| 单元 | 作用 |
|---|---|
| `sing-box.service` | 主服务（`/var/lib/sboxkit-runtime` 自包含运行时）；Web UI 走其内置的 `:9090/ui/` 路径，不再有独立面板服务 |
| `sing-box-watchdog.timer/.service` | 网络自愈探针（有上行但代理不通才重启） |
| `sing-box-update.timer/.service` | 每周自动更新（`sboxkit update`） |
