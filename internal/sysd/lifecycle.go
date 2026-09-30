// 主服务与伴生单元的运行期生命周期：状态查询、重启、暂停/启动。
// 系统级与用户级（用户模式）共用同一套逻辑，差异全部由 unitScope 承载。
package sysd

import (
	"fmt"
	"strings"

	"github.com/Trilives/sboxkit/internal/execx"
	"github.com/Trilives/sboxkit/internal/i18n"
	"github.com/Trilives/sboxkit/internal/paths"
)

func IsActive(name string) bool {
	if name == "" {
		name = DefaultName
	}
	return unitActive(currentScope(), name+".service")
}

func unitActive(s unitScope, unit string) bool {
	r, _ := execx.Run(s.systemctl("is-active", unit), &execx.Opt{Capture: true})
	return strings.TrimSpace(r.Stdout) == "active"
}

func Status(name string) {
	if name == "" {
		name = DefaultName
	}
	execx.Run(currentScope().systemctl("status", "--no-pager", name+".service"), nil)
}

// Restart 重启主服务（系统级经 sudo，用户级直接 systemctl --user）。
func Restart(name string) error {
	if name == "" {
		name = DefaultName
	}
	if err := validateManagedServiceName(name); err != nil {
		return err
	}
	s := currentScope()
	if err := s.ensurePrivilege(i18n.T("重启服务")); err != nil {
		return err
	}
	_, err := s.run(s.systemctl("restart", name+".service"), i18n.T("重启服务"), nil)
	return err
}

// CompanionUnits 已安装的伴生 systemd 单元（暂停/启动须一并带上，
// 否则 watchdog 会把刚停掉的主服务又拉起来）。
func CompanionUnits() []string {
	return companionUnits(DefaultName)
}

func companionUnits(name string) []string {
	var units []string
	if ResilienceInstalled() {
		units = append(units, WatchdogName+".timer")
	}
	if TimerInstalled() {
		units = append(units, TimerName+".timer")
	}
	// Keep the relay last: Pause must stop watchdog/update triggers before
	// stopping a required dependency, which also stops the main service.
	if BootstrapDNSInstalled(name) {
		units = append(units, bootstrapServiceName(name)+".service")
	}
	return units
}

func resumeCompanionUnits(units []string) []string {
	resumed := make([]string, 0, len(units))
	for _, unit := range units {
		if unit != WatchdogName+".timer" {
			resumed = append(resumed, unit)
		}
	}
	return resumed
}

// Pause 暂停主服务及全部伴生单元（运行时停止；单元保持开机自启）。
func Pause(name string) error {
	if name == "" {
		name = DefaultName
	}
	if err := validateManagedServiceName(name); err != nil {
		return err
	}
	if !IsInstalled(name) {
		execx.Warn(i18n.T("服务 ") + name + i18n.T(" 未安装，无需暂停。"))
		return nil
	}
	s := currentScope()
	companions := companionUnits(name)
	if err := s.ensurePrivilege(i18n.T("暂停服务")); err != nil {
		return err
	}
	// 先停伴生 watchdog，否则刚停掉主服务它又会拉起来
	for _, unit := range companions {
		s.run(s.systemctl("stop", unit), "", &execx.Opt{Capture: true})
	}
	s.run(s.systemctl("stop", name+".service"), "", nil)
	suffix := ""
	if len(companions) > 0 {
		suffix = fmt.Sprintf(i18n.T(" + %d 个伴生单元"), len(companions))
	}
	execx.Ok(i18n.T("已暂停：") + name + ".service" + suffix)
	if s.User {
		execx.Info(i18n.T("提示：暂停为运行时停止，下次开机（已开启 linger）或重新登录后会自动恢复运行。"))
	} else {
		execx.Info(i18n.T("提示：暂停为运行时停止，重启系统后会自动恢复运行。"))
	}
	return nil
}

// Resume 启动主服务及符合当前配置的伴生单元。watchdog 交给
// ReconcileResilience 校验完整性和用户偏好，避免启动历史残留单元。
func Resume(name string) error {
	if name == "" {
		name = DefaultName
	}
	if err := validateManagedServiceName(name); err != nil {
		return err
	}
	if !IsInstalled(name) {
		execx.Warn(i18n.T("服务 ") + name + i18n.T(" 未安装，请先执行『初始化』。"))
		return nil
	}
	s := currentScope()
	companions := resumeCompanionUnits(companionUnits(name))
	if err := s.ensurePrivilege(i18n.T("启动服务")); err != nil {
		return err
	}
	s.run(s.systemctl("start", name+".service"), "", nil)
	if err := ReconcileResilience(paths.Detect(), name); err != nil {
		execx.Warn(fmt.Sprintf(i18n.T("修复网络自愈失败（主服务不受影响）：%v"), err))
	}
	for _, unit := range companions {
		s.run(s.systemctl("start", unit), "", &execx.Opt{Capture: true})
	}
	suffix := ""
	if len(companions) > 0 {
		suffix = fmt.Sprintf(i18n.T(" + %d 个伴生单元"), len(companions))
	}
	execx.Ok(i18n.T("已启动：") + name + ".service" + suffix)
	return nil
}
