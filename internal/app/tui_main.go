package app

import (
	"fmt"
	"io"
)

func mainTUIItems() []tuiItem {
	return []tuiItem{
		{"首次初始化", "初始化状态、导入订阅，并可选安装服务", runTUIFirstSetup},
		{"节点", "通过 sing-box Clash API 查看或切换节点组", submenu("节点", nodeTUIItems)},
		{"订阅", "添加、查看、切换、刷新、重建或移除订阅与本地配置", submenu("订阅", subscriptionTUIItems)},
		{"服务", "安装、同步、查看或移除 sboxkit.service", submenu("服务", serviceTUIItems)},
		{"运行时资源", "下载可选规则或更新内置核心缓存", submenu("运行时资源", updateTUIItems)},
		{"配置", "查看或编辑 customize.json、TUN、WebUI 与 Shell 代理设置", submenu("配置", configTUIItems)},
		{"网络测试", "通过本地代理测试延迟和出口 IP", commandAction("网络测试", func(s *tuiSession) int {
			printNetworkTestProgress(s.stdout)
			runNettest(s.stdout, "127.0.0.1:7890")
			return 0
		})},
		{"定时任务与恢复", "每周更新定时器和网络恢复守护", submenu("定时任务与恢复", timerTUIItems)},
		{"卸载", "移除系统集成，可选清理用户状态", submenu("卸载", uninstallTUIItems)},
		{"帮助", "打印命令行帮助", commandAction("帮助", func(s *tuiSession) int {
			printHelp(s.stdout)
			return 0
		})},
		{"退出", "退出 sboxkit", func(*tuiSession) bool { return true }},
	}
}

func printNetworkTestProgress(stdout io.Writer) {
	fmt.Fprintln(stdout, "正在通过 127.0.0.1:7890 测试网络...")
}
