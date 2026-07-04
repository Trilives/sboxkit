// 每周自动更新定时器：周期性更新内核/geo 数据并同步重启服务。
// ExecStart 调用 sboxkit 自身的 `update` 子命令。
package sysd

import (
	"fmt"
	"os"

	"github.com/Trilives/sboxkit/internal/execx"
	"github.com/Trilives/sboxkit/internal/i18n"
)

const (
	TimerName         = "sing-box-update"
	DefaultOnCalendar = "Mon *-*-* 03:00:00"
	defaultDelay      = "30min"
)

func timerServiceFile() string { return "/etc/systemd/system/" + TimerName + ".service" }
func timerFile() string        { return "/etc/systemd/system/" + TimerName + ".timer" }

func timerServiceText(selfExe string) string {
	return fmt.Sprintf(`[Unit]
Description=Weekly sing-box core and rule-set update (%s)
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
ExecStart=%s update
`, TimerName, selfExe)
}

func timerText(onCalendar, delay string) string {
	return fmt.Sprintf(`[Unit]
Description=Run %[1]s.service weekly

[Timer]
OnCalendar=%[2]s
RandomizedDelaySec=%[3]s
Persistent=true
Unit=%[1]s.service

[Install]
WantedBy=timers.target
`, TimerName, onCalendar, delay)
}

// InstallTimer 安装每周更新定时器；onCalendar 为空取默认（周一 03:00 ± 30min）。
func InstallTimer(onCalendar string) error {
	if onCalendar == "" {
		onCalendar = DefaultOnCalendar
	}
	if !execx.Have("systemctl") {
		return fmt.Errorf("%s", i18n.T("未找到 systemctl，定时器需要 systemd"))
	}
	selfExe, err := os.Executable()
	if err != nil {
		return err
	}
	if err := execx.EnsureSudo(i18n.T("安装每周更新定时器")); err != nil {
		return err
	}
	if err := execx.WriteRoot(timerServiceFile(), timerServiceText(selfExe), "0644", i18n.T("写定时器服务")); err != nil {
		return err
	}
	if err := execx.WriteRoot(timerFile(), timerText(onCalendar, defaultDelay), "0644", i18n.T("写定时器")); err != nil {
		return err
	}
	if _, err := execx.RunRoot([]string{"systemctl", "daemon-reload"}, "", nil); err != nil {
		return err
	}
	if _, err := execx.RunRoot([]string{"systemctl", "enable", "--now", TimerName + ".timer"}, "", nil); err != nil {
		return err
	}
	execx.Ok(fmt.Sprintf(i18n.T("每周更新定时器已安装（%s）。"), onCalendar))
	return nil
}

// RemoveTimer 卸载每周更新定时器。
func RemoveTimer() error {
	if err := execx.EnsureSudo(i18n.T("卸载每周更新定时器")); err != nil {
		return err
	}
	quiet := &execx.Opt{Capture: true}
	for _, unit := range []string{TimerName + ".timer", TimerName + ".service"} {
		execx.RunRoot([]string{"systemctl", "stop", unit}, "", quiet)
		execx.RunRoot([]string{"systemctl", "disable", unit}, "", quiet)
	}
	execx.RunRoot([]string{"rm", "-f", timerFile(), timerServiceFile()}, "", nil)
	execx.RunRoot([]string{"systemctl", "daemon-reload"}, "", nil)
	execx.Ok(i18n.T("每周更新定时器已卸载。"))
	return nil
}

func TimerInstalled() bool {
	_, err := os.Stat(timerFile())
	return err == nil
}
