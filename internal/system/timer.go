package system

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

const (
	UpdateTimerName    = "sboxkit-update"
	DefaultOnCalendar  = "Mon *-*-* 03:00:00"
	DefaultRandomDelay = "30min"
)

func RenderUpdateService(binary string) string {
	return fmt.Sprintf(`[Unit]
Description=Weekly sboxkit runtime asset update (%s)
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
ExecStart=%s update --sync-service
`, UpdateTimerName, binary)
}

func RenderUpdateTimer(onCalendar string, delay string) string {
	return fmt.Sprintf(`[Unit]
Description=Run %s.service weekly

[Timer]
OnCalendar=%s
RandomizedDelaySec=%s
Persistent=true
Unit=%s.service

[Install]
WantedBy=timers.target
`, UpdateTimerName, onCalendar, delay, UpdateTimerName)
}

func InstallUpdateTimer(ctx context.Context, runner Runner, stateDir string, binary string, onCalendar string, delay string) error {
	if onCalendar == "" {
		onCalendar = DefaultOnCalendar
	}
	if delay == "" {
		delay = DefaultRandomDelay
	}
	serviceTmp := filepath.Join(stateDir, UpdateTimerName+".service")
	timerTmp := filepath.Join(stateDir, UpdateTimerName+".timer")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(serviceTmp, []byte(RenderUpdateService(binary)), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(timerTmp, []byte(RenderUpdateTimer(onCalendar, delay)), 0o644); err != nil {
		return err
	}
	if err := runner.Run(ctx, "install", "-m", "0644", serviceTmp, "/etc/systemd/system/"+UpdateTimerName+".service"); err != nil {
		return err
	}
	if err := runner.Run(ctx, "install", "-m", "0644", timerTmp, "/etc/systemd/system/"+UpdateTimerName+".timer"); err != nil {
		return err
	}
	if err := runner.Run(ctx, "systemctl", "daemon-reload"); err != nil {
		return err
	}
	return runner.Run(ctx, "systemctl", "enable", "--now", UpdateTimerName+".timer")
}

func RemoveUpdateTimer(ctx context.Context, runner Runner) error {
	for _, unit := range []string{UpdateTimerName + ".timer", UpdateTimerName + ".service"} {
		_ = runner.Run(ctx, "systemctl", "stop", unit)
		_ = runner.Run(ctx, "systemctl", "disable", unit)
	}
	if err := runner.Run(ctx, "rm", "-f", "/etc/systemd/system/"+UpdateTimerName+".timer", "/etc/systemd/system/"+UpdateTimerName+".service"); err != nil {
		return err
	}
	_ = runner.Run(ctx, "systemctl", "daemon-reload")
	return nil
}
