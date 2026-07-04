package i18n

func init() {
	register(map[string]string{
		"%s（%d 项）…": "%s (%d items)…",

		"超时": "Timeout",

		"工具":   "Tools",
		"网络测试": "Network Test",
		"经本地代理 %s 测试（走 sing-box 出口）。":    "Testing via local proxy %s (through sing-box egress).",
		"本地代理 %s 未监听，改用直连测试（结果不代表代理体验）。": "Local proxy %s is not listening; falling back to direct connection (results don't reflect the proxied experience).",

		"主要文件位置":      "Key File Locations",
		"生效配置":        "Active config",
		"定制层":         "Customize layer",
		"生效订阅名":       "Active subscription pointer",
		"订阅目录":        "Subscriptions directory",
		"sing-box 内核": "sing-box core binary",
		"基础规则目录":      "Base ruleset directory",
		"Web UI 目录":   "Web UI directory",
		"下载缓存目录":      "Download cache directory",
		"systemd 单元":  "systemd unit",

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
