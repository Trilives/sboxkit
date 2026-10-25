// 用户模式的 TUN DNS：sing-box 以普通用户（带文件能力）运行，systemd-resolved 同样拒绝它为 TUN 链路
// 登记 DNS，用户单元又无法以 root 执行钩子。启用 TUN 时经 sudo 装一个系统单元：链路出现时 systemd
// 以 root 执行 resolvectl 完成登记。单元只执行 root 独占的 resolvectl、参数固定，不执行用户可写的
// sboxkit（见 service_exe.go）。
package sysd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Trilives/sboxkit/internal/execx"
	"github.com/Trilives/sboxkit/internal/i18n"
	"github.com/Trilives/sboxkit/internal/tundns"
)

const (
	userTunDNSPrefix = "sboxkit-tun-dns-"
	// minOneshotRestartSystemd Type=oneshot 自 systemd 244 起可用 Restart=on-failure，更早的版本拒绝加载。
	minOneshotRestartSystemd = 244
)

// userTunDNSUnit 一个待安装的系统单元。
type userTunDNSUnit struct {
	Name, Text, Dev string
}

// 可在测试中替换。
var (
	resolvedActive = tundns.ResolvedActive
	lookResolvectl = func() (string, error) { return exec.LookPath("resolvectl") }
)

func userTunDNSUnitName(uid int, dev string) string {
	return fmt.Sprintf("%s%d-%s.service", userTunDNSPrefix, uid, dev)
}

// netDeviceUnit 链路对应的 systemd 设备单元；只接受按单元名转义后不变的链路名（不含 - 且不以 . 开头）。
func netDeviceUnit(dev string) (string, bool) {
	if strings.Contains(dev, "-") || strings.HasPrefix(dev, ".") {
		return "", false
	}
	return "sys-subsystem-net-devices-" + dev + ".device", true
}

// userTunDNSUnitText 只在链路是 TUN 设备时登记（接口名可能出自原样信任的原生订阅，见
// tundns.tunLinkUp）。retry 时失败后重试：设备单元可能先于 systemd-resolved 认出新链路启动，
// resolvectl 报「Link N not known」；重试次数受 systemd 默认的启动频率限制。
func userTunDNSUnitText(uid int, link tunDNSLink, device, resolvectl string, retry bool) string {
	restart := ""
	if retry {
		restart = "Restart=on-failure\nRestartSec=1\n"
	}
	return fmt.Sprintf(`# sboxkit 用户模式（uid %[1]d）生成：普通用户的 sing-box 无权向 systemd-resolved 登记 TUN 的 DNS。
[Unit]
Description=Register system DNS for sing-box TUN link %[2]s (sboxkit user mode, uid %[1]d)
BindsTo=%[3]s
After=%[3]s systemd-resolved.service
ConditionPathExists=/sys/class/net/%[2]s/tun_flags

[Service]
Type=oneshot
%[6]sExecStart=%[4]s dns %[2]s %[5]s
ExecStart=%[4]s domain %[2]s ~.
ExecStart=%[4]s default-route %[2]s yes

[Install]
WantedBy=%[3]s
`, uid, link.Dev, device, systemdExecArg(resolvectl), strings.Join(link.Servers, " "), restart)
}

// planUserTunDNS 需要安装或更新的单元；current 返回已安装单元的内容与是否已启用。无法登记的链路
// 记入 warnings：用户模式没有 root 可以退回。
func planUserTunDNS(uid int, links []tunDNSLink, resolvectl string, retry bool, current func(name string) (string, bool)) (units []userTunDNSUnit, warnings []string) {
	for _, link := range links {
		device, ok := netDeviceUnit(link.Dev)
		if !ok {
			warnings = append(warnings, fmt.Sprintf(i18n.T("TUN 接口名 %s 含 - 或以 . 开头，用户模式无法为它登记系统 DNS"), link.Dev))
			continue
		}
		unit := userTunDNSUnit{Name: userTunDNSUnitName(uid, link.Dev), Dev: link.Dev,
			Text: userTunDNSUnitText(uid, link, device, resolvectl, retry)}
		if text, enabled := current(unit.Name); text != unit.Text || !enabled {
			units = append(units, unit)
		}
	}
	return units, warnings
}

// ensureUserTunDNS 用户模式启用 TUN 时按需安装上述系统单元，内容一致且已启用时不提权；配置中
// 已没有对应链路的单元（关闭 TUN 或改了接口名）一并移除。
func ensureUserTunDNS(rt runtimePaths) error {
	if !rt.Scope.User {
		return nil
	}
	links, err := runtimeTunDNSLinks(rt.Config)
	if err != nil {
		execx.Warn(fmt.Sprintf(i18n.T("无法为 TUN 登记系统 DNS：%v"), err))
		return nil
	}
	pruneUserTunDNS(links)
	if len(links) == 0 || !resolvedActive() {
		return nil
	}
	resolvectl, err := trustedResolvectl()
	if err != nil {
		execx.Warn(fmt.Sprintf(i18n.T("无法为 TUN 登记系统 DNS：%v"), err))
		return nil
	}
	retry := systemdVersion() >= minOneshotRestartSystemd
	units, warnings := planUserTunDNS(os.Getuid(), links, resolvectl, retry, installedSystemUnit)
	for _, warning := range warnings {
		execx.Warn(warning)
	}
	if len(units) == 0 {
		return nil
	}
	execx.Info(i18n.T("配置启用了 TUN：将通过 sudo 安装系统单元，在 TUN 链路出现时向 systemd-resolved 登记系统 DNS（用户模式的 sing-box 无权自行登记），之后重启无需再次授权。"))
	return installUserTunDNS(units)
}

func trustedResolvectl() (string, error) {
	path, err := lookResolvectl()
	if err != nil {
		return "", err
	}
	return path, checkExecutable(path)
}

func installedSystemUnit(name string) (string, bool) {
	raw, err := os.ReadFile(unitScope{}.unitFile(name))
	if err != nil {
		return "", false
	}
	return string(raw), systemdUnitState(unitScope{}, name, "enabled")
}

func installUserTunDNS(units []userTunDNSUnit) error {
	reason := i18n.T("安装 TUN 系统 DNS 登记单元")
	for _, unit := range units {
		if err := execx.WriteRoot(unitScope{}.unitFile(unit.Name), unit.Text, "0644", reason); err != nil {
			return err
		}
	}
	if _, err := execx.RunRoot([]string{"systemctl", "daemon-reload"}, reason, nil); err != nil {
		return err
	}
	for _, unit := range units {
		// --quiet：链路尚未出现时 systemctl 会警告依赖的设备单元不存在，属预期情况。
		if _, err := execx.RunRoot([]string{"systemctl", "enable", "--quiet", unit.Name}, reason, nil); err != nil {
			return err
		}
		// 链路已存在（sing-box 正在运行）时立即登记；之后每次链路出现都由 systemd 触发。
		if _, err := os.Stat(filepath.Join("/sys/class/net", unit.Dev)); err == nil {
			if _, err := execx.RunRoot([]string{"systemctl", "start", unit.Name}, reason, nil); err != nil {
				return err
			}
		}
	}
	return nil
}

// userTunDNSUnitFiles 本用户已安装的上述单元文件。链路名不含 -，uid 之后的 - 使通配符不会匹配
// 其他 uid 的单元。
func userTunDNSUnitFiles() ([]string, error) {
	return filepath.Glob(unitScope{}.unitFile(fmt.Sprintf("%s%d-*.service", userTunDNSPrefix, os.Getuid())))
}

// staleUserTunDNS installed 中配置已没有对应链路的单元。
func staleUserTunDNS(uid int, installed []string, links []tunDNSLink) []string {
	keep := map[string]bool{}
	for _, link := range links {
		keep[userTunDNSUnitName(uid, link.Dev)] = true
	}
	var stale []string
	for _, path := range installed {
		if !keep[filepath.Base(path)] {
			stale = append(stale, path)
		}
	}
	return stale
}

// pruneUserTunDNS 移除配置已不需要的单元，免得它日后为同名的其他链路登记 DNS。无法提权（无人
// 值守）时留到下次，失败只提示。
func pruneUserTunDNS(links []tunDNSLink) {
	installed, err := userTunDNSUnitFiles()
	if err != nil {
		return
	}
	stale := staleUserTunDNS(os.Getuid(), installed, links)
	if len(stale) == 0 || !canElevate() {
		return
	}
	if err := removeUserTunDNSFiles(stale, false); err != nil {
		execx.Warn(fmt.Sprintf(i18n.T("移除不再需要的 TUN 系统 DNS 登记单元失败：%v"), err))
	}
}

// removeUserTunDNS 用户模式卸载服务时移除本用户的上述单元；没有时不提权。
func removeUserTunDNS(quiet bool) error {
	names, err := userTunDNSUnitFiles()
	if err != nil {
		return err
	}
	return removeUserTunDNSFiles(names, quiet)
}

func removeUserTunDNSFiles(names []string, quiet bool) error {
	if len(names) == 0 {
		return nil
	}
	reason := i18n.T("移除 TUN 系统 DNS 登记单元")
	opt := &execx.Opt{Capture: quiet}
	var errs []error
	for _, path := range names {
		if _, err := execx.RunRoot([]string{"systemctl", "disable", filepath.Base(path)}, reason, opt); err != nil {
			errs = append(errs, err)
		}
		if _, err := execx.RunRoot([]string{"rm", "-f", path}, reason, nil); err != nil {
			errs = append(errs, err)
		}
	}
	if _, err := execx.RunRoot([]string{"systemctl", "daemon-reload"}, reason, opt); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}
