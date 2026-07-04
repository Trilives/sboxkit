package i18n

func init() {
	register(map[string]string{
		"systemd 服务":                    "systemd service",
		"网络自愈（NM 钩子 + watchdog）":        "Network self-healing (NM hook + watchdog)",
		"每周更新定时器":                       "Weekly update timer",
		"清理产物（内核 / UI / 下载缓存 / geo 数据）": "Clean up artifacts (core / UI / download cache / geo data)",
		"清理所有订阅与配置（含整个状态目录）":            "Clean up all subscriptions and config (entire state directory)",

		"卸载（勾选要移除的项）": "Uninstall (check items to remove)",
		"未选择任何项，已取消。": "Nothing selected, cancelled.",
		"即将卸载":        "About to Uninstall",
		"确认执行？":       "Confirm?",
		"已取消。":        "Cancelled.",

		"已清理本地产物（内核 / UI / 缓存 / geo 数据）。": "Cleaned up local artifacts (core / UI / cache / geo data).",
		"已清理状态目录（所有订阅与配置）。":               "Cleaned up the state directory (all subscriptions and config).",
		"移除「%s」失败：%v":                     "Failed to remove \"%s\": %v",
		"卸载流程结束。":                         "Uninstall process finished.",
	})
}
