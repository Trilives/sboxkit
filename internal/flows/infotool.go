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
	"github.com/Trilives/sboxkit/internal/sysd"
	"github.com/Trilives/sboxkit/internal/tui"
)

// InfoTool 显示当前生效配置的连接信息（「工具」菜单的一项）。
func InfoTool(p paths.Paths) error {
	execx.Header(i18n.T("信息"))
	cfg := config.Load(p)
	printResilienceInfo(cfg)
	fmt.Println()
	rt, err := configfile.Read(p.ConfigFile)
	if err != nil {
		execx.Warn(i18n.T("尚未生成生效配置（先添加订阅并启动服务）。"))
		tui.Pause(i18n.T("回车返回主菜单… "))
		return nil
	}
	mixedPort, tun := runtimeInboundState(rt)
	if mixedPort == 0 {
		mixedPort = config.EffectiveMixedPort(cfg)
	}

	fmt.Printf("  %-28s %v  (%s)\n",
		i18n.T("代理端口（HTTP + SOCKS5 共用）")+":", mixedPort,
		i18n.T("mixed-port，同一端口两种协议都能用"))
	fmt.Printf("  %-28s %s\n", i18n.T("局域网代理")+":", boolLabel(config.Bool(cfg, "lan_proxy")))
	fmt.Printf("  %-28s %s\n", i18n.T("TUN 模式")+":", boolLabel(tun))

	secret := runtimeClashAPISecret(rt)
	secretLabel := i18n.T("未设置")
	if secret != "" {
		secretLabel = config.MaskSecret(secret)
	}
	fmt.Printf("  %-28s %s\n", i18n.T("面板密钥 secret")+":", secretLabel)
	fmt.Println()

	printAccessHint(p)
	fmt.Println()
	tui.Pause(i18n.T("回车返回主菜单… "))
	return nil
}

func printResilienceInfo(cfg config.Config) {
	status, err := sysd.InspectResilience(sysd.DefaultName, !cfg.EnableResilience)
	if err != nil {
		fmt.Printf("  %-28s %s\n", i18n.T("网络自愈")+":", i18n.T("状态检查失败")+": "+err.Error())
		return
	}
	dispatcher := componentInstallLabel(status.DispatcherInstalled, status.DispatcherStale)
	if !status.DispatcherSupported {
		dispatcher = i18n.T("不可用（未检测到 NetworkManager）")
	}
	fmt.Printf("  %-28s %s\n", i18n.T("NetworkManager dispatcher")+":", dispatcher)
	fmt.Printf("  %-28s %s\n", i18n.T("watchdog service")+":",
		componentInstallLabel(status.WatchdogServiceInstalled, status.WatchdogServiceStale))
	fmt.Printf("  %-28s %s / %s / %s\n", i18n.T("watchdog timer")+":",
		componentInstallLabel(status.WatchdogTimerInstalled, status.WatchdogTimerStale),
		enabledLabel(status.WatchdogTimerEnabled), activeLabel(status.WatchdogTimerActive))
	if status.UserDisabled {
		fmt.Printf("  %-28s %s\n", i18n.T("网络自愈偏好")+":", i18n.T("已明确禁用"))
	}
}

func componentInstallLabel(installed, stale bool) string {
	if !installed {
		return i18n.T("未安装")
	}
	if stale {
		return i18n.T("已安装（内容过期）")
	}
	return i18n.T("已安装")
}

func enabledLabel(enabled bool) string {
	if enabled {
		return i18n.T("已启用")
	}
	return i18n.T("未启用")
}

func activeLabel(active bool) string {
	if active {
		return i18n.T("运行中")
	}
	return i18n.T("未运行")
}

func boolLabel(b bool) string {
	if b {
		return i18n.T("开")
	}
	return i18n.T("关")
}
