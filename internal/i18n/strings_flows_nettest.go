package i18n

func init() {
	register(map[string]string{
		"%s（%d 项）…": "%s (%d items)…",

		"超时": "Timeout",

		"工具":   "Tools",
		"网络测试": "Network Test",
		"经本地代理 %s 测试（走 sing-box 出口）。":    "Testing via local proxy %s (through sing-box egress).",
		"本地代理 %s 未监听，改用直连测试（结果不代表代理体验）。": "Local proxy %s is not listening; falling back to direct connection (results don't reflect the proxied experience).",

		"主要文件位置":         "Key File Locations",
		"生效配置":           "Active config",
		"定制层":            "Customize layer",
		"生效订阅名":          "Active subscription pointer",
		"订阅目录":           "Subscriptions directory",
		"sing-box 内核":    "sing-box core binary",
		"基础规则目录":         "Base ruleset directory",
		"Web UI 目录":      "Web UI directory",
		"下载缓存目录":         "Download cache directory",
		"sboxkit TUI 日志": "sboxkit TUI log",
		"内核日志命令":         "Core log command",
		"查看日志":           "View logs",
		"sing-box 内核日志":  "sing-box core log",
		"使用说明":           "Usage guide",
		"日志文件尚不存在：%s":    "Log file does not exist yet: %s",
		`sboxkit 使用说明

• 运行时管理：切换或固定节点、管理服务、自愈与定时器。
• 配置变更：管理订阅，修改 TUN、端口、面板、下载与分流设置。
• 工具：网络测试、日志、文件位置、连接信息和全部更新入口。
• 暂停 / 启动：主服务及 watchdog、更新定时器会一起切换。

键位：↑/↓ 移动，Space 勾选，Enter 确认，Esc 保存返回，Ctrl-R 回退。
详细命令参考：docs/COMMANDS.md 或 sboxkit --help`: `sboxkit Usage Guide

• Runtime: switch or pin nodes; manage the service, recovery, and timers.
• Configuration: manage subscriptions and edit TUN, ports, panel, downloads, and routing.
• Tools: network tests, logs, file locations, connection info, and every update channel.
• Pause / Start: the main service, watchdog, and update timer switch together.

Keys: ↑/↓ move, Space toggle, Enter confirm, Esc save/back, Ctrl-R roll back.
Command reference: docs/COMMANDS.md or sboxkit --help`,
		"systemd 单元": "systemd unit",

		"延迟测试": "Latency test",
		"出口探测": "Egress probe",

		"流媒体": "Streaming",
		"站点":  "Sites",
		"AI":  "AI",

		"【%s】":       "[%s]",
		"出口 IP / 落地": "Egress IP / location",

		"  ✗ %-12s 探测失败\n":          "  ✗ %-12s probe failed\n",
		"  ✓ %-12s %-22s 落地 %s%s\n": "  ✓ %-12s %-22s location %s%s\n",

		"网络测试完成。":   "Network test complete.",
		"回车返回主菜单… ": "Press Enter to return to the main menu… ",
	})
}
