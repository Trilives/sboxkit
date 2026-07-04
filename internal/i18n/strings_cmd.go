package i18n

func init() {
	register(map[string]string{
		"sing-box 部署系统": "sing-box deployment system",
		"退出":            "Exit",
		"再见。":           "Bye.",

		"初始化（首次部署）":     "Initialize (first-time deployment)",
		"配置变更":          "Config Changes",
		"运行时管理":         "Runtime Management",
		"工具":            "Tools",
		"卸载":            "Uninstall",
		"语言 / Language": "Language / 语言",

		"暂停 / 启动服务": "Pause / Start Service",
		"暂停服务 ⏸":    "Pause Service ⏸",
		"启动服务 ▶":    "Start Service ▶",

		"未检测到已注册的服务，是否现在进行初始化？": "No registered service detected. Run initialization now?",

		"用法: sboxkit [init|modify|nettest|uninstall|update|pause|resume|version]\n不带参数则进入交互式主菜单。": "Usage: sboxkit [init|modify|nettest|uninstall|update|pause|resume|version]\nRun without arguments to enter the interactive main menu.",
		"未知子命令: %s\n%s\n": "Unknown subcommand: %s\n%s\n",
	})
}
