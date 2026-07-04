package i18n

func init() {
	register(map[string]string{
		"信息": "Info",

		"尚未生成生效配置（先添加订阅并启动服务）。": "No active config yet (add a subscription and start the service first).",

		"代理端口（HTTP + SOCKS5 共用）": "Proxy port (shared HTTP + SOCKS5)",
		"mixed-port，同一端口两种协议都能用": "mixed-port — both protocols work on the same port",
		"局域网代理":  "LAN proxy",
		"TUN 模式": "TUN mode",
	})
}
