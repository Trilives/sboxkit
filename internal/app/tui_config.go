package app

func configTUIItems() []tuiItem {
	return []tuiItem{
		{"显示配置", "打印当前 customize.json", commandAction("显示配置", func(s *tuiSession) int {
			return runConfig([]string{"show"}, s.stdout, s.stderr)
		})},
		{"编辑定制层", "按字段编辑 TUN、局域网、WebUI、下载代理、分流和规则相关配置", editConfigAction()},
		{"Shell 代理环境", "写入或移除托管的 Shell 代理变量", submenu("Shell 代理环境", proxyEnvConfigTUIItems)},
		{"高级键值", "直接设置任意支持的 customize.json 字段", submenu("高级键值", advancedConfigTUIItems)},
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
