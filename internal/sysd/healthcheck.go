package sysd

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Trilives/sboxkit/internal/config"
	"github.com/Trilives/sboxkit/internal/execx"
	"github.com/Trilives/sboxkit/internal/paths"
)

const (
	defaultProbeURL      = "http://connectivitycheck.gstatic.com/generate_204"
	defaultProbeAttempts = 3
	defaultProbeTimeout  = 8 * time.Second
	defaultProbeGap      = 4 * time.Second
	defaultMinUptime     = 90 * time.Second
)

var defaultProxyAddr = "127.0.0.1:" + strconv.Itoa(config.DefaultMixedPort)

// RunHealthcheck is the watchdog probe used by sing-box-watchdog.service.
// It restarts sing-box only when the host has a real uplink but the local
// proxy is unresponsive for several attempts.
func RunHealthcheck(args []string) int {
	fs := flag.NewFlagSet("healthcheck", flag.ContinueOnError)
	service := fs.String("service", envOr("SERVICE_NAME", DefaultName), "systemd service name")
	tunDev := fs.String("tun-dev", envOr("TUN_DEV", "singbox"), "TUN interface to ignore")
	event := fs.String("event", "timer", "probe trigger (timer or network-change)")
	proxyAddr := fs.String("proxy", envOr("PROXY_ADDR", ""), "HTTP proxy address")
	probeURL := fs.String("url", envOr("PROBE_URL", defaultProbeURL), "probe URL")
	attempts := fs.Int("attempts", envInt("PROBE_ATTEMPTS", defaultProbeAttempts), "probe attempts")
	timeout := fs.Duration("timeout", envDurationSeconds("PROBE_TIMEOUT", defaultProbeTimeout), "per-attempt timeout")
	gap := fs.Duration("gap", envDurationSeconds("PROBE_GAP", defaultProbeGap), "gap between attempts")
	minUptime := fs.Duration("min-uptime", envDurationSeconds("MIN_UPTIME", defaultMinUptime), "minimum service uptime before restart")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if err := validateHealthcheckInputs(*service, *tunDev, *event, *probeURL, *attempts, *timeout, *gap, *minUptime); err != nil {
		execx.Error(err.Error())
		return 2
	}
	release, err := acquireHealthcheckRunLock()
	if err != nil {
		if errors.Is(err, errHealthcheckAlreadyRunning) {
			logHealthcheck("classification=concurrent_probe event=%s action=skip reason=already_running", *event)
			return 0
		}
		execx.Error("healthcheck lock: " + err.Error())
		return 1
	}
	defer release()

	if !systemctlQuiet("is-active", *service) {
		logHealthcheck("classification=service_inactive service=%s event=%s action=skip", *service, *event)
		return 0
	}
	ensureTunDNS(*service)

	addr := strings.TrimSpace(*proxyAddr)
	if addr == "" {
		addr = runtimeProxyAddr(*service)
	}
	if addr == "" {
		addr = defaultProxyAddr
	}
	if err := validateLoopbackProxyAddress(addr); err != nil {
		execx.Error(err.Error())
		return 2
	}
	probes, runtime := productionHealthProbes(
		*service, *tunDev, addr, *probeURL, *attempts, *timeout, *gap,
	)
	report := annotateBootstrapDegraded(RunLayeredProbes(probes), runtime.bootstrapDegraded)

	restartAllowed := report.Healthy || serviceUptime(*service) >= *minUptime
	decision, err := evaluateWatchdogFile(
		DefaultWatchdogStatePath(), time.Now().UTC(), report, DefaultWatchdogPolicy(), restartAllowed,
	)
	if err != nil {
		execx.Error("watchdog state: " + err.Error())
		return 1
	}
	// Full per-layer details are emitted only at incident boundaries or before
	// an action. Periodic healthy/backoff rounds retain a single summary line,
	// avoiding both log floods and repeated sensitive failure strings.
	if decision.CaptureSnapshot || decision.Restart || decision.Recovered {
		logHealthReport(report, *event)
	}

	if decision.Recovered {
		if err := RemoveDiagnosticSnapshot(DefaultDiagnosticSnapshotPath()); err != nil {
			logHealthcheck("classification=healthy action=retain_snapshot error=%q", err.Error())
			return 0
		}
		if err := completeWatchdogRecovery(DefaultWatchdogStatePath()); err != nil {
			execx.Error("complete watchdog recovery: " + err.Error())
			return 1
		}
		logHealthcheck("classification=healthy action=recovered healthy_rounds=2")
		return 0
	}
	if decision.CaptureSnapshot {
		if decision.ReplaceSnapshot {
			if err := RemoveDiagnosticSnapshot(DefaultDiagnosticSnapshotPath()); err != nil {
				logHealthcheck("classification=%s action=discard_stale_snapshot_failed error=%q", report.Failure, err.Error())
				decision.CaptureSnapshot = false
			}
		}
	}
	if decision.CaptureSnapshot {
		cfg := config.NormalizeBootstrapDNS(config.Load(paths.Detect()))
		snapshot := collectDiagnosticSnapshot(
			report, runtime.interfaceName, *tunDev, *service,
			bootstrapTargetLabels(cfg.BootstrapDNSUpstreams),
		)
		created, snapshotErr := WriteDiagnosticSnapshotOnce(
			DefaultDiagnosticSnapshotPath(), snapshot, defaultDiagnosticMaxBytes,
		)
		if snapshotErr != nil {
			logHealthcheck("classification=%s action=snapshot_failed error=%q", report.Failure, snapshotErr.Error())
		} else {
			if stateErr := setWatchdogSnapshotCaptured(DefaultWatchdogStatePath(), true); stateErr != nil {
				logHealthcheck("classification=%s action=snapshot_state_failed error=%q", report.Failure, stateErr.Error())
			}
			if created {
				logHealthcheck("classification=%s action=snapshot_created path=%s", report.Failure, DefaultDiagnosticSnapshotPath())
			}
		}
	}
	if report.Healthy {
		return 0
	}
	if !decision.Restart {
		logHealthcheck("classification=%s action=skip_restart reason=%s", report.Failure, decision.Reason)
		return 0
	}

	logHealthcheck("classification=%s action=restart service=%s attempts=%d", report.Failure, *service, len(decision.State.RestartedAt))
	if _, err := execx.Run(currentScope().systemctl("restart", *service), &execx.Opt{Capture: true}); err != nil {
		execx.Error(err.Error())
		return 1
	}
	return 0
}

func validateHealthcheckInputs(
	service, tunDev, event, probeURL string,
	attempts int,
	timeout, gap, minUptime time.Duration,
) error {
	if err := validateServiceName(service); err != nil {
		return err
	}
	if err := validateInterfaceName(tunDev); err != nil {
		return err
	}
	if event != "timer" && event != "network-change" {
		return fmt.Errorf("invalid healthcheck event %q", event)
	}
	if attempts < 1 || attempts > 10 || timeout <= 0 || timeout > 30*time.Second || gap < 0 || gap > 30*time.Second || minUptime < 0 || minUptime > 24*time.Hour {
		return fmt.Errorf("healthcheck limits are outside the safe range")
	}
	if time.Duration(attempts)*timeout+time.Duration(attempts-1)*gap > 2*time.Minute {
		return fmt.Errorf("healthcheck probe budget exceeds two minutes")
	}
	parsed, err := url.Parse(probeURL)
	if err != nil || parsed.Hostname() == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil {
		return fmt.Errorf("healthcheck URL must be HTTP(S), include a host, and contain no credentials")
	}
	return nil
}

func validateLoopbackProxyAddress(address string) error {
	host, portText, err := net.SplitHostPort(address)
	port, portErr := strconv.Atoi(portText)
	ip := net.ParseIP(strings.Trim(host, "[]"))
	if err != nil || portErr != nil || port < 1 || port > 65535 || ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("healthcheck proxy must be a literal loopback address with a valid port")
	}
	return nil
}

func logHealthReport(report HealthReport, event string) {
	for _, result := range report.Results {
		status := "ok"
		if !result.OK {
			status = "failed"
		}
		logHealthcheck(
			"classification=%s event=%s layer=%s status=%s duration_ms=%d detail=%q",
			report.Failure, event, result.Layer, status, result.Duration.Milliseconds(), redactDiagnosticText(result.Detail),
		)
	}
}

func envOr(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

func envInt(name string, fallback int) int {
	v, err := strconv.Atoi(os.Getenv(name))
	if err != nil || v <= 0 {
		return fallback
	}
	return v
}

func envDurationSeconds(name string, fallback time.Duration) time.Duration {
	v, err := strconv.Atoi(os.Getenv(name))
	if err != nil || v <= 0 {
		return fallback
	}
	return time.Duration(v) * time.Second
}

func logHealthcheck(format string, args ...any) {
	fmt.Printf("[%s] %s\n", time.Now().Format("2006-01-02 15:04:05"), fmt.Sprintf(format, args...))
}

func systemctlQuiet(args ...string) bool {
	_, err := execx.Run(currentScope().systemctl(args...), &execx.Opt{Capture: true})
	return err == nil
}

func runtimeProxyAddr(service string) string {
	raw, err := os.ReadFile(filepath.Join(currentScope().RuntimeDir, service+".json"))
	if err != nil {
		return ""
	}
	var cfg struct {
		Inbounds []struct {
			Type       string `json:"type"`
			ListenPort int    `json:"listen_port"`
		} `json:"inbounds"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return ""
	}
	for _, in := range cfg.Inbounds {
		if in.Type == "mixed" && in.ListenPort > 0 {
			return "127.0.0.1:" + strconv.Itoa(in.ListenPort)
		}
	}
	return ""
}

func haveUplink(tunDev string) bool {
	raw, err := os.ReadFile("/proc/net/route")
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(raw), "\n")[1:] {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		iface, destination, flagsHex := fields[0], fields[1], fields[3]
		if iface == "" || iface == tunDev || destination != "00000000" {
			continue
		}
		flags, err := strconv.ParseInt(flagsHex, 16, 64)
		if err == nil && flags&0x1 == 0x1 {
			return true
		}
	}
	return false
}

func proxyWorks(proxyAddr, probeURL string, attempts int, timeout, gap time.Duration) bool {
	if attempts <= 0 {
		attempts = 1
	}
	proxyURL, err := url.Parse("http://" + proxyAddr)
	if err != nil {
		return false
	}
	client := &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
		},
	}
	for i := 1; i <= attempts; i++ {
		if httpProbe(client, probeURL) {
			return true
		}
		if i < attempts {
			time.Sleep(gap)
		}
	}
	return false
}

func httpProbe(client *http.Client, probeURL string) bool {
	req, err := http.NewRequest(http.MethodGet, probeURL, nil)
	if err != nil {
		return false
	}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode < http.StatusBadRequest
}

func serviceUptime(service string) time.Duration {
	res, err := execx.Run(currentScope().systemctl("show", "-p", "ActiveEnterTimestampMonotonic", "--value", service), &execx.Opt{Capture: true})
	if err != nil {
		return 999999 * time.Second
	}
	startUsec, err := strconv.ParseInt(strings.TrimSpace(res.Stdout), 10, 64)
	if err != nil || startUsec <= 0 {
		return 999999 * time.Second
	}
	raw, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 999999 * time.Second
	}
	fields := strings.Fields(string(raw))
	if len(fields) == 0 {
		return 999999 * time.Second
	}
	uptime, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 999999 * time.Second
	}
	elapsed := time.Duration(uptime*float64(time.Second)) - time.Duration(startUsec)*time.Microsecond
	if elapsed < 0 {
		return 0
	}
	return elapsed
}
