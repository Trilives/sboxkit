package app

import "fmt"

func serviceTUIItems() []tuiItem {
	return []tuiItem{
		{"Install and start service", "Sync runtime files, install systemd unit, and restart service", commandAction("Install service", func(s *tuiSession) int {
			if !s.confirmServiceTrafficRisk("install and start sboxkit.service") {
				fmt.Fprintln(s.stdout, "Cancelled.")
				return 0
			}
			return runService([]string{"install"}, s.stdout, s.stderr)
		})},
		{"Install without starting", "Install unit and runtime files but leave service stopped", commandAction("Install service without start", func(s *tuiSession) int {
			return runService([]string{"install", "--no-start"}, s.stdout, s.stderr)
		})},
		{"Sync and restart", "Copy active config/assets into /etc/sboxkit and restart", commandAction("Sync service", func(s *tuiSession) int {
			if !s.confirmServiceTrafficRisk("sync and restart sboxkit.service") {
				fmt.Fprintln(s.stdout, "Cancelled.")
				return 0
			}
			return runService([]string{"sync"}, s.stdout, s.stderr)
		})},
		{"Status", "Open systemctl status for sboxkit.service", commandAction("Service status", func(s *tuiSession) int {
			return runService([]string{"status"}, s.stdout, s.stderr)
		})},
		{"Remove service", "Stop service and remove systemd runtime files", commandAction("Remove service", func(s *tuiSession) int {
			if !s.confirm("Remove sboxkit.service and /etc/sboxkit?", false) {
				return 0
			}
			return runService([]string{"remove"}, s.stdout, s.stderr)
		})},
	}
}

func updateTUIItems() []tuiItem {
	return []tuiItem{
		{"Download optional rules through proxy", "Recommended after the service is running", commandAction("Update runtime assets", func(s *tuiSession) int {
			args := []string{"--proxy", s.promptDefault("Proxy URL", "http://127.0.0.1:7890")}
			if s.confirm("Sync assets to service and restart?", true) {
				if !s.confirmServiceTrafficRisk("sync assets and restart sboxkit.service") {
					fmt.Fprintln(s.stdout, "Service sync skipped.")
					return runUpdate(args, s.stdout, s.stderr)
				}
				args = append(args, "--sync-service")
			}
			return runUpdate(args, s.stdout, s.stderr)
		})},
		{"Download optional rules direct", "Fetch large rule-set assets without a proxy", commandAction("Update runtime assets", func(s *tuiSession) int {
			return runUpdate(nil, s.stdout, s.stderr)
		})},
		{"Update core cache and rules", "Download sing-box core into user state plus optional assets", commandAction("Update core and rules", func(s *tuiSession) int {
			args := []string{"--core"}
			if s.confirm("Force re-download?", false) {
				args = append(args, "--force")
			}
			return runUpdate(args, s.stdout, s.stderr)
		})},
	}
}

func nodeTUIItems() []tuiItem {
	return []tuiItem{
		{"List nodes", "Read selector groups from the running Clash API", commandAction("List nodes", func(s *tuiSession) int {
			return runNode([]string{"list"}, s.stdout, s.stderr)
		})},
		{"Switch node", "Switch a selector group without restarting sing-box", promptCommand("Switch node", func(s *tuiSession) ([]string, bool) {
			group := s.promptDefault("Group", "Proxy")
			name, ok := s.promptRequired("Node name")
			if !ok {
				return nil, false
			}
			return []string{"switch", "--group", group, "--name", name}, true
		}, runNode)},
	}
}

func timerTUIItems() []tuiItem {
	return []tuiItem{
		{"Install weekly update timer", "Install systemd timer for periodic updates", commandAction("Install timer", func(s *tuiSession) int {
			return runTimer([]string{"install", "--binary", "/usr/bin/sboxkit"}, s.stdout, s.stderr)
		})},
		{"Remove weekly update timer", "Remove the update timer", commandAction("Remove timer", func(s *tuiSession) int {
			return runTimer([]string{"remove"}, s.stdout, s.stderr)
		})},
		{"Install resilience watchdog", "Install network self-healing service/timer hooks", commandAction("Install resilience", func(s *tuiSession) int {
			return runResilience([]string{"install"}, s.stdout, s.stderr)
		})},
		{"Remove resilience watchdog", "Remove network self-healing integration", commandAction("Remove resilience", func(s *tuiSession) int {
			return runResilience([]string{"remove"}, s.stdout, s.stderr)
		})},
	}
}

func uninstallTUIItems() []tuiItem {
	return []tuiItem{
		{"Uninstall system integration", "Remove service, timer, resilience, and runtime files", commandAction("Uninstall", func(s *tuiSession) int {
			if !s.confirm("Uninstall system integration?", false) {
				return 0
			}
			return runUninstall(nil, s.stdout, s.stderr)
		})},
		{"Uninstall and purge user state", "Also remove subscriptions, generated configs, downloads, and UI state", commandAction("Uninstall and purge state", func(s *tuiSession) int {
			if !s.confirm("Purge all sboxkit user state?", false) {
				return 0
			}
			return runUninstall([]string{"--purge-state"}, s.stdout, s.stderr)
		})},
		{"Show apt package removal commands", "Explain how to remove the installed .deb package", commandAction("APT package removal", func(s *tuiSession) int {
			printPackageRemovalHint(s.stdout)
			return 0
		})},
	}
}
