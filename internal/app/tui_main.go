package app

import (
	"fmt"
	"io"
)

func mainTUIItems() []tuiItem {
	return mainTUIItemsFor(languageEnglish)
}

func mainTUIItemsFor(lang uiLanguage) []tuiItem {
	return []tuiItem{
		{label(lang, "No-Restart Changes", "无需重启配置"), label(lang, "Node operations and local settings that do not restart sing-box", "节点等不需要重启 sing-box 的本地操作"), submenu(label(lang, "No-Restart Changes", "无需重启配置"), func() []tuiItem { return noRestartTUIItemsFor(lang) })},
		{label(lang, "Restart Required", "需重启配置"), label(lang, "Initial setup, subscriptions, custom config, and runtime assets", "初始化、订阅、定制配置和运行时资源"), submenu(label(lang, "Restart Required", "需重启配置"), func() []tuiItem { return restartRequiredTUIItemsFor(lang) })},
		{label(lang, "Diagnostics", "诊断工具"), label(lang, "Network tests and important file locations", "网络测试和主要文件位置"), submenu(label(lang, "Diagnostics", "诊断工具"), func() []tuiItem { return diagnosticsTUIItemsFor(lang) })},
		{label(lang, "Service Control", "服务控制"), label(lang, "Start, stop, inspect, install, or sync sboxkit.service", "启动、暂停、查看、安装或同步 sboxkit.service"), submenu(label(lang, "Service Control", "服务控制"), func() []tuiItem { return serviceTUIItemsFor(lang) })},
		{"Language / 语言", label(lang, "Switch interface language", "切换界面语言"), runLanguageTUI},
		{label(lang, "Uninstall", "卸载"), label(lang, "Remove system integration and optionally purge user state", "移除系统集成，可选清理用户状态"), submenu(label(lang, "Uninstall", "卸载"), func() []tuiItem { return uninstallTUIItemsFor(lang) })},
	}
}

func noRestartTUIItemsFor(lang uiLanguage) []tuiItem {
	return []tuiItem{
		{label(lang, "Nodes", "节点"), label(lang, "Inspect or switch node groups through the sing-box Clash API", "通过 sing-box Clash API 查看或切换节点组"), submenu(label(lang, "Nodes", "节点"), func() []tuiItem { return nodeTUIItemsFor(lang) })},
		{label(lang, "Shell Proxy Environment", "Shell 代理环境"), label(lang, "Write or remove local shell proxy variables without restarting sing-box", "写入或移除本机 Shell 代理变量，不重启 sing-box"), submenu(label(lang, "Shell Proxy Environment", "Shell 代理环境"), func() []tuiItem { return proxyEnvConfigTUIItemsFor(lang) })},
		{label(lang, "Show Config", "显示配置"), label(lang, "Print the current customize.json", "打印当前 customize.json"), commandAction(label(lang, "Show Config", "显示配置"), func(s *tuiSession) int {
			return runConfig([]string{"show"}, s.stdout, s.stderr)
		})},
	}
}

func restartRequiredTUIItemsFor(lang uiLanguage) []tuiItem {
	return []tuiItem{
		{label(lang, "First Setup", "首次初始化"), label(lang, "Initialize state, import a subscription, and optionally install the service", "初始化状态、导入订阅，并可选安装服务"), runTUIFirstSetup},
		{label(lang, "Subscriptions", "订阅"), label(lang, "Add, view, switch, refresh, rebuild, or remove subscriptions and local configs", "添加、查看、切换、刷新、重建或移除订阅与本地配置"), submenu(label(lang, "Subscriptions", "订阅"), func() []tuiItem { return subscriptionTUIItemsFor(lang) })},
		{label(lang, "Custom Config", "定制配置"), label(lang, "Edit customize.json; changes usually need service sync/restart", "编辑 customize.json，通常需要同步并重启服务"), submenu(label(lang, "Custom Config", "定制配置"), func() []tuiItem { return configTUIItemsFor(lang) })},
		{label(lang, "Runtime Assets", "运行时资源"), label(lang, "Download optional rules or update the core cache", "下载可选规则或更新内核缓存"), submenu(label(lang, "Runtime Assets", "运行时资源"), func() []tuiItem { return updateTUIItemsFor(lang) })},
		{label(lang, "Timers and Recovery", "定时任务与恢复"), label(lang, "Weekly update timers and network recovery watchdog", "每周更新定时器和网络恢复守护"), submenu(label(lang, "Timers and Recovery", "定时任务与恢复"), func() []tuiItem { return timerTUIItemsFor(lang) })},
		{label(lang, "Help", "帮助"), label(lang, "Print command-line help", "打印命令行帮助"), commandAction(label(lang, "Help", "帮助"), func(s *tuiSession) int {
			printHelp(s.stdout)
			return 0
		})},
	}
}

func printNetworkTestProgress(stdout io.Writer) {
	fmt.Fprintln(stdout, "正在通过 127.0.0.1:7890 测试网络...")
}

func runLanguageTUI(s *tuiSession) bool {
	selected, ok := s.selectMenu("Language / 语言", []tuiItem{
		{Label: "English"},
		{Label: "中文"},
	}, int(s.languageIndex()))
	if !ok {
		return false
	}
	next := languageEnglish
	if selected == 1 {
		next = languageChinese
	}
	if err := s.setLanguage(next); err != nil {
		fmt.Fprintf(s.stderr, "save language preference: %v\n", err)
		s.wait()
		return false
	}
	fmt.Fprintf(s.stdout, "\n%s\n", label(s.language, "Language switched.", "语言已切换。"))
	s.wait()
	return false
}

func (s *tuiSession) languageIndex() int {
	if s.language == languageChinese {
		return 1
	}
	return 0
}
