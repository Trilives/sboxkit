package app

import (
	"fmt"

	"github.com/Trilives/sboxkit/internal/paths"
	"github.com/Trilives/sboxkit/internal/subscription"
	ui "github.com/Trilives/sboxkit/internal/tui"
)

func subscriptionTUIItems() []tuiItem {
	return subscriptionTUIItemsFor(languageEnglish)
}

func subscriptionTUIItemsFor(lang uiLanguage) []tuiItem {
	return []tuiItem{
		{label(lang, "Switch Active Subscription", "切换当前订阅"), label(lang, "Choose which saved subscription feeds the active runtime config", "选择哪个已保存订阅为当前运行配置提供数据"), promptCommand(label(lang, "Switch Active Subscription", "切换当前订阅"), func(s *tuiSession) ([]string, bool) {
			args, ok := s.buildSwitchSubscriptionArgs()
			if !ok {
				return nil, false
			}
			return args, true
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
		{label(lang, "Add Subscription", "添加订阅"), label(lang, "Import a remote URL or a local file as a saved subscription", "将远程链接或本地文件导入为保存的订阅"), runTUIAddSubscription},
		{label(lang, "Overwrite Current From Local File", "用本地文件覆盖当前配置"), label(lang, "Copy a local yaml/json file into the active subscription slot", "将本地 yaml/json 文件复制并切换为当前订阅"), runTUIOverwriteCurrentFromLocalFile},
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

func (s *tuiSession) buildSwitchSubscriptionArgs() ([]string, bool) {
	name, ok := s.chooseSubscriptionName(label(s.language, "Switch Active Subscription", "切换当前订阅"))
	if !ok {
		return nil, false
	}
	return []string{"switch", "--name", name}, true
}

func (s *tuiSession) chooseSubscriptionName(title string) (string, bool) {
	p := paths.FromRoot("")
	manager := subscription.NewManager(p, loadConfigOrDefault(p.CustomizeFile))
	subs, err := manager.List()
	if err != nil {
		fmt.Fprintf(s.stderr, "list subscriptions: %v\n", err)
		return "", false
	}
	if len(subs) == 0 {
		fmt.Fprintln(s.stdout, "暂无订阅。")
		return "", false
	}
	active, _ := manager.Active()
	options, initial := subscriptionSwitchOptions(subs, active)
	idx, err := s.selectF(title, options, ui.SelectOpts{BackLabel: label(s.language, "Back", "返回"), Initial: initial})
	if err != nil {
		return "", false
	}
	return subs[idx].Name, true
}

func subscriptionSwitchOptions(subs []subscription.Subscription, active *subscription.Subscription) ([]string, int) {
	options := make([]string, len(subs))
	initial := 0
	for i, sub := range subs {
		options[i] = fmt.Sprintf("%s (%s, %d nodes)", sub.Name, sub.SourceType, sub.LastNodeCount)
		if active != nil && active.Name == sub.Name {
			options[i] = "当前 · " + options[i]
			initial = i
		}
	}
	return options, initial
}

func runTUIAddSubscription(s *tuiSession) bool {
	return commandAction(label(s.language, "Add Subscription", "添加订阅"), runTUIAddSubscriptionCommand)(s)
}

func runTUIAddSubscriptionCommand(s *tuiSession) int {
	args, ok := s.buildAddSubscriptionArgs()
	if !ok {
		fmt.Fprintln(s.stdout, "已取消。")
		return 0
	}
	fmt.Fprintln(s.stdout)
	return runSub(args, s.stdout, s.stderr)
}

func (s *tuiSession) buildAddSubscriptionArgs() ([]string, bool) {
	source, ok := s.promptDefaultOK(label(s.language, "Source type (clash, sing-box, base64, local-file)", "来源类型（clash、sing-box、base64、local-file）"), "clash")
	if !ok {
		return nil, false
	}
	if source == "local-file" || source == "local" || source == "file" {
		return s.buildLocalFileSubscriptionArgs(false)
	}
	return s.buildRemoteSubscriptionArgsWithSource(source)
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
	return s.buildRemoteSubscriptionArgsWithSource(source)
}

func (s *tuiSession) buildRemoteSubscriptionArgsWithSource(source string) ([]string, bool) {
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
	args, ok := s.buildLocalFileSubscriptionArgs(false)
	if !ok {
		fmt.Fprintln(s.stdout, "已取消。")
		return 0
	}
	fmt.Fprintln(s.stdout)
	return runSub(args, s.stdout, s.stderr)
}

func runTUIOverwriteCurrentFromLocalFile(s *tuiSession) bool {
	return commandAction(label(s.language, "Overwrite Current From Local File", "用本地文件覆盖当前配置"), func(s *tuiSession) int {
		args, ok := s.buildLocalFileSubscriptionArgs(true)
		if !ok {
			fmt.Fprintln(s.stdout, "已取消。")
			return 0
		}
		fmt.Fprintln(s.stdout)
		return runSub(args, s.stdout, s.stderr)
	})(s)
}

func (s *tuiSession) buildLocalFileSubscriptionArgs(overwrite bool) ([]string, bool) {
	name := "local"
	if overwrite {
		name = "local-overwrite"
	} else {
		var ok bool
		name, ok = s.promptDefaultOK("名称", "local")
		if !ok {
			return nil, false
		}
	}
	filePath, ok := s.promptRequired("配置文件路径")
	if !ok {
		return nil, false
	}
	command := "add"
	if overwrite {
		command = "overwrite-local"
	}
	args := []string{command, "--name", name, "--file", filePath}
	source, ok := s.promptDefaultOK("来源覆盖（clash、sing-box、base64，留空自动识别）", "")
	if !ok {
		return nil, false
	}
	if source != "" {
		args = append(args, "--source", source)
	}
	if !overwrite && !s.confirm("是否设为当前订阅？", true) {
		args = append(args, "--no-active")
	}
	return args, true
}
