package i18n

func init() {
	register(map[string]string{
		"customize.json 读取失败，使用默认值：": "Failed to read customize.json, using defaults: ",
		"customize.json 解析失败，使用默认值：": "Failed to parse customize.json, using defaults: ",

		"已设置（***%s）": "Set (***%s)",
		"已设置（***）":   "Set (***)",

		"未设置":    "Not set",
		"开":      "On",
		"关":      "Off",
		"空":      "Empty",
		"%d 条":   "%d entries",
		"%s（%s）": "%s (%s)",
		"%s：%s":  "%s: %s",

		// ListFields
		"TUN 排除网段":   "TUN excluded CIDRs",
		"TUN 排除 UID": "TUN excluded UIDs",
		"主选择组识别关键词（按顺序匹配，新增项插最前）": "Main-group identification keywords (matched in order; new entries are inserted at the front)",
		"FakeIP 排除规则（追加）":         "Additional FakeIP exclusions",
		"AI 域名后缀":                 "AI domain suffixes",
		"流媒体域名后缀":                 "Streaming domain suffixes",
		"直连域名后缀":                  "Direct domain suffixes",
		"本地域名绕过":                  "Local domain bypass",
		"绕过进程名":                   "TUN direct process names",
		"新加坡关键词":                  "Singapore keywords",
		"香港关键词":                   "Hong Kong keywords",

		// BoolFields
		"TUN 模式（全局透明代理）":                  "TUN mode (global transparent proxy)",
		"局域网代理（其他主机可用本机代理）":               "LAN proxy (other hosts can use this machine as a proxy)",
		"base64 应急本地解析":                   "base64 local fallback parsing",
		"网络自愈":                            "Network self-healing",
		"面板对外监听（0.0.0.0，否则仅本机 127.0.0.1）": "Panel listens on all interfaces (0.0.0.0; otherwise 127.0.0.1 only)",
		"生成新加坡自动测速聚合组":                    "Generate a Singapore auto url-test group",
		"生成香港自动测速聚合组":                     "Generate a Hong Kong auto url-test group",
		"保存文件日志":                          "Write a log file",

		// ScalarFields
		"本地代理 mixed 端口":                     "Local mixed proxy port",
		"端口必须是 1-65535 之间的整数":               "Port must be an integer between 1 and 65535",
		"面板密钥 secret":                       "Panel secret",
		"引导 DNS 服务器":                        "Bootstrap DNS server",
		"引导 DNS 端口":                         "Bootstrap DNS port",
		"引导 DNS 类型":                         "Bootstrap DNS type",
		"引导 DNS HTTPS 路径":                   "Bootstrap DNS HTTPS path",
		"引导 DNS TLS 服务器名称":                  "Bootstrap DNS TLS server name",
		"引导 DNS 类型必须是 udp、tcp、https 或 dhcp": "Bootstrap DNS type must be udp, tcp, https, or dhcp",
		"引导 DNS 服务器必须是 IP 地址":               "Bootstrap DNS server must be an IP address",
		"引导 DNS HTTPS 路径必须以 / 开头":           "Bootstrap DNS HTTPS path must start with /",
		"引导 DNS TLS 服务器名称不能为空":              "Bootstrap DNS TLS server name cannot be empty",
		"subconverter 后端":                   "subconverter backend",
		"GitHub 加速前缀":                       "GitHub mirror prefix",
		"下载代理":                              "Download proxy",
		"默认出站（节点切换的目标分组）":                   "Default outbound (the group that node switching targets)",
		"日志大小上限（MB）":                        "Log size limit (MB)",
		"下载代理需为 http(s):// 或 socks5(h):// 地址，例如 http://127.0.0.1:7890": "Download proxy must be an http(s):// or socks5(h):// URL, e.g. http://127.0.0.1:7890",
	})
}
