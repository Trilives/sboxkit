package i18n

func init() {
	register(map[string]string{
		"[信息] ": "[info] ",
		"[完成] ": "[ok] ",
		"[注意] ": "[warn] ",
		"[错误] ": "[error] ",

		"命令失败(%d): %s": "command failed(%d): %s",
		"启动命令 %s: %w":  "failed to start command %s: %w",

		"需要管理员权限，但未找到 sudo，请改用 root 运行": "administrator privileges required, but sudo was not found; please run as root instead",
		"需要管理员权限。": "requires administrator privileges.",
		"提示：也可以直接用 sudo 启动，避免中途输入密码。": "tip: you can also start directly with sudo to avoid a mid-run password prompt.",
		"该操作": "this operation",
	})
}
