package i18n

func init() {
	register(map[string]string{
		// internal/firewall
		"未探测到防火墙工具，请自行确认放行 %d/tcp,udp（或本机无防火墙）。": "No firewall tool detected; please manually confirm port %d/tcp,udp is open (or this machine has no firewall).",
		"经 %s 放行 %d/tcp,udp …": "Allowing %d/tcp,udp via %s…",
		"已放行 %d 端口（%s）。":       "Port %d allowed (%s).",
		"经 %s 撤销放行 %d …":       "Revoking allow-rule for %d via %s…",
		"更新防火墙":                "Update firewall",
		"nftables 规则请手动移除：nft -a list chain inet filter input 查看句柄后 delete。": "For nftables, please remove the rule manually: run `nft -a list chain inet filter input` to find the handle, then `delete` it.",

		// internal/proxyenv
		"已写入代理环境变量到 %s（新开终端生效；当前终端可 `source %s`）。": "Proxy environment variables written to %s (effective in new terminals; run `source %s` in the current one).",
		"已从 %s 移除代理环境变量。":                          "Proxy environment variables removed from %s.",

		// internal/fetchx
		"直连可达，跳过代理。":           "Direct connection reachable, skipping proxy.",
		"  %s 通道失败（%v），改直连重试…": "  %s channel failed (%v), retrying direct…",

		// internal/configfile
		"配置根节点不是映射":   "The config root node is not a mapping",
		"解析配置 %s: %w": "Failed to parse config %s: %w",

		// internal/txn
		"已取消「%s」。":        "\"%s\" cancelled.",
		"「%s」出错：%v":       "\"%s\" failed: %v",
		"还原文件 ":           "Restore file ",
		"删除新建文件 ":         "Delete newly created file ",
		"还原 ":             "Restore ",
		"删除新建路径 ":         "Delete newly created path ",
		"正在回退「%s」已应用的改动…": "Rolling back changes applied by \"%s\"…",
		"  回退失败: %s (%v)": "  Rollback failed: %s (%v)",
		"  已回退: ":         "  Rolled back: ",
		"回退完成，但有 %d 项失败，请手动检查。": "Rollback finished, but %d item(s) failed; please check manually.",
		"已回退到操作前状态。":            "Rolled back to the state before the operation.",
	})
}
