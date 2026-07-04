// 「信息」工具：汇总当前生效配置里与"怎么连上代理/面板"相关的关键信息——
// 各协议共用的代理端口、局域网可达性、TUN 模式、面板地址与密钥状态。
package flows

import (
	"fmt"

	"github.com/Trilives/sboxkit/internal/config"
	"github.com/Trilives/sboxkit/internal/configfile"
	"github.com/Trilives/sboxkit/internal/execx"
	"github.com/Trilives/sboxkit/internal/i18n"
	"github.com/Trilives/sboxkit/internal/paths"
	"github.com/Trilives/sboxkit/internal/tui"
)

// InfoTool 显示当前生效配置的连接信息（「工具」菜单的一项）。
func InfoTool(p paths.Paths) error {
	execx.Header(i18n.T("信息"))
	rt, err := configfile.Read(p.ConfigFile)
	if err != nil {
		execx.Warn(i18n.T("尚未生成生效配置（先添加订阅并启动服务）。"))
		tui.Pause(i18n.T("回车返回主菜单… "))
		return nil
	}
	cfg := config.Load(p)

	fmt.Printf("  %-28s %v  (%s)\n",
		i18n.T("代理端口（HTTP + SOCKS5 共用）")+":", rt["mixed-port"],
		i18n.T("mixed-port，同一端口两种协议都能用"))
	fmt.Printf("  %-28s %s\n", i18n.T("局域网代理")+":", boolLabel(config.Bool(cfg, "lan_proxy")))
	fmt.Printf("  %-28s %s\n", i18n.T("TUN 模式")+":", boolLabel(tunEnabled(rt)))

	secret := fmt.Sprint(rt["secret"])
	secretLabel := i18n.T("未设置")
	if secret != "" && secret != "<nil>" {
		secretLabel = config.MaskSecret(secret)
	}
	fmt.Printf("  %-28s %s\n", i18n.T("面板密钥 secret")+":", secretLabel)
	fmt.Println()

	printAccessHint(p)
	fmt.Println()
	tui.Pause(i18n.T("回车返回主菜单… "))
	return nil
}

func tunEnabled(rt map[string]any) bool {
	tun, ok := rt["tun"].(map[string]any)
	if !ok {
		return false
	}
	on, _ := tun["enable"].(bool)
	return on
}

func boolLabel(b bool) string {
	if b {
		return i18n.T("开")
	}
	return i18n.T("关")
}
