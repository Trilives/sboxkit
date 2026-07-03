package app

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/Trilives/sboxkit/internal/paths"
)

func diagnosticsTUIItemsFor(lang uiLanguage) []tuiItem {
	return []tuiItem{
		{label(lang, "Network Test", "网络测试"), label(lang, "Test latency and exit IP through the local proxy", "通过本地代理测试延迟和出口 IP"), commandActionPaused(label(lang, "Network Test", "网络测试"), func(s *tuiSession) int {
			printNetworkTestProgress(s.stdout)
			runNettest(s.stdout, "127.0.0.1:7890")
			return 0
		})},
		{label(lang, "File Locations", "主要文件位置"), label(lang, "Show state, runtime, service, and packaged binary paths", "显示状态、运行时、服务和打包二进制路径"), commandActionPaused(label(lang, "File Locations", "主要文件位置"), func(s *tuiSession) int {
			printMainFileLocations(s.stdout)
			return 0
		})},
	}
}

func printMainFileLocations(stdout io.Writer) {
	p := paths.FromRoot("")
	rows := []struct {
		name string
		path string
	}{
		{"State root", p.StateDir},
		{"Generated config", p.ConfigFile},
		{"Custom layer", p.CustomizeFile},
		{"Active subscription", p.ActiveFile},
		{"Subscriptions", p.SubscriptionsDir},
		{"Downloads", p.DownloadsDir},
		{"Logs", p.LogDir},
		{"Rulesets", p.RulesetDir},
		{"Built-in WebUI", p.UIDir},
		{"User sing-box cache", p.SingBoxBin},
		{"System runtime", p.EtcDir},
		{"Runtime config", filepath.Join(p.EtcDir, "sboxkit.json")},
		{"Runtime sing-box", filepath.Join(p.EtcDir, "sing-box")},
		{"Packaged sing-box", p.SystemSingBoxBin},
		{"Systemd unit", "/etc/systemd/system/sboxkit.service"},
	}
	for _, row := range rows {
		fmt.Fprintf(stdout, "%-22s %s\n", row.name+":", row.path)
	}
}
