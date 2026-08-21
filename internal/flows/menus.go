// 各系统组件的小型交互菜单（对应 resilience/timer/service 各自的 menu_flow / toggle_flow）。
package flows

import (
	"fmt"

	"github.com/Trilives/sboxkit/internal/config"
	"github.com/Trilives/sboxkit/internal/errs"
	"github.com/Trilives/sboxkit/internal/execx"
	"github.com/Trilives/sboxkit/internal/i18n"
	"github.com/Trilives/sboxkit/internal/paths"
	"github.com/Trilives/sboxkit/internal/subscription"
	"github.com/Trilives/sboxkit/internal/sysd"
	"github.com/Trilives/sboxkit/internal/tui"
)

func resilienceMenuFlow() error {
	p := paths.Detect()
	cfg := config.Load(p)
	resilienceStatus, statusErr := sysd.InspectResilience(sysd.DefaultName, !cfg.EnableResilience)
	complete := statusErr == nil && resilienceStatus.Complete()
	present := statusErr == nil && resilienceStatus.AnyInstalled()
	status := i18n.T("未安装")
	opts := []string{i18n.T("安装网络自愈")}
	if complete {
		status = i18n.T("已安装")
		opts = []string{i18n.T("调整探测间隔"), i18n.T("卸载网络自愈")}
	} else if present {
		status = i18n.T("安装不完整或已禁用")
		opts = []string{i18n.T("修复并启用网络自愈"), i18n.T("卸载网络自愈")}
	}
	idx, err := tui.Select(fmt.Sprintf(i18n.T("网络自愈设置（当前：%s）"), status), opts, tui.SelectOpts{})
	if err != nil {
		return nil
	}
	switch {
	case !complete && (!present || idx == 0):
		return applyResiliencePreference(p, true, func() error {
			return sysd.InstallResilience(sysd.ResilienceOptions{})
		})
	case complete && idx == 0:
		interval, err := tui.Ask(i18n.T("探测间隔（如 2min / 90s）"), tui.AskOpts{Default: "2min"})
		if err != nil {
			return nil
		}
		return sysd.InstallResilience(sysd.ResilienceOptions{Interval: interval})
	default:
		return applyResiliencePreference(p, false, func() error {
			return sysd.RemoveResilience(sysd.DefaultName)
		})
	}
}

func timerMenuFlow() error {
	installed := sysd.TimerInstalled()
	status := i18n.T("未安装")
	opts := []string{i18n.T("安装每周更新定时器")}
	if installed {
		status = i18n.T("已安装")
		opts = []string{i18n.T("改时间表"), i18n.T("卸载定时器")}
	}
	idx, err := tui.Select(fmt.Sprintf(i18n.T("每周更新定时器（当前：%s）"), status), opts, tui.SelectOpts{})
	if err != nil {
		return nil
	}
	switch {
	case !installed:
		return sysd.InstallTimer("")
	case idx == 0:
		cal, err := tui.Ask(i18n.T("OnCalendar 表达式"), tui.AskOpts{Default: sysd.DefaultOnCalendar})
		if err != nil {
			return nil
		}
		return sysd.InstallTimer(cal)
	default:
		return sysd.RemoveTimer()
	}
}

// ServiceToggleFlow 主菜单『暂停 / 启动服务』统一入口。
func ServiceToggleFlow(p paths.Paths) error {
	if !sysd.IsInstalled(sysd.DefaultName) {
		execx.Warn(i18n.T("服务尚未安装，请先执行『初始化（首次部署）』。"))
		return nil
	}
	active := sysd.IsActive(sysd.DefaultName)
	execx.Header(i18n.T("暂停 / 启动服务"))
	state := i18n.T("已停止")
	if active {
		state = i18n.T("运行中")
	}
	fmt.Printf(i18n.T("  主服务 %s.service：%s\n"), sysd.DefaultName, state)
	for _, unit := range sysd.CompanionUnits() {
		fmt.Printf(i18n.T("  伴生单元 %s：状态见 systemctl\n"), unit)
	}
	action := i18n.T("启动")
	if active {
		action = i18n.T("暂停")
	}
	ok, err := tui.Confirm(fmt.Sprintf(i18n.T("确认%s全部服务？"), action), true)
	if err != nil || !ok {
		return nil
	}
	if active {
		return sysd.Pause(sysd.DefaultName)
	}
	return sysd.Resume(sysd.DefaultName)
}

func serviceSettings(p paths.Paths) error {
	act, err := tui.Select(i18n.T("服务设置"), []string{
		i18n.T("查看状态"), i18n.T("重启服务"), i18n.T("重载服务（重建订阅与服务）"),
	}, tui.SelectOpts{})
	if err != nil {
		return nil
	}
	switch act {
	case 0:
		sysd.Status(sysd.DefaultName)
	case 1:
		execx.RunRoot([]string{"systemctl", "restart", sysd.DefaultName + ".service"}, i18n.T("重启服务"), nil)
	default:
		return reloadService(p)
	}
	return nil
}

// reloadService first regenerates the active config while the old service is
// still intact, then Install validates the staged runtime before replacing the
// unit. A conversion/check failure therefore does not leave a half-installed
// service behind.
func reloadService(p paths.Paths) error {
	active := subscription.GetActive(p)
	if active == nil {
		return fmt.Errorf("%s", i18n.T("没有生效订阅，无法重载服务"))
	}
	execx.Info(i18n.T("正在从订阅原文重新生成配置…"))
	if _, err := subscription.Rebuild(p, active.Name); err != nil {
		return fmt.Errorf(i18n.T("重新生成订阅失败：%w"), err)
	}
	execx.Info(i18n.T("正在校验运行时并重新安装服务…"))
	if err := sysd.Install(p, sysd.DefaultName, true); err != nil {
		return fmt.Errorf(i18n.T("重新安装服务失败：%w"), err)
	}
	return nil
}

func reinitializeFlow(p paths.Paths) error {
	ok, err := tui.Confirm(i18n.T("确认移除现有服务并重新初始化？订阅原文和定制配置会保留。"), false)
	if err != nil || !ok {
		return err
	}
	if sysd.IsInstalled(sysd.DefaultName) {
		if err := sysd.Remove(p, sysd.DefaultName, true); err != nil {
			return err
		}
	}
	if err := Init(p); err != nil {
		return err
	}
	return errs.ErrSaveExit
}

// printAccessHint 初始化完成后的访问方式提示。
func printAccessHint(p paths.Paths) {
	cfg := config.Load(p)
	lanPanel := config.Bool(cfg, "lan_panel")
	host := "127.0.0.1"
	if lanPanel {
		host = "0.0.0.0"
	}
	execx.Info(fmt.Sprintf(i18n.T("Web UI（sing-box 内置路径）: http://%s:9090/ui/"), host))
	if host == "127.0.0.1" {
		execx.Info(i18n.T("远程查看建议用 SSH 端口转发： ssh -N -L 9090:127.0.0.1:9090 user@server"))
	}
	if config.Bool(cfg, "lan_proxy") {
		execx.Info(fmt.Sprintf(i18n.T("局域网代理已开启：其他主机可设置 http/socks 代理为 本机IP:%d"), config.EffectiveMixedPort(cfg)))
	}
}
