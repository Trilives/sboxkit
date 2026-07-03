package app

import "fmt"

func serviceTUIItems() []tuiItem {
	return serviceTUIItemsFor(languageEnglish)
}

func serviceTUIItemsFor(lang uiLanguage) []tuiItem {
	return []tuiItem{
		{label(lang, "Status", "状态"), label(lang, "Show systemctl status for sboxkit.service", "查看 sboxkit.service 的 systemctl 状态"), commandAction(label(lang, "Service Status", "服务状态"), func(s *tuiSession) int {
			return runService([]string{"status"}, s.stdout, s.stderr)
		})},
		{label(lang, "Start / Restart Service", "启动/重启服务"), label(lang, "Start the existing systemd service without changing files", "启动已有 systemd 服务，不改运行文件"), commandAction(label(lang, "Start Service", "启动服务"), func(s *tuiSession) int {
			if !s.confirmServiceTrafficRisk("启动或重启 sboxkit.service") {
				fmt.Fprintln(s.stdout, "已取消。")
				return 0
			}
			return runService([]string{"start"}, s.stdout, s.stderr)
		})},
		{label(lang, "Stop Service", "暂停服务"), label(lang, "Stop sboxkit.service without removing files", "停止 sboxkit.service，不移除文件"), commandAction(label(lang, "Stop Service", "暂停服务"), func(s *tuiSession) int {
			return runService([]string{"stop"}, s.stdout, s.stderr)
		})},
		{label(lang, "Sync and Restart", "同步并重启"), label(lang, "Copy current config and assets to /etc/sboxkit and restart", "将当前配置和资产复制到 /etc/sboxkit 并重启"), commandAction(label(lang, "Sync Service", "同步服务"), func(s *tuiSession) int {
			if !s.confirmServiceTrafficRisk("同步并重启 sboxkit.service") {
				fmt.Fprintln(s.stdout, "已取消。")
				return 0
			}
			return runService([]string{"sync"}, s.stdout, s.stderr)
		})},
		{label(lang, "Install and Start Service", "安装并启动服务"), label(lang, "Sync runtime files, install the systemd unit, and restart the service", "同步运行时文件、安装 systemd 单元并重启服务"), commandAction(label(lang, "Install Service", "安装服务"), func(s *tuiSession) int {
			if !s.confirmServiceTrafficRisk("安装并启动 sboxkit.service") {
				fmt.Fprintln(s.stdout, "已取消。")
				return 0
			}
			return runService([]string{"install"}, s.stdout, s.stderr)
		})},
		{label(lang, "Install Without Starting", "安装但不启动"), label(lang, "Install the unit and runtime files but keep the service stopped", "安装单元和运行时文件，但保持服务停止"), commandAction(label(lang, "Install Without Starting", "安装服务但不启动"), func(s *tuiSession) int {
			return runService([]string{"install", "--no-start"}, s.stdout, s.stderr)
		})},
		{label(lang, "Remove Service", "移除服务"), label(lang, "Stop the service and remove systemd runtime files", "停止服务并移除 systemd 运行时文件"), commandAction(label(lang, "Remove Service", "移除服务"), func(s *tuiSession) int {
			if !s.confirm("是否移除 sboxkit.service 和 /etc/sboxkit？", false) {
				return 0
			}
			return runService([]string{"remove"}, s.stdout, s.stderr)
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
			if s.confirm("是否同步资产到服务并重启？", true) {
				if !s.confirmServiceTrafficRisk("同步资产并重启 sboxkit.service") {
					fmt.Fprintln(s.stdout, "已跳过服务同步。")
					return runUpdate(args, s.stdout, s.stderr)
				}
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
			if !s.confirmServiceTrafficRisk("同步节点顺序并重启 sboxkit.service") {
				fmt.Fprintln(s.stdout, "已跳过服务同步。")
			} else {
				args = append(args, "--sync-service")
			}
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
	syncService := s.confirm(label(s.language, "Sync and restart the service now to apply the new order?", "是否立即同步并重启服务以应用节点顺序？"), false)
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
