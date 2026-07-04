package app

import (
	"fmt"
	"os"

	"github.com/Trilives/sboxkit/internal/paths"
)

func runTUIFirstSetup(s *tuiSession) bool {
	if !s.selectSetupLanguage() {
		return false
	}
	if !s.confirm(label(s.language, "Run first setup now?", "现在进行初始化吗？"), true) {
		fmt.Fprintln(s.stdout, label(s.language, "First setup skipped.", "已跳过初始化。"))
		return false
	}
	return commandAction(label(s.language, "First Setup", "首次初始化"), func(s *tuiSession) int {
		args := []string{}
		if !s.confirm(label(s.language, "Enable TUN mode?", "是否启用 TUN 模式？"), true) {
			args = append(args, "--no-tun")
			if s.confirm(label(s.language, "TUN is disabled. Write shell proxy variables to ~/.bashrc?", "TUN 已关闭，是否将 Shell 代理变量写入 ~/.bashrc？"), false) {
				args = append(args, "--write-proxy-env")
			} else {
				args = append(args, "--no-write-proxy-env")
			}
		}
		code := initState("", args, s.stdout, s.stderr)
		if code != 0 {
			return code
		}
		if s.confirm(label(s.language, "Import a subscription or local file now?", "现在导入订阅或本地文件吗？"), true) {
			if s.confirm(label(s.language, "Import from a URL?", "是否从链接导入？"), true) {
				code = runTUIAddRemoteSubscriptionCommand(s)
			} else {
				code = runTUIAddLocalConfigCommand(s)
			}
			if code != 0 {
				return code
			}
		}
		if s.confirmServiceRestart(label(s.language, "Install and start sboxkit.service now?", "现在安装并启动 sboxkit.service 吗？"), true) {
			code = s.runServiceF([]string{"install"}, s.stdout, s.stderr)
			if code != 0 {
				return code
			}
			s.runFirstSetupOptionalRuleUpdate()
		}
		return code
	})(s)
}

func (s *tuiSession) runFirstSetupOptionalRuleUpdate() {
	fmt.Fprintln(s.stdout, "\n服务已启动，正在通过本地代理下载可选规则集，然后重启服务。")
	code := s.runUpdateF(firstSetupPostStartUpdateArgs(), s.stdout, s.stderr)
	if code == 0 {
		return
	}
	fmt.Fprintf(s.stdout, "\n可选规则集下载失败（状态 %d）。初始化已完成，已启动的服务会保持运行；稍后可从“运行时资源”重新下载。\n", code)
}

func (s *tuiSession) selectSetupLanguage() bool {
	selected, ok := s.selectMenu("Language / 语言", []tuiItem{
		{Label: "English"},
		{Label: "中文"},
	}, int(s.languageIndex()))
	if !ok {
		fmt.Fprintln(s.stdout, label(s.language, "Cancelled.", "已取消。"))
		return false
	}
	next := languageEnglish
	if selected == 1 {
		next = languageChinese
	}
	if err := s.setLanguage(next); err != nil {
		fmt.Fprintf(s.stderr, "save language preference: %v\n", err)
		return false
	}
	return true
}

func defaultServiceIntegrationExists() bool {
	return serviceIntegrationExists(defaultServiceIntegrationMarkers())
}

func defaultServiceIntegrationMarkers() []string {
	p := paths.FromRoot("")
	return []string{
		"/etc/systemd/system/sboxkit.service",
		p.RuntimeLink,
	}
}

func serviceIntegrationExists(markers []string) bool {
	for _, marker := range markers {
		if _, err := os.Stat(marker); err == nil {
			return true
		}
	}
	return false
}

func firstSetupPostStartUpdateArgs() []string {
	return []string{"--proxy", "http://127.0.0.1:7890", "--sync-service"}
}
