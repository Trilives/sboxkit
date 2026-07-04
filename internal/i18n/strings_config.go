package i18n

func init() {
	register(map[string]string{
		"customize.json 读取失败，使用默认值：": "Failed to read customize.json, using defaults: ",
		"customize.json 解析失败，使用默认值：": "Failed to parse customize.json, using defaults: ",
		"序列化字段 %s: %w":               "Failed to serialize field %s: %w",

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
		"AI 域名后缀（叠加）":             "AI domain suffixes (overlay)",
		"流媒体域名后缀（叠加）":             "Streaming domain suffixes (overlay)",
		"直连域名后缀（叠加）":              "Direct domain suffixes (overlay)",
		"强制直连端口（叠加，默认 22/SSH）":    "Ports forced direct (overlay, default 22/SSH)",
		"新加坡关键词（叠加）":              "Singapore keywords (overlay)",
		"香港关键词（叠加）":               "Hong Kong keywords (overlay)",

		// BoolFields
		"TUN 模式（全局透明代理）":              "TUN mode (global transparent proxy)",
		"局域网代理（其他主机可用本机代理）":           "LAN proxy (other hosts can use this machine as a proxy)",
		"LAN 面板暴露":                    "Expose panel on LAN",
		"生成新加坡自动测速聚合组（SG-Auto，可直接选用）": "Generate Singapore auto url-test group (SG-Auto, directly selectable)",
		"生成香港自动测速聚合组（HK-Auto，可直接选用）":  "Generate Hong Kong auto url-test group (HK-Auto, directly selectable)",
		"启用自定义分流叠加（AI / 流媒体）":         "Enable custom routing overlay (AI / streaming)",
		"base64 应急本地解析":               "base64 local fallback parsing",
		"启用日志（写入文件，超限自动裁剪旧内容）":        "Enable logging (write to file, auto-trims oldest content past the size cap)",

		// ScalarFields
		"TUN 协议栈（gvisor/system/mixed）": "TUN stack (gvisor/system/mixed)",
		"面板密钥 secret":                  "Panel secret",
		"引导 DNS 服务器":                   "Bootstrap DNS server",
		"引导 DNS 端口":                    "Bootstrap DNS port",
		"subconverter 后端":              "subconverter backend",
		"GitHub 加速前缀":                  "GitHub mirror prefix",
		"GitHub Token（提升 API 限额）":      "GitHub token (raises API rate limit)",
		"下载代理":                         "Download proxy",
	})
}
