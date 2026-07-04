package i18n

func init() {
	register(map[string]string{
		"未安装": "Not installed",
		"已安装": "Installed",

		"安装网络自愈":             "Install network self-healing",
		"调整探测间隔":             "Adjust probe interval",
		"卸载网络自愈":             "Uninstall network self-healing",
		"网络自愈设置（当前：%s）":      "Network Self-Healing Settings (current: %s)",
		"探测间隔（如 2min / 90s）": "Probe interval (e.g. 2min / 90s)",

		"安装每周更新定时器":      "Install weekly update timer",
		"改时间表":           "Change schedule",
		"卸载定时器":          "Uninstall timer",
		"每周更新定时器（当前：%s）": "Weekly Update Timer (current: %s)",
		"OnCalendar 表达式": "OnCalendar expression",

		"服务尚未安装，请先执行『初始化（首次部署）』。": "The service is not installed yet; please run 'Initialize (first-time deployment)' first.",
		"暂停 / 启动服务":             "Pause / Start Service",
		"已停止":                   "Stopped",
		"运行中":                   "Running",
		"  主服务 %s.service：%s\n": "  Main service %s.service: %s\n",
		"  伴生单元 %s：状态见 systemctl\n": "  Companion unit %s: see systemctl for status\n",
		"启动":        "start",
		"暂停":        "pause",
		"确认%s全部服务？": "Confirm %s all services?",

		"服务设置":      "Service Settings",
		"查看状态":      "View status",
		"重启服务":      "Restart service",
		"同步当前配置并重启": "Sync current config and restart",

		"Web UI（sing-box 内置路径）: http://%s:9090/ui/":                   "Web UI (sing-box built-in path): http://%s:9090/ui/",
		"远程查看建议用 SSH 端口转发： ssh -N -L 9090:127.0.0.1:9090 user@server": "For remote viewing, use SSH port forwarding: ssh -N -L 9090:127.0.0.1:9090 user@server",
		"局域网代理已开启：其他主机可设置 http/socks 代理为 本机IP:7890":                   "LAN proxy is enabled: other hosts can set their http/socks proxy to this machine's IP:7890",
	})
}
