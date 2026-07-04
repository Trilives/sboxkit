package i18n

func init() {
	register(map[string]string{
		"内容看起来更像「%s」而非你选择的「%s」。": "The content looks more like \"%s\" than the \"%s\" you selected.",

		"拉取订阅失败: %w":            "Failed to fetch subscription: %w",
		"拉取失败（%v），第 %d/%d 次重试…": "Fetch failed (%v), retrying (%d/%d)…",

		"subconverter 返回内容无法解析为 Clash":                               "subconverter's response could not be parsed as Clash config",
		"subconverter 解析失败：%w。可更换后端，或开启应急本地解析 base64_local_fallback": "subconverter parsing failed: %w. Try a different backend, or enable the local fallback parser (base64_local_fallback)",
		"subconverter 失败，改用应急本地解析：%v":                                "subconverter failed, falling back to local parsing: %v",
		"未配置 subconverter 后端，且未开启应急本地解析（base64_local_fallback）":      "No subconverter backend configured, and the local fallback parser (base64_local_fallback) is not enabled",
		"本地解析未得到任何节点":                                                "Local parsing yielded no nodes",
		"subconverter 返回内容不含 proxies，可能后端不可用或订阅无效":                   "subconverter's response has no proxies; the backend may be unavailable or the subscription invalid",

		"订阅「%s」已存在，请改名或先删除":                     "Subscription \"%s\" already exists; rename or delete it first",
		"订阅不存在: %s":                             "Subscription does not exist: %s",
		"本地缺少订阅原文，改为联网刷新。":                      "Local subscription source is missing, refreshing from the network instead.",
		"用本地原文重新生成「%s」（不重新拉取）…":                 "Regenerating \"%s\" from the local source (not re-fetching)…",
		"拉取订阅「%s」…":                             "Fetching subscription \"%s\"…",
		"读取本地文件生成订阅「%s」…":                       "Reading local file to build subscription \"%s\"…",
		"读取本地文件: %w":                            "Failed to read the local file: %w",
		"生成 sing-box 配置（原生订阅）…":                 "Generating sing-box config (native subscription)…",
		"经 subconverter/本地解析将 base64 转为 Clash…": "Converting base64 to Clash via subconverter/local parsing…",
		"生成 sing-box 配置…":                       "Generating sing-box config…",
		"订阅「%s」就绪：%v 个节点":                       "Subscription \"%s\" ready: %v node(s)",

		"已切换生效订阅: ":                "Active subscription switched to: ",
		"配置已切换，但同步到服务失败：%v":        "Config switched, but syncing to the service failed: %v",
		"已删除当前生效订阅；请切换到其它订阅或重新添加。": "The active subscription was deleted; please switch to another subscription or add a new one.",
		"已删除订阅: ":                  "Subscription deleted: ",
		"目标名已存在: %s":               "Target name already exists: %s",
		"已改名: %s → %s":             "Renamed: %s → %s",
	})
}
