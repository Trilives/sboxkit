//go:build linux && (amd64 || arm64)

package sysd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Trilives/sboxkit/internal/bootstrapdns"
)

const (
	relaySandboxFamiliesEnv = "SBOXKIT_RELAY_SANDBOX_FAMILIES"
	relaySandboxResult      = "relay-sandbox-result: "
	// The helper cancels the exchange right after discovery, so the dial to this
	// loopback discard port is abandoned before a socket is opened.
	relaySandboxUpstream = "127.0.0.1:9"
)

// Outcomes reported by the sandboxed helper process.
const (
	relaySandboxReachedDial = "reached-dial"
	relaySandboxPreDial     = "pre-dial-failed"
	relaySandboxSetupFailed = "setup-failed"
)

// TestBootstrapRelayUnitsAllowRuntimeInterfaceDiscovery runs the relay's real
// per-query exchange — /proc/net/route discovery and Go's netlink-backed
// interface lookup, up to the dial — inside the socket-family allow-list read
// from each generated unit, as systemd's RestrictAddressFamilies applies it.
func TestBootstrapRelayUnitsAllowRuntimeInterfaceDiscovery(t *testing.T) {
	requirePhysicalDefaultRoute(t)
	rt := runtimePaths{Dir: "/var/lib/sboxkit-runtime"}
	units := map[string]string{
		"system": bootstrapServiceText(DefaultName, rt, "/usr/bin/sboxkit"),
		"user":   bootstrapUserServiceText(DefaultName, rt, "/usr/bin/sboxkit"),
	}
	for scope, unit := range units {
		families := unitDirective(t, unit, "RestrictAddressFamilies")
		outcome, detail := runSandboxedRelayExchange(t, families)
		if outcome != relaySandboxReachedDial {
			t.Fatalf("%s relay unit (RestrictAddressFamilies=%s) breaks runtime interface discovery: %s %s",
				scope, families, outcome, detail)
		}
	}
}

// TestBootstrapRelaySandboxReproducesMissingNetlink guards the emulation itself:
// the allow-list shipped before the fix must fail exactly as it did in production.
func TestBootstrapRelaySandboxReproducesMissingNetlink(t *testing.T) {
	requirePhysicalDefaultRoute(t)
	outcome, detail := runSandboxedRelayExchange(t, "AF_INET AF_INET6")
	if outcome != relaySandboxPreDial || !strings.Contains(detail, syscall.EAFNOSUPPORT.Error()) {
		t.Fatalf("pre-fix allow-list should fail the interface lookup with EAFNOSUPPORT, got %s %s", outcome, detail)
	}
}

// TestBootstrapRelaySandboxHelper is the child half of the tests above.
func TestBootstrapRelaySandboxHelper(t *testing.T) {
	allowList := os.Getenv(relaySandboxFamiliesEnv)
	if allowList == "" {
		t.Skip("helper process for the sandboxed relay tests")
	}
	families, err := parseAddressFamilies(allowList)
	if err == nil {
		err = restrictSocketFamilies(families)
	}
	if err != nil {
		reportRelaySandbox(relaySandboxSetupFailed, err)
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	discoverThenStop := func(ctx context.Context, excluded []string) (string, error) {
		name, err := bootstrapdns.DiscoverDefaultInterface(ctx, excluded)
		cancel() // the interface lookup still runs; the dial then aborts unopened
		return name, err
	}
	exchange, err := bootstrapdns.NewDirectExchange(bootstrapdns.DirectExchangeConfig{
		ExcludedInterfaces: []string{bootstrapDNSTunDevice},
		DiscoverInterface:  discoverThenStop,
		DialTimeout:        time.Second,
	})
	if err != nil {
		reportRelaySandbox(relaySandboxSetupFailed, err)
		return
	}
	_, err = exchange(ctx, bootstrapdns.Upstream{Network: "udp", Address: relaySandboxUpstream}, minimalDNSQuery())
	reportRelaySandbox(classifyRelaySandboxError(err), err)
}

// classifyRelaySandboxError separates failures before the dial (discovery,
// interface lookup, family denial) from the expected cancelled dial.
func classifyRelaySandboxError(err error) string {
	message := ""
	if err != nil {
		message = err.Error()
	}
	if errors.Is(err, syscall.EAFNOSUPPORT) ||
		strings.HasPrefix(message, "discover physical interface:") ||
		strings.HasPrefix(message, "bind interface ") {
		return relaySandboxPreDial
	}
	if errors.Is(err, context.Canceled) {
		return relaySandboxReachedDial
	}
	return "unexpected"
}

func reportRelaySandbox(outcome string, err error) {
	detail := ""
	if err != nil {
		detail = err.Error()
	}
	fmt.Printf("%s%s %s\n", relaySandboxResult, outcome, detail)
}

func runSandboxedRelayExchange(t *testing.T, families string) (outcome, detail string) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestBootstrapRelaySandboxHelper$", "-test.count=1")
	cmd.Env = append(os.Environ(), relaySandboxFamiliesEnv+"="+families)
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	if err := cmd.Run(); err != nil {
		t.Fatalf("sandbox helper failed: %v\n%s", err, output.String())
	}
	for _, line := range strings.Split(output.String(), "\n") {
		if rest, ok := strings.CutPrefix(line, relaySandboxResult); ok {
			outcome, detail, _ = strings.Cut(rest, " ")
			break
		}
	}
	if outcome == relaySandboxSetupFailed {
		t.Skipf("seccomp is unavailable in this environment: %s", detail)
	}
	if outcome == "" {
		t.Fatalf("sandbox helper reported no result:\n%s", output.String())
	}
	return outcome, detail
}

func requirePhysicalDefaultRoute(t *testing.T) {
	t.Helper()
	if _, err := bootstrapdns.DiscoverDefaultInterface(context.Background(), []string{bootstrapDNSTunDevice}); err != nil {
		t.Skipf("host has no physical default route to bind: %v", err)
	}
}

func unitDirective(t *testing.T, unit, key string) string {
	t.Helper()
	for _, line := range strings.Split(unit, "\n") {
		if value, ok := strings.CutPrefix(line, key+"="); ok {
			return value
		}
	}
	t.Fatalf("unit has no %s=:\n%s", key, unit)
	return ""
}

// minimalDNSQuery is a valid one-question A query for example.com.
func minimalDNSQuery() []byte {
	query := []byte{0x12, 0x34, 0x01, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	for _, label := range []string{"example", "com"} {
		query = append(query, byte(len(label)))
		query = append(query, label...)
	}
	return append(query, 0x00, 0x00, 0x01, 0x00, 0x01)
}
