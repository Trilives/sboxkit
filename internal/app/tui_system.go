package app

import "fmt"

func serviceTUIItems() []tuiItem {
	return serviceTUIItemsFor(languageEnglish)
}

func serviceTUIItemsFor(lang uiLanguage) []tuiItem {
	return []tuiItem{
		{label(lang, "Start Service", "启动服务"), label(lang, "Start the installed systemd service. Use Custom Layer Config to install or sync files.", "启动已安装的 systemd 服务；安装或同步请到定制层配置。"), commandAction(label(lang, "Start Service", "启动服务"), func(s *tuiSession) int {
			if !s.confirmServiceRestart(label(lang, "Start sboxkit.service? This may interrupt SSH if TUN or routing changes take effect.", "启动 sboxkit.service 吗？如果 TUN 或路由变更生效，可能中断当前 SSH。"), false) {
				fmt.Fprintln(s.stdout, "已取消。")
				return 0
			}
			return runService([]string{"start"}, s.stdout, s.stderr)
		})},
		{label(lang, "Stop Service", "暂停服务"), label(lang, "Stop sboxkit.service without removing files", "停止 sboxkit.service，不移除文件"), commandAction(label(lang, "Stop Service", "暂停服务"), func(s *tuiSession) int {
			return runService([]string{"stop"}, s.stdout, s.stderr)
		})},
		{label(lang, "Service Status", "服务状态"), label(lang, "Show systemctl status for sboxkit.service", "查看 sboxkit.service 的 systemctl 状态"), commandAction(label(lang, "Service Status", "服务状态"), func(s *tuiSession) int {
			return runService([]string{"status"}, s.stdout, s.stderr)
		})},
	}
}

func updateTUIItems() []tuiItem {
	return updateTUIItemsFor(languageEnglish)
}

func updateTUIItemsFor(lang uiLanguage) []tuiItem {
	return []tuiItem{
		{label(lang, "Download Optional Rules via Proxy", "通过代理下载可选规则"), label(lang, "Recommended after the service is running", "建议在服务运行后执行"), commandAction(label(lang, "Update Runtime Assets", "更新运行时资源"), func(s *tuiSession) int {
			proxy, ok := s.promptDefaultOK("代理地址", "http://127.0.0.1:7890")
			if !ok {
				fmt.Fprintln(s.stdout, "已取消。")
				return 0
			}
			args := []string{"--proxy", proxy}
			if s.confirmServiceRestart(label(s.language, "Sync assets and restart the service after download?", "下载后同步资产并重启服务吗？"), true) {
				args = append(args, "--sync-service")
			}
			return runUpdate(args, s.stdout, s.stderr)
		})},
		{label(lang, "Download Optional Rules Directly", "直接下载可选规则"), label(lang, "Fetch large rule-set assets without a proxy", "不使用代理获取大型规则集资产"), commandAction(label(lang, "Update Runtime Assets", "更新运行时资源"), func(s *tuiSession) int {
			return runUpdate(nil, s.stdout, s.stderr)
		})},
		{label(lang, "Update Core Cache and Rules", "更新核心缓存和规则"), label(lang, "Download the sing-box core into user state and include optional assets", "将 sing-box 内核下载到用户状态目录，并附带可选资产"), commandAction(label(lang, "Update Core and Rules", "更新核心和规则"), func(s *tuiSession) int {
			args := []string{"--core"}
			if s.confirm("是否强制重新下载？", false) {
				args = append(args, "--force")
			}
			return runUpdate(args, s.stdout, s.stderr)
		})},
		{label(lang, "Update sboxkit Stable", "更新 sboxkit 稳定版"), label(lang, "Update this application from the latest stable release", "从最新稳定版发布更新本体"), commandAction(label(lang, "Update sboxkit Stable", "更新 sboxkit 稳定版"), func(s *tuiSession) int {
			return runUpdate([]string{"--self", "--channel", "stable"}, s.stdout, s.stderr)
		})},
		{label(lang, "Update sboxkit Preview", "更新 sboxkit 预览版"), label(lang, "Update this application from the latest preview release", "从最新预览版发布更新本体"), commandAction(label(lang, "Update sboxkit Preview", "更新 sboxkit 预览版"), func(s *tuiSession) int {
			return runUpdate([]string{"--self", "--channel", "preview"}, s.stdout, s.stderr)
		})},
	}
}

func nodeTUIItems() []tuiItem {
	return nodeTUIItemsFor(languageEnglish)
}

func nodeTUIItemsFor(lang uiLanguage) []tuiItem {
	return []tuiItem{
		{label(lang, "Switch Node", "切换节点"), label(lang, "Switch selector groups without restarting sing-box", "无需重启 sing-box 即可切换选择组"), runSwitchNodeTUI},
		{label(lang, "List Nodes", "查看节点"), label(lang, "Read selector groups from the running Clash API", "读取正在运行的 Clash API 选择组"), commandAction(label(lang, "List Nodes", "查看节点"), func(s *tuiSession) int {
			return runNode([]string{"list"}, s.stdout, s.stderr)
		})},
	}
}

func runSwitchNodeTUI(s *tuiSession) bool {
	return commandAction(label(s.language, "Switch Node", "切换节点"), func(s *tuiSession) int {
		args, syncService, ok := s.buildSwitchNodeArgs()
		if !ok {
			fmt.Fprintln(s.stdout, "已取消。")
			return 0
		}
		if syncService {
			args = append(args, "--sync-service")
		}
		return runNode(args, s.stdout, s.stderr)
	})(s)
}

func (s *tuiSession) buildSwitchNodeArgs() ([]string, bool, bool) {
	group, ok := s.promptDefaultOK(label(s.language, "Group", "组名"), "Proxy")
	if !ok {
		return nil, false, false
	}
	name, ok := s.promptRequired(label(s.language, "Node name", "节点名称"))
	if !ok {
		return nil, false, false
	}
	args := []string{"switch", "--group", group, "--name", name}
	if !s.confirm(label(s.language, "Also move this node to the front of the generated config?", "是否同时调整配置中的节点顺序？"), false) {
		return args, false, true
	}
	args = append(args, "--reorder")
	syncService := s.confirmServiceRestart(label(s.language, "Sync and restart the service now to apply the new order?", "是否立即同步并重启服务以应用节点顺序？"), false)
	return args, syncService, true
}

func timerTUIItems() []tuiItem {
	return timerTUIItemsFor(languageEnglish)
}

func timerTUIItemsFor(lang uiLanguage) []tuiItem {
	return []tuiItem{
		{label(lang, "Install Weekly Update Timer", "安装每周更新定时器"), label(lang, "Install a systemd timer for periodic updates", "安装 systemd 定时器以执行周期更新"), commandAction(label(lang, "Install Timer", "安装定时器"), func(s *tuiSession) int {
			return runTimer([]string{"install", "--binary", "/usr/bin/sboxkit"}, s.stdout, s.stderr)
		})},
		{label(lang, "Remove Weekly Update Timer", "移除每周更新定时器"), label(lang, "Remove the update timer", "移除更新定时器"), commandAction(label(lang, "Remove Timer", "移除定时器"), func(s *tuiSession) int {
			return runTimer([]string{"remove"}, s.stdout, s.stderr)
		})},
		{label(lang, "Install Recovery Watchdog", "安装恢复守护"), label(lang, "Install network recovery service/timer hooks", "安装网络自愈服务/定时器钩子"), commandAction(label(lang, "Install Recovery Watchdog", "安装恢复守护"), func(s *tuiSession) int {
			return runResilience([]string{"install"}, s.stdout, s.stderr)
		})},
		{label(lang, "Remove Recovery Watchdog", "移除恢复守护"), label(lang, "Remove network recovery integration", "移除网络自愈集成"), commandAction(label(lang, "Remove Recovery Watchdog", "移除恢复守护"), func(s *tuiSession) int {
			return runResilience([]string{"remove"}, s.stdout, s.stderr)
		})},
	}
}

func uninstallTUIItems() []tuiItem {
	return uninstallTUIItemsFor(languageEnglish)
}

func uninstallTUIItemsFor(lang uiLanguage) []tuiItem {
	return []tuiItem{
		{label(lang, "Uninstall System Integration", "卸载系统集成"), label(lang, "Remove service, timers, recovery watchdog, and runtime files", "移除服务、定时器、恢复守护和运行时文件"), commandAction(label(lang, "Uninstall", "卸载"), func(s *tuiSession) int {
			if !s.confirm("是否卸载系统集成？", false) {
				return 0
			}
			return runUninstall(nil, s.stdout, s.stderr)
		})},
		{label(lang, "Uninstall and Purge User State", "卸载并清理用户状态"), label(lang, "Also remove subscriptions, generated config, downloads, and UI state", "同时移除订阅、生成配置、下载内容和 UI 状态"), commandAction(label(lang, "Uninstall and Purge State", "卸载并清理状态"), func(s *tuiSession) int {
			if !s.confirm("是否清理全部 sboxkit 用户状态？", false) {
				return 0
			}
			return runUninstall([]string{"--purge-state"}, s.stdout, s.stderr)
		})},
		{label(lang, "Show apt Removal Commands", "查看 apt 卸载命令"), label(lang, "Explain how to remove the installed .deb package", "说明如何移除已安装的 .deb 包"), commandAction(label(lang, "APT Package Removal", "APT 包卸载说明"), func(s *tuiSession) int {
			printPackageRemovalHint(s.stdout)
			return 0
		})},
	}
}

func systemTUIItemsFor(lang uiLanguage) []tuiItem {
	return []tuiItem{
		{label(lang, "Network Test", "网络测试"), label(lang, "Test latency and exit IP through the local proxy", "通过本地代理测试延迟和出口 IP"), commandAction(label(lang, "Network Test", "网络测试"), func(s *tuiSession) int {
			printNetworkTestProgress(s.stdout)
			runNettest(s.stdout, "127.0.0.1:7890")
			return 0
		})},
		{label(lang, "Service", "服务"), label(lang, "Install, sync, inspect, or remove sboxkit.service", "安装、同步、查看或移除 sboxkit.service"), submenu(label(lang, "Service", "服务"), func() []tuiItem { return serviceTUIItemsFor(lang) })},
		{label(lang, "Runtime Assets", "运行时资源"), label(lang, "Download optional rules or update the core cache", "下载可选规则或更新内置核心缓存"), submenu(label(lang, "Runtime Assets", "运行时资源"), func() []tuiItem { return updateTUIItemsFor(lang) })},
		{label(lang, "Timers and Recovery", "定时任务与恢复"), label(lang, "Weekly update timers and network recovery watchdog", "每周更新定时器和网络恢复守护"), submenu(label(lang, "Timers and Recovery", "定时任务与恢复"), func() []tuiItem { return timerTUIItemsFor(lang) })},
		{label(lang, "Uninstall", "卸载"), label(lang, "Remove system integration and optionally purge user state", "移除系统集成，可选清理用户状态"), submenu(label(lang, "Uninstall", "卸载"), func() []tuiItem { return uninstallTUIItemsFor(lang) })},
	}
}
