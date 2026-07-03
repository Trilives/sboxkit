package app

import "fmt"

func subscriptionTUIItems() []tuiItem {
	return subscriptionTUIItemsFor(languageEnglish)
}

func subscriptionTUIItemsFor(lang uiLanguage) []tuiItem {
	return []tuiItem{
		{label(lang, "Switch Active Subscription", "切换当前订阅"), label(lang, "Choose which saved subscription feeds the active runtime config", "选择哪个已保存订阅为当前运行配置提供数据"), promptCommand(label(lang, "Switch Active Subscription", "切换当前订阅"), func(s *tuiSession) ([]string, bool) {
			name, ok := s.promptRequired("订阅名称")
			if !ok {
				return nil, false
			}
			return []string{"switch", "--name", name}, true
		}, runSub)},
		{label(lang, "Refresh Subscription", "刷新订阅"), label(lang, "Fetch latest remote content and rebuild config", "拉取最新远程内容并重建配置"), promptCommand(label(lang, "Refresh Subscription", "刷新订阅"), func(s *tuiSession) ([]string, bool) {
			name, ok := s.promptRequired("订阅名称")
			if !ok {
				return nil, false
			}
			args := []string{"refresh", "--name", name}
			proxy, ok := s.promptDefaultOK("下载代理", "")
			if !ok {
				return nil, false
			}
			if proxy != "" {
				args = append(args, "--proxy", proxy)
			}
			return args, true
		}, runSub)},
		{label(lang, "Add Remote URL", "添加远程链接"), label(lang, "Import a Clash, sing-box, or base64 subscription URL", "导入 Clash、sing-box 或 base64 订阅链接"), runTUIAddRemoteSubscription},
		{label(lang, "Add Local Config", "添加本地配置"), label(lang, "Copy config.yaml/json into the fixed state directory", "将 config.yaml/json 复制到固定状态目录"), runTUIAddLocalConfig},
		{label(lang, "List Subscriptions", "查看订阅"), label(lang, "Show saved subscriptions and node counts", "显示已保存的订阅和节点数量"), commandAction(label(lang, "List Subscriptions", "查看订阅"), func(s *tuiSession) int {
			return runSub([]string{"list"}, s.stdout, s.stderr)
		})},
		{label(lang, "Rebuild Current Config", "重建当前配置"), label(lang, "Rebuild from locally saved source without fetching", "从本地保存的原始来源重建，不再拉取"), promptCommand(label(lang, "Rebuild Subscription", "重建订阅"), func(s *tuiSession) ([]string, bool) {
			name, ok := s.promptRequired("订阅名称")
			if !ok {
				return nil, false
			}
			return []string{"rebuild", "--name", name}, true
		}, runSub)},
		{label(lang, "Remove Subscription", "删除订阅"), label(lang, "Remove a saved subscription", "删除已保存的订阅"), promptCommand(label(lang, "Remove Subscription", "删除订阅"), func(s *tuiSession) ([]string, bool) {
			name, ok := s.promptRequired("订阅名称")
			if !ok || !s.confirm("是否删除订阅 "+name+"？", false) {
				return nil, false
			}
			return []string{"remove", "--name", name}, true
		}, runSub)},
	}
}

func runTUIAddRemoteSubscription(s *tuiSession) bool {
	return commandAction("添加远程订阅", runTUIAddRemoteSubscriptionCommand)(s)
}

func runTUIAddRemoteSubscriptionCommand(s *tuiSession) int {
	args, ok := s.buildRemoteSubscriptionArgs()
	if !ok {
		fmt.Fprintln(s.stdout, "已取消。")
		return 0
	}
	fmt.Fprintln(s.stdout)
	return runSub(args, s.stdout, s.stderr)
}

func (s *tuiSession) buildRemoteSubscriptionArgs() ([]string, bool) {
	source, ok := s.promptDefaultOK("来源类型（clash、sing-box 或 base64）", "clash")
	if !ok {
		return nil, false
	}
	name, ok := s.promptDefaultOK("名称", "main")
	if !ok {
		return nil, false
	}
	url, ok := s.promptRequired("订阅链接")
	if !ok {
		return nil, false
	}
	args := []string{"add", "--name", name, "--source", source, "--url", url}
	proxy, ok := s.promptDefaultOK("下载代理（默认留空）", "")
	if !ok {
		return nil, false
	}
	if proxy != "" {
		args = append(args, "--proxy", proxy)
	}
	if !s.confirm("是否设为当前订阅？", true) {
		args = append(args, "--no-active")
	}
	return args, true
}

func runTUIAddLocalConfig(s *tuiSession) bool {
	return commandAction("添加本地配置", runTUIAddLocalConfigCommand)(s)
}

func runTUIAddLocalConfigCommand(s *tuiSession) int {
	name, ok := s.promptDefaultOK("名称", "local")
	if !ok {
		fmt.Fprintln(s.stdout, "已取消。")
		return 0
	}
	filePath, ok := s.promptRequired("配置文件路径")
	if !ok {
		fmt.Fprintln(s.stdout, "已取消。")
		return 0
	}
	args := []string{"add", "--name", name, "--file", filePath}
	source, ok := s.promptDefaultOK("来源覆盖（clash、sing-box、base64，留空自动识别）", "")
	if !ok {
		fmt.Fprintln(s.stdout, "已取消。")
		return 0
	}
	if source != "" {
		args = append(args, "--source", source)
	}
	if s.confirm("是否将 sing-box 配置原样透传？", false) {
		args = append(args, "--passthrough")
	}
	if !s.confirm("是否设为当前订阅？", true) {
		args = append(args, "--no-active")
	}
	fmt.Fprintln(s.stdout)
	return runSub(args, s.stdout, s.stderr)
}
