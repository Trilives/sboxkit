package i18n

func init() {
	register(map[string]string{
		"Clash 订阅（★推荐：机场通用格式，经 converter 转换为 sing-box 配置）": "Clash subscription (★recommended: common provider format, converted to a sing-box config)",
		"sing-box 原生订阅（机场直接提供 sing-box JSON）":              "Native sing-box subscription (provider directly serves sing-box JSON)",
		"通用 base64 订阅（经 subconverter 云端解析为 Clash）":         "Generic base64 subscription (parsed to Clash via cloud subconverter)",
		"本地文件（直接导入为订阅，不联网拉取）":                              "Local file (import directly as a subscription, no network fetch)",

		"订阅名称，留空=时间戳":                                "Subscription name, empty = timestamp",
		"选择订阅来源类型":                                   "Select subscription source type",
		"订阅链接，留空=暂不配置":                               "Subscription URL, empty = skip for now",
		"本地文件路径（Clash YAML 或 sing-box JSON），留空=暂不配置": "Local file path (Clash YAML or sing-box JSON), empty = skip for now",
		"按 sboxkit 统一规则重建该订阅（TUN / DNS / AI / 流媒体 / 地区自动测速组）？否则仅信任你的原生配置，只补齐面板/控制器设置。": "Rebuild this subscription using sboxkit's unified rules (TUN / DNS / AI / streaming / region auto-test groups)? Otherwise trust your native config as-is, only filling in panel/controller settings.",

		"创建数据目录 ": "Create data directory ",

		"未配置 GitHub Token，匿名 API 限额较低（60 次/小时），高频操作易被限流。": "No GitHub token configured; the anonymous API rate limit is low (60 requests/hour) and frequent operations may get throttled.",
		"现在添加 GitHub Token？":                "Add a GitHub token now?",
		"Token 保存失败：":                       "Failed to save token: ",
		"GitHub Token 已保存到 customize.json。": "GitHub token saved to customize.json.",

		"YAML 配置文件路径":              "Path to the YAML config file",
		"已导入 YAML 配置文件，并设为当前生效配置。": "YAML config file imported and set as the active config.",
		"服务已安装，立即同步并重启以使用该配置？":     "The service is installed; sync and restart now to use this config?",
		"本地文件路径不能为空":               "Local file path must not be empty",
		"读取本地文件: %w":               "Failed to read the local file: %w",
		"请输入文件路径，而不是目录: %s":        "Please provide a file path, not a directory: %s",
		"解析 YAML 配置文件: %w":         "Failed to parse the YAML config file: %w",
	})
}
