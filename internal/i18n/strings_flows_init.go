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

		"使用本地内核与基础规则启动服务（系统包种子或既有资源）。":            "Starting the service with the local core and basic rules (system package seed or existing resources).",
		"未找到本地内核或基础规则，正在自动下载…":                    "Local core or basic rules not found; downloading automatically…",
		"下载内核/基础规则失败（可稍后在主菜单『运行时管理 → 内核更新』重试）：%w": "Failed to download core/basic rules (you can retry later via the main menu 'Runtime management → Update core'): %w",

		"正在自动下载/更新内核与 geo 数据…": "Automatically downloading/updating the core and geo data…",
		"已更新资源，重新部署运行时并重启服务…":  "Resources updated; redeploying the runtime and restarting the service…",
		"内核/geo 数据下载失败（服务仍按现有资源正常运行），可稍后在主菜单『运行时管理 → 内核更新』里重试：%v": "Core/geo data download failed (the service keeps running fine on its existing resources); you can retry later via the main menu 'Runtime management → Update core': %v",
		"重新部署运行时失败（可稍后在设置里重试）：%v":                                 "Failed to redeploy the runtime (you can retry later in settings): %v",

		"安装网络切换自愈？":      "Install network self-healing?",
		"卸载网络自愈":         "Uninstall network self-healing",
		"安装网络自愈失败：%v":    "Failed to install network self-healing: %v",
		"安装每周自动更新定时器？":   "Install weekly auto-update timer?",
		"卸载每周更新":         "Uninstall weekly update",
		"安装每周更新定时器失败：%v": "Failed to install the weekly update timer: %v",
	})
}
