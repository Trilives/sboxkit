package app

func configTUIItems() []tuiItem {
	return []tuiItem{
		{"显示配置", "打印当前 customize.json", commandAction("显示配置", func(s *tuiSession) int {
			return runConfig([]string{"show"}, s.stdout, s.stderr)
		})},
		{"TUN 与路由", "启用或关闭 TUN 及路由相关行为", submenu("TUN 与路由", tunConfigTUIItems)},
		{"WebUI 与局域网", "启用或关闭内置 WebUI 和局域网相关选项", submenu("WebUI 与局域网", webUIConfigTUIItems)},
		{"Shell 代理环境", "写入或移除托管的 Shell 代理变量", submenu("Shell 代理环境", proxyEnvConfigTUIItems)},
		{"高级键值", "直接设置任意支持的 customize.json 字段", submenu("高级键值", advancedConfigTUIItems)},
	}
}

func tunConfigTUIItems() []tuiItem {
	return []tuiItem{
		{"启用 TUN", "将 enable_tun 设为 true", configSetAction("enable_tun", "true")},
		{"关闭 TUN", "将 enable_tun 设为 false，并可选写入 Shell 代理环境", commandAction("关闭 TUN", func(s *tuiSession) int {
			code := runConfig([]string{"set", "--key", "enable_tun", "--value", "false"}, s.stdout, s.stderr)
			if code == 0 && s.confirm("是否将 Shell 代理变量写入 ~/.bashrc？", false) {
				code = runProxyEnv([]string{"write"}, s.stdout, s.stderr)
			}
			return code
		})},
	}
}

func webUIConfigTUIItems() []tuiItem {
	return []tuiItem{
		{"启用 WebUI", "将 lan_panel 设为 true；修改后会重建并同步", configSetAction("lan_panel", "true")},
		{"关闭 WebUI", "将 lan_panel 设为 false", configSetAction("lan_panel", "false")},
		{"启用局域网代理", "将 lan_proxy 设为 true，让代理端口监听局域网接口", configSetAction("lan_proxy", "true")},
		{"关闭局域网代理", "将 lan_proxy 设为 false，让代理端口仅本地可用", configSetAction("lan_proxy", "false")},
	}
}

func proxyEnvConfigTUIItems() []tuiItem {
	return []tuiItem{
		{"写入 Shell 代理环境", "向 ~/.bashrc 追加托管的代理配置块", commandAction("写入 Shell 代理环境", func(s *tuiSession) int {
			return runProxyEnv([]string{"write"}, s.stdout, s.stderr)
		})},
		{"移除 Shell 代理环境", "从 ~/.bashrc 移除托管的代理配置块", commandAction("移除 Shell 代理环境", func(s *tuiSession) int {
			return runProxyEnv([]string{"remove"}, s.stdout, s.stderr)
		})},
	}
}

func advancedConfigTUIItems() []tuiItem {
	return []tuiItem{
		{"设置配置项", "通过键值直接设置任意支持的配置字段", promptCommand("设置配置项", func(s *tuiSession) ([]string, bool) {
			key, ok := s.promptRequired("键")
			if !ok {
				return nil, false
			}
			value, ok := s.promptRequired("值")
			if !ok {
				return nil, false
			}
			return []string{"set", "--key", key, "--value", value}, true
		}, runConfig)},
	}
}
