package app

import "fmt"

func serviceTUIItems() []tuiItem {
	return []tuiItem{
		{"安装并启动服务", "同步运行时文件、安装 systemd 单元并重启服务", commandAction("安装服务", func(s *tuiSession) int {
			if !s.confirmServiceTrafficRisk("安装并启动 sboxkit.service") {
				fmt.Fprintln(s.stdout, "已取消。")
				return 0
			}
			return runService([]string{"install"}, s.stdout, s.stderr)
		})},
		{"安装但不启动", "安装单元和运行时文件，但保持服务停止", commandAction("安装服务但不启动", func(s *tuiSession) int {
			return runService([]string{"install", "--no-start"}, s.stdout, s.stderr)
		})},
		{"同步并重启", "将当前配置和资产复制到 /etc/sboxkit 并重启", commandAction("同步服务", func(s *tuiSession) int {
			if !s.confirmServiceTrafficRisk("同步并重启 sboxkit.service") {
				fmt.Fprintln(s.stdout, "已取消。")
				return 0
			}
			return runService([]string{"sync"}, s.stdout, s.stderr)
		})},
		{"状态", "查看 sboxkit.service 的 systemctl 状态", commandAction("服务状态", func(s *tuiSession) int {
			return runService([]string{"status"}, s.stdout, s.stderr)
		})},
		{"移除服务", "停止服务并移除 systemd 运行时文件", commandAction("移除服务", func(s *tuiSession) int {
			if !s.confirm("是否移除 sboxkit.service 和 /etc/sboxkit？", false) {
				return 0
			}
			return runService([]string{"remove"}, s.stdout, s.stderr)
		})},
	}
}

func updateTUIItems() []tuiItem {
	return []tuiItem{
		{"通过代理下载可选规则", "建议在服务运行后执行", commandAction("更新运行时资源", func(s *tuiSession) int {
			args := []string{"--proxy", s.promptDefault("代理地址", "http://127.0.0.1:7890")}
			if s.confirm("是否同步资产到服务并重启？", true) {
				if !s.confirmServiceTrafficRisk("同步资产并重启 sboxkit.service") {
					fmt.Fprintln(s.stdout, "已跳过服务同步。")
					return runUpdate(args, s.stdout, s.stderr)
				}
				args = append(args, "--sync-service")
			}
			return runUpdate(args, s.stdout, s.stderr)
		})},
		{"直接下载可选规则", "不使用代理获取大型规则集资产", commandAction("更新运行时资源", func(s *tuiSession) int {
			return runUpdate(nil, s.stdout, s.stderr)
		})},
		{"更新核心缓存和规则", "将 sing-box 内核下载到用户状态目录，并附带可选资产", commandAction("更新核心和规则", func(s *tuiSession) int {
			args := []string{"--core"}
			if s.confirm("是否强制重新下载？", false) {
				args = append(args, "--force")
			}
			return runUpdate(args, s.stdout, s.stderr)
		})},
	}
}

func nodeTUIItems() []tuiItem {
	return []tuiItem{
		{"查看节点", "读取正在运行的 Clash API 选择组", commandAction("查看节点", func(s *tuiSession) int {
			return runNode([]string{"list"}, s.stdout, s.stderr)
		})},
		{"切换节点", "无需重启 sing-box 即可切换选择组", promptCommand("切换节点", func(s *tuiSession) ([]string, bool) {
			group := s.promptDefault("组名", "Proxy")
			name, ok := s.promptRequired("节点名称")
			if !ok {
				return nil, false
			}
			return []string{"switch", "--group", group, "--name", name}, true
		}, runNode)},
	}
}

func timerTUIItems() []tuiItem {
	return []tuiItem{
		{"安装每周更新定时器", "安装 systemd 定时器以执行周期更新", commandAction("安装定时器", func(s *tuiSession) int {
			return runTimer([]string{"install", "--binary", "/usr/bin/sboxkit"}, s.stdout, s.stderr)
		})},
		{"移除每周更新定时器", "移除更新定时器", commandAction("移除定时器", func(s *tuiSession) int {
			return runTimer([]string{"remove"}, s.stdout, s.stderr)
		})},
		{"安装恢复守护", "安装网络自愈服务/定时器钩子", commandAction("安装恢复守护", func(s *tuiSession) int {
			return runResilience([]string{"install"}, s.stdout, s.stderr)
		})},
		{"移除恢复守护", "移除网络自愈集成", commandAction("移除恢复守护", func(s *tuiSession) int {
			return runResilience([]string{"remove"}, s.stdout, s.stderr)
		})},
	}
}

func uninstallTUIItems() []tuiItem {
	return []tuiItem{
		{"卸载系统集成", "移除服务、定时器、恢复守护和运行时文件", commandAction("卸载", func(s *tuiSession) int {
			if !s.confirm("是否卸载系统集成？", false) {
				return 0
			}
			return runUninstall(nil, s.stdout, s.stderr)
		})},
		{"卸载并清理用户状态", "同时移除订阅、生成配置、下载内容和 UI 状态", commandAction("卸载并清理状态", func(s *tuiSession) int {
			if !s.confirm("是否清理全部 sboxkit 用户状态？", false) {
				return 0
			}
			return runUninstall([]string{"--purge-state"}, s.stdout, s.stderr)
		})},
		{"查看 apt 卸载命令", "说明如何移除已安装的 .deb 包", commandAction("APT 包卸载说明", func(s *tuiSession) int {
			printPackageRemovalHint(s.stdout)
			return 0
		})},
	}
}
