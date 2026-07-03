package system

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

const WatchdogName = "sboxkit-watchdog"

func RenderDispatcher(debounce int, tunDev string) string {
	return fmt.Sprintf(`#!/usr/bin/env bash
interface="$1"
action="$2"
[ "${interface}" = "%s" ] && exit 0
case "${action}" in
  up|connectivity-change|dhcp4-change|dhcp6-change) ;;
  *) exit 0 ;;
esac
systemctl is-active --quiet "sboxkit.service" || exit 0
stamp="/run/sboxkit-dispatcher.last"
now="$(date +%%s)"
if [ -f "${stamp}" ]; then
  last="$(cat "${stamp}" 2>/dev/null || echo 0)"
  [ "$(( now - last ))" -lt %d ] && exit 0
fi
echo "${now}" > "${stamp}"
systemctl restart --no-block "sboxkit.service"
exit 0
`, tunDev, debounce)
}

func RenderHealthcheck() string {
	return `#!/usr/bin/env bash
set -uo pipefail

SERVICE_NAME="${SERVICE_NAME:-sboxkit.service}"
TUN_DEV="${TUN_DEV:-singbox}"
PROXY_ADDR="${PROXY_ADDR:-127.0.0.1:7890}"
PROBE_URL="${PROBE_URL:-http://connectivitycheck.gstatic.com/generate_204}"
PROBE_ATTEMPTS="${PROBE_ATTEMPTS:-3}"
PROBE_TIMEOUT="${PROBE_TIMEOUT:-8}"
PROBE_GAP="${PROBE_GAP:-4}"
MIN_UPTIME="${MIN_UPTIME:-90}"

have_uplink() {
  local dev
  while read -r dev; do
    [[ -n "${dev}" && "${dev}" != "${TUN_DEV}" ]] && return 0
  done < <(ip route show default 2>/dev/null | awk '{for (i=1;i<=NF;i++) if ($i=="dev") print $(i+1)}')
  return 1
}

proxy_works() {
  local i
  for ((i = 1; i <= PROBE_ATTEMPTS; i++)); do
    if curl -fsS -o /dev/null -m "${PROBE_TIMEOUT}" -x "http://${PROXY_ADDR}" "${PROBE_URL}"; then
      return 0
    fi
    [[ ${i} -lt ${PROBE_ATTEMPTS} ]] && sleep "${PROBE_GAP}"
  done
  return 1
}

service_uptime_seconds() {
  local enter enter_s now_s
  enter="$(systemctl show -p ActiveEnterTimestamp --value "${SERVICE_NAME}" 2>/dev/null)"
  [[ -z "${enter}" ]] && { echo 999999; return; }
  enter_s="$(date -d "${enter}" +%s 2>/dev/null || echo 0)"
  now_s="$(date +%s)"
  echo $(( now_s - enter_s ))
}

systemctl is-active --quiet "${SERVICE_NAME}" || exit 0
have_uplink || exit 0
proxy_works && exit 0
uptime="$(service_uptime_seconds)"
[[ "${uptime}" -lt "${MIN_UPTIME}" ]] && exit 0
systemctl restart "${SERVICE_NAME}"
`
}

func RenderWatchdogService(healthcheckPath string, tunDev string) string {
	return fmt.Sprintf(`[Unit]
Description=Probe sboxkit and restart it if proxying soft-dies (%s)
After=sboxkit.service

[Service]
Type=oneshot
Environment=SERVICE_NAME=sboxkit.service
Environment=TUN_DEV=%s
ExecStart=%s
`, WatchdogName, tunDev, healthcheckPath)
}

func RenderWatchdogTimer(interval string) string {
	return fmt.Sprintf(`[Unit]
Description=Run %s.service every %s

[Timer]
OnBootSec=2min
OnUnitActiveSec=%s
Unit=%s.service

[Install]
WantedBy=timers.target
`, WatchdogName, interval, interval, WatchdogName)
}

func InstallResilience(ctx context.Context, runner Runner, stateDir string, interval string, debounce int, tunDev string) error {
	if interval == "" {
		interval = "2min"
	}
	if debounce == 0 {
		debounce = 20
	}
	if tunDev == "" {
		tunDev = "singbox"
	}
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return err
	}
	healthTmp := filepath.Join(stateDir, "healthcheck.sh")
	dispTmp := filepath.Join(stateDir, "90-sboxkit-restart")
	svcTmp := filepath.Join(stateDir, WatchdogName+".service")
	timerTmp := filepath.Join(stateDir, WatchdogName+".timer")
	for path, text := range map[string]string{
		healthTmp: RenderHealthcheck(),
		dispTmp:   RenderDispatcher(debounce, tunDev),
		svcTmp:    RenderWatchdogService("/etc/sboxkit/healthcheck.sh", tunDev),
		timerTmp:  RenderWatchdogTimer(interval),
	} {
		if err := os.WriteFile(path, []byte(text), 0o755); err != nil {
			return err
		}
	}
	if err := runner.Run(ctx, "mkdir", "-p", "/etc/sboxkit"); err != nil {
		return err
	}
	if err := runner.Run(ctx, "install", "-m", "0755", healthTmp, "/etc/sboxkit/healthcheck.sh"); err != nil {
		return err
	}
	if err := runner.Run(ctx, "install", "-m", "0755", dispTmp, "/etc/NetworkManager/dispatcher.d/90-sboxkit-restart"); err != nil {
		return err
	}
	if err := runner.Run(ctx, "install", "-m", "0644", svcTmp, "/etc/systemd/system/"+WatchdogName+".service"); err != nil {
		return err
	}
	if err := runner.Run(ctx, "install", "-m", "0644", timerTmp, "/etc/systemd/system/"+WatchdogName+".timer"); err != nil {
		return err
	}
	if err := runner.Run(ctx, "systemctl", "daemon-reload"); err != nil {
		return err
	}
	return runner.Run(ctx, "systemctl", "enable", "--now", WatchdogName+".timer")
}

func RemoveResilience(ctx context.Context, runner Runner) error {
	for _, unit := range []string{WatchdogName + ".timer", WatchdogName + ".service"} {
		_ = runner.Run(ctx, "systemctl", "stop", unit)
		_ = runner.Run(ctx, "systemctl", "disable", unit)
	}
	if err := runner.Run(ctx, "rm", "-f", "/etc/NetworkManager/dispatcher.d/90-sboxkit-restart", "/etc/systemd/system/"+WatchdogName+".timer", "/etc/systemd/system/"+WatchdogName+".service"); err != nil {
		return err
	}
	_ = runner.Run(ctx, "systemctl", "daemon-reload")
	return nil
}
