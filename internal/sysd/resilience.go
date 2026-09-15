// 网络切换自愈：NetworkManager 钩子 + systemd watchdog 定时器。
//
// 解决 sing-box 在网卡晚启动/掉线/漫游时卡在 "missing default interface" 的软死：
//
//	A. NM dispatcher 钩子：真实网卡 up / 连通性变化时触发分层探测（防抖，忽略 tun），
//	   由 healthcheck 的持久冷却/熔断策略决定是否重启。
//	B. watchdog 定时器：周期探测，仅当「有上行但代理打不通」才重启。
//
// watchdog 探针由 sboxkit healthcheck 子命令提供，避免额外 shell/curl 运行时依赖。
package sysd

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/Trilives/sboxkit/internal/config"
	"github.com/Trilives/sboxkit/internal/execx"
	"github.com/Trilives/sboxkit/internal/i18n"
	"github.com/Trilives/sboxkit/internal/paths"
)

const (
	WatchdogName  = "sing-box-watchdog"
	dispatcherDir = "/etc/NetworkManager/dispatcher.d"
)

func dispatcherFile(name string) string {
	return filepath.Join(dispatcherDir, "90-"+name+"-restart")
}

func wdService() string { return "/etc/systemd/system/" + WatchdogName + ".service" }
func wdTimer() string   { return "/etc/systemd/system/" + WatchdogName + ".timer" }

func dispatcherText(name, tunDev string, debounce int, exe string) string {
	exe = shellQuote(exe)
	quotedName := shellQuote(name)
	quotedTunDev := shellQuote(tunDev)
	return fmt.Sprintf(`#!/usr/bin/env bash
# Auto-generated. Ask sboxkit healthcheck to classify a network change and
# apply its persistent restart limiter. Ignores the tun device; debounced.
interface="$1"
action="$2"
[ "${interface}" = %[2]s ] && exit 0
case "${action}" in
  up|connectivity-change|dhcp4-change|dhcp6-change) ;;
  *) exit 0 ;;
esac
systemctl is-active --quiet %[1]s.service || exit 0
stamp="/run/%[4]s-dispatcher.last"
now="$(date +%%s)"
if [ -f "${stamp}" ]; then
  last="$(cat "${stamp}" 2>/dev/null || echo 0)"
  [ "$(( now - last ))" -lt %[3]d ] && exit 0
fi
echo "${now}" > "${stamp}"
%[5]s healthcheck --event network-change --service %[1]s --tun-dev %[2]s
exit 0
`, quotedName, quotedTunDev, debounce, name, exe)
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}

func wdServiceText(name, tunDev, exe string) string {
	exe = strconv.Quote(strings.ReplaceAll(exe, "%", "%%"))
	return fmt.Sprintf(`[Unit]
Description=Probe %[1]s and restart it if it has soft-died (%[2]s)
After=%[1]s.service

[Service]
Type=oneshot
Environment=SERVICE_NAME=%[1]s
Environment=TUN_DEV=%[3]s
ExecStart=%[4]s healthcheck
`, name, WatchdogName, tunDev, exe)
}

func wdTimerText(interval string) string {
	return fmt.Sprintf(`[Unit]
Description=Run %[1]s.service every %[2]s

[Timer]
OnBootSec=2min
OnUnitActiveSec=%[2]s
Unit=%[1]s.service

[Install]
WantedBy=timers.target
`, WatchdogName, interval)
}

// ResilienceOptions InstallResilience 的选项；零值取默认。
type ResilienceOptions struct {
	Name     string // 主服务名，默认 sing-box
	Interval string // 探测间隔，默认 2min
	Debounce int    // NM 钩子防抖秒数，默认 20
	TunDev   string // tun 设备名，默认 singbox
}

var resilienceIntervalPattern = regexp.MustCompile(`^[1-9][0-9]*(s|min|h)$`)

func validateResilienceInterval(interval string) error {
	if !resilienceIntervalPattern.MatchString(interval) {
		return fmt.Errorf("%s", i18n.T("探测间隔必须是正整数加 s、min 或 h（例如 90s、2min）"))
	}
	return nil
}

func (o *ResilienceOptions) defaults() {
	if o.Name == "" {
		o.Name = DefaultName
	}
	if o.Interval == "" {
		o.Interval = "2min"
	}
	if o.Debounce == 0 {
		o.Debounce = 20
	}
	if o.TunDev == "" {
		o.TunDev = "singbox"
	}
}

// InstallResilience 安装 NM 钩子（如有 NetworkManager）与 watchdog 定时器。
func InstallResilience(opts ResilienceOptions) error {
	opts.defaults()
	if err := validateManagedServiceName(opts.Name); err != nil {
		return err
	}
	if err := validateInterfaceName(opts.TunDev); err != nil {
		return err
	}
	if err := validateResilienceInterval(opts.Interval); err != nil {
		return err
	}
	if !execx.Have("systemctl") {
		return fmt.Errorf("%s", i18n.T("未找到 systemctl，自愈需要 systemd"))
	}
	if err := execx.EnsureSudo(i18n.T("安装网络自愈")); err != nil {
		return err
	}
	if _, err := execx.RunRoot([]string{"mkdir", "-p", paths.RuntimeDir}, "", nil); err != nil {
		return err
	}
	exe, err := StableExecutablePath()
	if err != nil {
		return fmt.Errorf(i18n.T("定位 sboxkit 可执行文件: %w"), err)
	}

	if st, err := os.Stat(dispatcherDir); err == nil && st.IsDir() {
		if err := execx.WriteRoot(dispatcherFile(opts.Name), dispatcherText(opts.Name, opts.TunDev, opts.Debounce, exe), "0755", i18n.T("安装 NM 钩子")); err != nil {
			return err
		}
		execx.Ok(i18n.T("已装 NetworkManager 钩子：") + dispatcherFile(opts.Name))
	} else {
		execx.Warn(dispatcherDir + i18n.T(" 不存在，跳过 NM 钩子（watchdog 仍兜底）。"))
	}

	if err := execx.WriteRoot(wdService(), wdServiceText(opts.Name, opts.TunDev, exe), "0644", i18n.T("写 watchdog 单元")); err != nil {
		return err
	}
	if err := execx.WriteRoot(wdTimer(), wdTimerText(opts.Interval), "0644", i18n.T("写 watchdog 定时器")); err != nil {
		return err
	}
	if _, err := execx.RunRoot([]string{"systemctl", "daemon-reload"}, "", nil); err != nil {
		return err
	}
	if _, err := execx.RunRoot([]string{"systemctl", "enable", "--now", WatchdogName + ".timer"}, "", nil); err != nil {
		return err
	}
	execx.Ok(fmt.Sprintf(i18n.T("网络自愈已安装（探测间隔 %s）。"), opts.Interval))
	return nil
}

// RemoveResilience 卸载 NM 钩子与 watchdog。
func RemoveResilience(name string) error {
	if name == "" {
		name = DefaultName
	}
	if err := validateManagedServiceName(name); err != nil {
		return err
	}
	if err := execx.EnsureSudo(i18n.T("卸载网络自愈")); err != nil {
		return err
	}
	execx.RunRoot([]string{"rm", "-f", dispatcherFile(name)}, "", nil)
	quiet := &execx.Opt{Capture: true}
	for _, unit := range []string{WatchdogName + ".timer", WatchdogName + ".service"} {
		execx.RunRoot([]string{"systemctl", "stop", unit}, "", quiet)
		execx.RunRoot([]string{"systemctl", "disable", unit}, "", quiet)
	}
	execx.RunRoot([]string{"rm", "-f", wdTimer(), wdService()}, "", nil)
	execx.RunRoot([]string{"systemctl", "daemon-reload"}, "", nil)
	execx.Ok(i18n.T("网络自愈已卸载。"))
	return nil
}

func ResilienceInstalled() bool {
	_, err := os.Stat(wdTimer())
	return err == nil
}

// InspectResilience returns the complete three-component installation state.
// userDisabled comes from customize.json and suppresses NeedsRepair while still
// exposing any files left behind from an older installation.
func InspectResilience(name string, userDisabled bool) (ResilienceStatus, error) {
	opts := ResilienceOptions{Name: name, Interval: installedWatchdogInterval(wdTimer())}
	opts.defaults()
	if err := validateManagedServiceName(opts.Name); err != nil {
		return ResilienceStatus{}, err
	}
	exe, err := StableExecutablePath()
	if err != nil {
		return ResilienceStatus{}, fmt.Errorf(i18n.T("定位 sboxkit 可执行文件: %w"), err)
	}
	enabled := systemdUnitState(WatchdogName+".timer", "enabled")
	active := systemdUnitState(WatchdogName+".timer", "active")
	return inspectResilienceStatus(
		defaultResilienceLayout, opts.Name, opts.Interval, opts.TunDev, exe,
		enabled, userDisabled, active,
	), nil
}

// ReconcileResilience repairs missing, partial, stale, disabled, or inactive
// watchdog installations. It is intentionally a no-op for an explicitly
// disabled preference and when the primary service has not been installed.
func ReconcileResilience(p paths.Paths, name string) error {
	if name == "" {
		name = DefaultName
	}
	if err := validateManagedServiceName(name); err != nil {
		return err
	}
	cfg := config.Load(p)
	if !cfg.EnableResilience || !IsInstalled(name) {
		return nil
	}
	status, err := InspectResilience(name, false)
	if err != nil {
		return err
	}
	if !status.NeedsRuntimeRepair(IsActive(name)) {
		return nil
	}
	execx.Info(i18n.T("检测到网络自愈组件缺失或过期，正在修复…"))
	return InstallResilience(ResilienceOptions{Name: name, Interval: installedWatchdogInterval(wdTimer())})
}
