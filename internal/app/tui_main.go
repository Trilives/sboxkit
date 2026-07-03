package app

import (
	"fmt"
	"io"
)

func mainTUIItems() []tuiItem {
	return []tuiItem{
		{"First setup wizard", "Initialize state, import a subscription, and optionally install the service", runTUIFirstSetup},
		{"Nodes", "List or switch selector nodes through the sing-box Clash API", submenu("Nodes", nodeTUIItems)},
		{"Subscriptions", "Add, list, switch, refresh, rebuild, or remove subscriptions and local configs", submenu("Subscriptions", subscriptionTUIItems)},
		{"Service", "Install, sync, inspect, or remove sboxkit.service", submenu("Service", serviceTUIItems)},
		{"Runtime assets", "Download optional rules or update the packaged core cache", submenu("Runtime assets", updateTUIItems)},
		{"Configuration", "Show or edit customize.json, TUN, WebUI, and shell proxy settings", submenu("Configuration", configTUIItems)},
		{"Network test", "Probe latency and exit IP through the local proxy", commandAction("Network test", func(s *tuiSession) int {
			printNetworkTestProgress(s.stdout)
			runNettest(s.stdout, "127.0.0.1:7890")
			return 0
		})},
		{"Timers and resilience", "Weekly update timer and network resilience watchdog", submenu("Timers and resilience", timerTUIItems)},
		{"Uninstall", "Remove system integration, with an optional state purge", submenu("Uninstall", uninstallTUIItems)},
		{"Help", "Print command-line help", commandAction("Help", func(s *tuiSession) int {
			printHelp(s.stdout)
			return 0
		})},
		{"Quit", "Exit sboxkit", func(*tuiSession) bool { return true }},
	}
}

func printNetworkTestProgress(stdout io.Writer) {
	fmt.Fprintln(stdout, "Testing network through 127.0.0.1:7890...")
}
