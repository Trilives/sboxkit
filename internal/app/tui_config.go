package app

func configTUIItems() []tuiItem {
	return []tuiItem{
		{"Show config", "Print current customize.json", commandAction("Show config", func(s *tuiSession) int {
			return runConfig([]string{"show"}, s.stdout, s.stderr)
		})},
		{"TUN and routing", "Enable or disable TUN and routing-related behavior", submenu("TUN and routing", tunConfigTUIItems)},
		{"WebUI and LAN", "Enable or disable the built-in WebUI and LAN-facing options", submenu("WebUI and LAN", webUIConfigTUIItems)},
		{"Shell proxy environment", "Write or remove managed shell proxy variables", submenu("Shell proxy environment", proxyEnvConfigTUIItems)},
		{"Advanced key/value", "Set any supported customize.json field directly", submenu("Advanced key/value", advancedConfigTUIItems)},
	}
}

func tunConfigTUIItems() []tuiItem {
	return []tuiItem{
		{"Enable TUN", "Set enable_tun=true", configSetAction("enable_tun", "true")},
		{"Disable TUN", "Set enable_tun=false and optionally write shell proxy env", commandAction("Disable TUN", func(s *tuiSession) int {
			code := runConfig([]string{"set", "--key", "enable_tun", "--value", "false"}, s.stdout, s.stderr)
			if code == 0 && s.confirm("Write shell proxy variables to ~/.bashrc?", false) {
				code = runProxyEnv([]string{"write"}, s.stdout, s.stderr)
			}
			return code
		})},
	}
}

func webUIConfigTUIItems() []tuiItem {
	return []tuiItem{
		{"Enable WebUI", "Set lan_panel=true; rebuild and sync after changing it", configSetAction("lan_panel", "true")},
		{"Disable WebUI", "Set lan_panel=false", configSetAction("lan_panel", "false")},
		{"Enable LAN proxy", "Set lan_proxy=true so proxy ports listen on LAN interfaces", configSetAction("lan_proxy", "true")},
		{"Disable LAN proxy", "Set lan_proxy=false so proxy ports stay local-only", configSetAction("lan_proxy", "false")},
	}
}

func proxyEnvConfigTUIItems() []tuiItem {
	return []tuiItem{
		{"Write shell proxy env", "Append managed proxy block to ~/.bashrc", commandAction("Write shell proxy env", func(s *tuiSession) int {
			return runProxyEnv([]string{"write"}, s.stdout, s.stderr)
		})},
		{"Remove shell proxy env", "Remove managed proxy block from ~/.bashrc", commandAction("Remove shell proxy env", func(s *tuiSession) int {
			return runProxyEnv([]string{"remove"}, s.stdout, s.stderr)
		})},
	}
}

func advancedConfigTUIItems() []tuiItem {
	return []tuiItem{
		{"Set config key", "Set any supported config field by key/value", promptCommand("Set config key", func(s *tuiSession) ([]string, bool) {
			key, ok := s.promptRequired("Key")
			if !ok {
				return nil, false
			}
			value, ok := s.promptRequired("Value")
			if !ok {
				return nil, false
			}
			return []string{"set", "--key", key, "--value", value}, true
		}, runConfig)},
	}
}
