package app

func configTUIItems() []tuiItem {
	return configTUIItemsFor(languageEnglish)
}

func configTUIItemsFor(lang uiLanguage) []tuiItem {
	return []tuiItem{
		{label(lang, "Show Config", "显示配置"), label(lang, "Print the current customize.json", "打印当前 customize.json"), commandAction(label(lang, "Show Config", "显示配置"), func(s *tuiSession) int {
			return runConfig([]string{"show"}, s.stdout, s.stderr)
		})},
		{label(lang, "Edit Custom Layer", "编辑定制层"), label(lang, "Edit TUN, LAN, WebUI, download proxy, routing, and rule settings by field", "按字段编辑 TUN、局域网、WebUI、下载代理、分流和规则相关配置"), editConfigAction()},
		{label(lang, "Shell Proxy Environment", "Shell 代理环境"), label(lang, "Write or remove managed shell proxy variables", "写入或移除托管的 Shell 代理变量"), submenu(label(lang, "Shell Proxy Environment", "Shell 代理环境"), func() []tuiItem { return proxyEnvConfigTUIItemsFor(lang) })},
		{label(lang, "Advanced Key/Value", "高级键值"), label(lang, "Directly set any supported customize.json field", "直接设置任意支持的 customize.json 字段"), submenu(label(lang, "Advanced Key/Value", "高级键值"), func() []tuiItem { return advancedConfigTUIItemsFor(lang) })},
	}
}

func proxyEnvConfigTUIItems() []tuiItem {
	return proxyEnvConfigTUIItemsFor(languageEnglish)
}

func proxyEnvConfigTUIItemsFor(lang uiLanguage) []tuiItem {
	return []tuiItem{
		{label(lang, "Write Shell Proxy Environment", "写入 Shell 代理环境"), label(lang, "Append the managed proxy block to ~/.bashrc", "向 ~/.bashrc 追加托管的代理配置块"), commandAction(label(lang, "Write Shell Proxy Environment", "写入 Shell 代理环境"), func(s *tuiSession) int {
			return runProxyEnv([]string{"write"}, s.stdout, s.stderr)
		})},
		{label(lang, "Remove Shell Proxy Environment", "移除 Shell 代理环境"), label(lang, "Remove the managed proxy block from ~/.bashrc", "从 ~/.bashrc 移除托管的代理配置块"), commandAction(label(lang, "Remove Shell Proxy Environment", "移除 Shell 代理环境"), func(s *tuiSession) int {
			return runProxyEnv([]string{"remove"}, s.stdout, s.stderr)
		})},
	}
}

func advancedConfigTUIItems() []tuiItem {
	return advancedConfigTUIItemsFor(languageEnglish)
}

func advancedConfigTUIItemsFor(lang uiLanguage) []tuiItem {
	return []tuiItem{
		{label(lang, "Set Config Field", "设置配置项"), label(lang, "Set any supported config field by key/value", "通过键值直接设置任意支持的配置字段"), promptCommand(label(lang, "Set Config Field", "设置配置项"), func(s *tuiSession) ([]string, bool) {
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
