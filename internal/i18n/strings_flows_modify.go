package i18n

func init() {
	register(map[string]string{
		"同步服务到回退后的配置": "Sync service to the rolled-back config",
		"回退并退出":       "Roll back and exit",
		"保存并退出":       "Save and exit",

		"订阅管理（增 / 删 / 改名 / 切换 / 刷新）": "Subscription management (add / remove / rename / switch / refresh)",

		"节点切换":    "Node switch",
		"固定节点":    "Pin node",
		"服务设置":    "Service settings",
		"更新":      "Update",
		"网络自愈设置":  "Network self-healing settings",
		"每周更新定时器": "Weekly update timer",

		"内核":         "Core",
		"Web UI":     "Web UI",
		"geo 数据":     "Geo data",
		"sboxkit 自身": "sboxkit itself",

		"订阅管理":            "Subscription Management",
		"  • （暂无订阅）":      "  • (no subscriptions yet)",
		"%s  [%s, %d 节点]": "%s  [%s, %d nodes]",
		"  ← 生效":          "  ← active",
		"订阅操作":            "Subscription Actions",
		"添加订阅":            "Add subscription",
		"本地文件覆盖":          "Overwrite from local file",
		"切换生效订阅":          "Switch active subscription",
		"刷新订阅":            "Refresh subscription",
		"重命名":             "Rename",
		"删除订阅":            "Delete subscription",
		"返回上层":            "Back",

		"订阅已更新，是否现在切换 / 固定节点？": "Subscription updated. Switch / pin node now?",
		"订阅链接留空，已取消添加。":        "Subscription URL left empty; add cancelled.",
		"设为生效订阅？":              "Set as the active subscription?",
		"暂无订阅。":                "No subscriptions yet.",
		"切换到哪个订阅":              "Which subscription to switch to",
		"刷新哪个订阅":               "Which subscription to refresh",
		"重命名哪个订阅":              "Which subscription to rename",
		"新名称":                  "New name",
		"删除哪个订阅":               "Which subscription to delete",
		"确认删除订阅「%s」？":          "Confirm deleting subscription \"%s\"?",
		"立即用本地原文重新生成生效订阅并重启？（不重新拉取链接）": "Regenerate the active subscription from the local original now and restart? (won't re-fetch the URL)",
		"更新 内核 / UI / geo 数据？":         "Update core / UI / geo data?",
		"同时下载 / 更新 Web UI？":            "Also download / update the Web UI?",
		"服务同步失败：%v":                    "Service sync failed: %v",
	})
}
