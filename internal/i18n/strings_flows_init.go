package i18n

func init() {
	register(map[string]string{
		"初始化":       "Initialize",
		"初始化（首次部署）": "Initialize (first-time deployment)",
		"种子接管失败（不影响后续下载）：": "Seed takeover failed (does not affect later downloads): ",

		"部署设置": "Deployment settings",
		"添加订阅": "Add subscription",
		"注册服务": "Register service",
		"网络自愈": "Network self-healing",

		"启用 TUN 模式？（整机流量自动走代理；否=纯代理，需各 App 手动设代理）":     "Enable TUN mode? (all system traffic auto-routes through the proxy; no = pure proxy mode, each app must set its own proxy)",
		"开启局域网代理？（让局域网其他主机可用本机作为代理，监听 0.0.0.0:7890）":   "Enable LAN proxy? (lets other hosts on the LAN use this machine as a proxy, listening on 0.0.0.0:7890)",
		"把代理环境变量写入 ~/.bashrc？（新开终端自动走 127.0.0.1:7890）": "Write proxy environment variables to ~/.bashrc? (new terminals will automatically use 127.0.0.1:7890)",
		"更新防火墙放行 7890 端口？":                             "Update firewall to allow port 7890?",
		"撤销防火墙放行 7890":                                 "Revoke firewall rule for port 7890",

		"已跳过订阅与服务注册，结束初始化。设置已保存，":    "Skipped subscription and service registration; initialization ended. Settings have been saved; ",
		"稍后可在主菜单「订阅 → 添加订阅」补配并启动服务。": "you can later finish setup via the main menu 'Subscriptions → Add subscription' and start the service.",

		"卸载服务 sing-box": "Uninstall sing-box service",

		"配置已就绪，是否现在切换节点？": "Config is ready. Switch a node now?",
		"初始化完成。":          "Initialization complete.",

		"删除订阅 ": "Delete subscription ",

		"检测到本地已有 %d 个订阅记录，是否直接使用现有订阅？": "Detected %d existing subscription(s) locally. Use the existing subscription directly?",
		"已使用现有订阅：%s": "Using existing subscription: %s",

		"使用本地内核与基础规则启动服务（系统包种子或既有资源）。":             "Starting the service with the local core and basic rules (system package seed or existing resources).",
		"未找到本地内核或基础规则；非 .deb 安装/种子缺失时需要先下载才能启动服务。": "Local core or basic rules not found; for non-.deb installs or missing seeds, you need to download them first to start the service.",
		"现在下载内核和基础规则以便启动服务？":                       "Download the core and basic rules now to start the service?",
		"缺少 sing-box 内核或基础规则，无法注册并启动服务":            "Missing sing-box core or basic rules; cannot register and start the service",

		"服务已启动。现在下载/更新内核和 geo 数据？（内置 Web 面板已随服务部署，浏览器访问 http://host:9090/ui/ 即可查看/切换节点）": "Service started. Download/update the core and geo data now? (The built-in web panel is already deployed with the service; browse http://host:9090/ui/ to view/switch nodes.)",
		"下载/更新内核 / geo 数据…":             "Downloading/updating core / geo data…",
		"已更新资源，重新部署运行时并重启服务…":           "Resources updated; redeploying the runtime and restarting the service…",
		"更新内核/geo 数据失败（服务仍按原资源正常运行）：%v": "Failed to update core/geo data (the service keeps running fine on its existing resources): %v",

		"安装网络切换自愈？":      "Install network self-healing?",
		"卸载网络自愈":         "Uninstall network self-healing",
		"安装网络自愈失败：%v":    "Failed to install network self-healing: %v",
		"安装每周自动更新定时器？":   "Install weekly auto-update timer?",
		"卸载每周更新":         "Uninstall weekly update",
		"安装每周更新定时器失败：%v": "Failed to install the weekly update timer: %v",
	})
}
