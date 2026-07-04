package sysd

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Trilives/sboxkit/internal/execx"
	"github.com/Trilives/sboxkit/internal/paths"
)

const (
	defaultProbeURL      = "http://connectivitycheck.gstatic.com/generate_204"
	defaultProxyAddr     = "127.0.0.1:7890"
	defaultProbeAttempts = 3
	defaultProbeTimeout  = 8 * time.Second
	defaultProbeGap      = 4 * time.Second
	defaultMinUptime     = 90 * time.Second
)

// RunHealthcheck is the watchdog probe used by sing-box-watchdog.service.
// It restarts sing-box only when the host has a real uplink but the local
// proxy is unresponsive for several attempts.
func RunHealthcheck(args []string) int {
	fs := flag.NewFlagSet("healthcheck", flag.ContinueOnError)
	service := fs.String("service", envOr("SERVICE_NAME", DefaultName), "systemd service name")
	tunDev := fs.String("tun-dev", envOr("TUN_DEV", "singbox"), "TUN interface to ignore")
	proxyAddr := fs.String("proxy", envOr("PROXY_ADDR", ""), "HTTP proxy address")
	probeURL := fs.String("url", envOr("PROBE_URL", defaultProbeURL), "probe URL")
	attempts := fs.Int("attempts", envInt("PROBE_ATTEMPTS", defaultProbeAttempts), "probe attempts")
	timeout := fs.Duration("timeout", envDurationSeconds("PROBE_TIMEOUT", defaultProbeTimeout), "per-attempt timeout")
	gap := fs.Duration("gap", envDurationSeconds("PROBE_GAP", defaultProbeGap), "gap between attempts")
	minUptime := fs.Duration("min-uptime", envDurationSeconds("MIN_UPTIME", defaultMinUptime), "minimum service uptime before restart")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	if !systemctlQuiet("is-active", *service) {
		logHealthcheck("%s is not active; leaving it to systemd.", *service)
		return 0
	}
	if !haveUplink(*tunDev) {
		logHealthcheck("No uplink except %s/none; restart cannot help. Skipping.", *tunDev)
		return 0
	}

	addr := strings.TrimSpace(*proxyAddr)
	if addr == "" {
		addr = runtimeProxyAddr(*service)
	}
	if addr == "" {
		addr = defaultProxyAddr
	}
	if proxyWorks(addr, *probeURL, *attempts, *timeout, *gap) {
		return 0
	}

	uptime := serviceUptime(*service)
	if uptime < *minUptime {
		logHealthcheck("Proxy probe failed but %s is only %.0fs old; letting it settle.", *service, uptime.Seconds())
		return 0
	}

	logHealthcheck("Uplink present but proxy %s is dead after %d tries; restarting %s.", addr, *attempts, *service)
	if _, err := execx.Run([]string{"systemctl", "restart", *service}, &execx.Opt{Capture: true}); err != nil {
		execx.Error(err.Error())
		return 1
	}
	return 0
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
	_, err := execx.Run(append([]string{"systemctl"}, args...), &execx.Opt{Capture: true})
	return err == nil
}

func runtimeProxyAddr(service string) string {
	raw, err := os.ReadFile(paths.RuntimeDir + "/" + service + ".json")
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
	res, err := execx.Run([]string{"systemctl", "show", "-p", "ActiveEnterTimestampMonotonic", "--value", service}, &execx.Opt{Capture: true})
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
