package app

import "fmt"

func subscriptionTUIItems() []tuiItem {
	return []tuiItem{
		{"List subscriptions", "Show saved subscriptions and node counts", commandAction("List subscriptions", func(s *tuiSession) int {
			return runSub([]string{"list"}, s.stdout, s.stderr)
		})},
		{"Add remote URL", "Import Clash, sing-box, or base64 subscription URL", runTUIAddRemoteSubscription},
		{"Add local config file", "Copy a config.yaml/json into the fixed state directory", runTUIAddLocalConfig},
		{"Switch active subscription", "Select which saved subscription feeds the running config", promptCommand("Switch active subscription", func(s *tuiSession) ([]string, bool) {
			name, ok := s.promptRequired("Subscription name")
			if !ok {
				return nil, false
			}
			return []string{"switch", "--name", name}, true
		}, runSub)},
		{"Refresh subscription", "Fetch the latest remote content and rebuild config", promptCommand("Refresh subscription", func(s *tuiSession) ([]string, bool) {
			name, ok := s.promptRequired("Subscription name")
			if !ok {
				return nil, false
			}
			args := []string{"refresh", "--name", name}
			if proxy := s.promptDefault("Download proxy", ""); proxy != "" {
				args = append(args, "--proxy", proxy)
			}
			return args, true
		}, runSub)},
		{"Rebuild active config", "Rebuild from stored local/raw source without fetching", promptCommand("Rebuild subscription", func(s *tuiSession) ([]string, bool) {
			name, ok := s.promptRequired("Subscription name")
			if !ok {
				return nil, false
			}
			return []string{"rebuild", "--name", name}, true
		}, runSub)},
		{"Remove subscription", "Delete a saved subscription", promptCommand("Remove subscription", func(s *tuiSession) ([]string, bool) {
			name, ok := s.promptRequired("Subscription name")
			if !ok || !s.confirm("Remove subscription "+name+"?", false) {
				return nil, false
			}
			return []string{"remove", "--name", name}, true
		}, runSub)},
	}
}

func runTUIAddRemoteSubscription(s *tuiSession) bool {
	return commandAction("Add remote subscription", runTUIAddRemoteSubscriptionCommand)(s)
}

func runTUIAddRemoteSubscriptionCommand(s *tuiSession) int {
	args, ok := s.buildRemoteSubscriptionArgs()
	if !ok {
		fmt.Fprintln(s.stdout, "Cancelled.")
		return 0
	}
	fmt.Fprintln(s.stdout)
	return runSub(args, s.stdout, s.stderr)
}

func (s *tuiSession) buildRemoteSubscriptionArgs() ([]string, bool) {
	source := s.promptDefault("Source type: clash, sing-box, or base64", "clash")
	name := s.promptDefault("Name", "main")
	url, ok := s.promptRequired("Subscription URL")
	if !ok {
		return nil, false
	}
	args := []string{"add", "--name", name, "--source", source, "--url", url}
	if proxy := s.promptDefault("Download proxy (empty by default)", ""); proxy != "" {
		args = append(args, "--proxy", proxy)
	}
	if !s.confirm("Set as active subscription?", true) {
		args = append(args, "--no-active")
	}
	return args, true
}

func runTUIAddLocalConfig(s *tuiSession) bool {
	return commandAction("Add local config file", runTUIAddLocalConfigCommand)(s)
}

func runTUIAddLocalConfigCommand(s *tuiSession) int {
	name := s.promptDefault("Name", "local")
	filePath, ok := s.promptRequired("Config file path")
	if !ok {
		fmt.Fprintln(s.stdout, "Cancelled.")
		return 0
	}
	args := []string{"add", "--name", name, "--file", filePath}
	if source := s.promptDefault("Source override: clash, sing-box, base64, or blank for auto", ""); source != "" {
		args = append(args, "--source", source)
	}
	if s.confirm("Use sing-box config as passthrough?", false) {
		args = append(args, "--passthrough")
	}
	if !s.confirm("Set as active subscription?", true) {
		args = append(args, "--no-active")
	}
	fmt.Fprintln(s.stdout)
	return runSub(args, s.stdout, s.stderr)
}
