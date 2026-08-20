package i18n

func init() {
	register(map[string]string{
		"信息": "Info",

		"尚未生成生效配置（先添加订阅并启动服务）。": "No active config yet (add a subscription and start the service first).",

		"代理端口（HTTP + SOCKS5 共用）": "Proxy port (shared HTTP + SOCKS5)",
		"mixed-port，同一端口两种协议都能用": "mixed-port — both protocols work on the same port",
		"局域网代理":  "LAN proxy",
		"TUN 模式": "TUN mode",

		"网络自愈":                      "Network self-healing",
		"状态检查失败":                    "status check failed",
		"NetworkManager dispatcher": "NetworkManager dispatcher",
		"watchdog service":          "watchdog service",
		"watchdog timer":            "watchdog timer",
		"不可用（未检测到 NetworkManager）":  "unavailable (NetworkManager not detected)",
		"已安装（内容过期）":                 "installed (stale content)",
		"已启用":                       "enabled",
		"未启用":                       "disabled",
		"未运行":                       "inactive",
		"网络自愈偏好":                    "Self-healing preference",
		"已明确禁用":                     "explicitly disabled",
	})
}
