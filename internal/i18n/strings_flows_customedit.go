package i18n

func init() {
	register(map[string]string{
		"编辑定制层":   "Edit Customization Layer",
		"放弃修改并退出": "Discard changes and exit",
		"返回上层":    "Back",

		"部署设置（TUN / 面板 / 下载）":     "Deployment Settings (TUN / Panel / Downloads)",
		"自定义分流叠加（AI / 流媒体 / 地区组）": "Custom Routing Overlay (AI / Streaming / Region Groups)",

		"未做修改。":         "No changes made.",
		"定制层已保存。":       "Customization layer saved.",
		"已放弃本次修改（未写盘）。": "Changes discarded (not written to disk).",

		"已开启局域网代理，更新防火墙放行 7890 端口？": "LAN proxy enabled. Update firewall to allow port 7890?",
		"已关闭局域网代理，撤销防火墙放行 7890 端口？": "LAN proxy disabled. Revoke firewall rule for port 7890?",

		"%s：当前 %d 条%s": "%s: currently %d entries%s",
		"编辑 · ":        "Edit · ",
		"添加一条":         "Add an entry",
		"删除一条":         "Remove an entry",
		"批量粘贴替换（逗号/空格分隔）": "Bulk paste to replace (comma/space separated)",
		"恢复默认":        "Restore default",
		"清空":          "Clear",
		"新增值":         "New value",
		"删除哪一条":       "Which entry to remove",
		"粘贴（逗号或空格分隔）": "Paste (comma or space separated)",
		"输入无效，已跳过。":   "Invalid input, skipped.",
		"（留空清除）":      " (leave empty to clear)",
		"端口需为整数，未修改。": "Port must be an integer, not modified.",
	})
}
