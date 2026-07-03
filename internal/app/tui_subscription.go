package app

import "fmt"

func subscriptionTUIItems() []tuiItem {
	return []tuiItem{
		{"查看订阅", "显示已保存的订阅和节点数量", commandAction("查看订阅", func(s *tuiSession) int {
			return runSub([]string{"list"}, s.stdout, s.stderr)
		})},
		{"添加远程链接", "导入 Clash、sing-box 或 base64 订阅链接", runTUIAddRemoteSubscription},
		{"添加本地配置", "将 config.yaml/json 复制到固定状态目录", runTUIAddLocalConfig},
		{"切换当前订阅", "选择哪个已保存订阅为当前运行配置提供数据", promptCommand("切换当前订阅", func(s *tuiSession) ([]string, bool) {
			name, ok := s.promptRequired("订阅名称")
			if !ok {
				return nil, false
			}
			return []string{"switch", "--name", name}, true
		}, runSub)},
		{"刷新订阅", "拉取最新远程内容并重建配置", promptCommand("刷新订阅", func(s *tuiSession) ([]string, bool) {
			name, ok := s.promptRequired("订阅名称")
			if !ok {
				return nil, false
			}
			args := []string{"refresh", "--name", name}
			if proxy := s.promptDefault("下载代理", ""); proxy != "" {
				args = append(args, "--proxy", proxy)
			}
			return args, true
		}, runSub)},
		{"重建当前配置", "从本地保存的原始来源重建，不再拉取", promptCommand("重建订阅", func(s *tuiSession) ([]string, bool) {
			name, ok := s.promptRequired("订阅名称")
			if !ok {
				return nil, false
			}
			return []string{"rebuild", "--name", name}, true
		}, runSub)},
		{"删除订阅", "删除已保存的订阅", promptCommand("删除订阅", func(s *tuiSession) ([]string, bool) {
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
	source := s.promptDefault("来源类型（clash、sing-box 或 base64）", "clash")
	name := s.promptDefault("名称", "main")
	url, ok := s.promptRequired("订阅链接")
	if !ok {
		return nil, false
	}
	args := []string{"add", "--name", name, "--source", source, "--url", url}
	if proxy := s.promptDefault("下载代理（默认留空）", ""); proxy != "" {
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
	name := s.promptDefault("名称", "local")
	filePath, ok := s.promptRequired("配置文件路径")
	if !ok {
		fmt.Fprintln(s.stdout, "已取消。")
		return 0
	}
	args := []string{"add", "--name", name, "--file", filePath}
	if source := s.promptDefault("来源覆盖（clash、sing-box、base64，留空自动识别）", ""); source != "" {
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
