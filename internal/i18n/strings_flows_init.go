package i18n

func init() {
	register(map[string]string{
		"初始化":         "Initialize",
		"初始化（首次部署）":   "Initialize (first-time deployment)",
		"sboxkit 初始化": "sboxkit Initialization",
		"开始初始化":       "Start initialization",
		"详细配置请在初始化完成后通过“配置变更”修改": "Detailed settings can be changed under Configuration after initialization",
		"种子接管失败（不影响后续下载）：":       "Seed takeover failed (does not affect later downloads): ",

		"部署设置":           "Deployment settings",
		"添加订阅":           "Add subscription",
		"注册服务":           "Register service",
		"网络自愈":           "Network self-healing",
		"启用 TUN":         "Enable TUN",
		"放行代理端口":         "Allow proxy port",
		"写入 shell 代理变量":  "Write shell proxy variables",
		"TUN 直连 UID":     "TUN direct UIDs",
		"TUN 直连进程名":      "TUN direct process names",
		"订阅名称":           "Subscription name",
		"订阅类型":           "Subscription type",
		"订阅地址 / 本地路径":    "Subscription URL / local path",
		"使用代理拉取":         "Fetch through proxy",
		"使用下载代理":         "Use download proxy",
		"下载代理地址":         "Download proxy URL",
		"按 sboxkit 规则重建": "Rebuild using sboxkit rules",
		"订阅名称不能为空":       "Subscription name cannot be empty",
		"UID 必须是非负整数：%s": "UID must be a non-negative integer: %s",

		"订阅地址须以 http:// 或 https:// 开头，本地文件请直接填写路径": "Subscription URLs must start with http:// or https://; for a local file, enter its path",
		"%w（远程订阅地址须以 http:// 或 https:// 开头）":       "%w (remote subscription URLs must start with http:// or https://)",

		"撤销防火墙放行 %d": "Revoke firewall rule for port %d",

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

		"安装网络切换自愈？":      "Install network self-healing?",
		"卸载网络自愈":         "Uninstall network self-healing",
		"安装网络自愈失败：%v":    "Failed to install network self-healing: %v",
		"安装每周自动更新定时器？":   "Install weekly auto-update timer?",
		"卸载每周更新":         "Uninstall weekly update",
		"安装每周更新定时器失败：%v": "Failed to install the weekly update timer: %v",
	})
}
