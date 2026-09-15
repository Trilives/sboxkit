package sysd

import (
	"context"
	"encoding/binary"
	"net"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Trilives/sboxkit/internal/bootstrapdns"
	"github.com/Trilives/sboxkit/internal/config"
)

func TestBuildDNSQueryValidatesNamesAndBuildsAnAQuestion(t *testing.T) {
	t.Parallel()

	query, err := buildDNSQuery("node.example.")
	if err != nil {
		t.Fatal(err)
	}
	if len(query) < 12 || binary.BigEndian.Uint16(query[4:6]) != 1 || query[2]&1 == 0 {
		t.Fatalf("DNS header = %v", query[:12])
	}
	wantQuestion := []byte{4, 'n', 'o', 'd', 'e', 7, 'e', 'x', 'a', 'm', 'p', 'l', 'e', 0, 0, 1, 0, 1}
	if got := query[12:]; !reflect.DeepEqual(got, wantQuestion) {
		t.Fatalf("question = %v, want %v", got, wantQuestion)
	}
	for _, invalid := range []string{"", ".", strings.Repeat("a", 64) + ".example", strings.Repeat("a", 254)} {
		if _, err := buildDNSQuery(invalid); err == nil {
			t.Errorf("buildDNSQuery(%q) error = nil", invalid)
		}
	}
}

func TestRequireSuccessfulDNSResponseChecksRcodeAndAnswers(t *testing.T) {
	t.Parallel()

	response := make([]byte, 12)
	if err := requireSuccessfulDNSResponse(response, false); err != nil {
		t.Fatal(err)
	}
	if err := requireSuccessfulDNSResponse(response, true); err == nil {
		t.Fatal("empty answer accepted when an answer is required")
	}
	binary.BigEndian.PutUint16(response[6:8], 1)
	if err := requireSuccessfulDNSResponse(response, true); err != nil {
		t.Fatal(err)
	}
	response[3] = 3
	if err := requireSuccessfulDNSResponse(response, false); err == nil {
		t.Fatal("NXDOMAIN response accepted")
	}
	if err := requireSuccessfulDNSResponse([]byte{1}, false); err == nil {
		t.Fatal("short response accepted")
	}
}

func TestActiveNodeDomainsFollowRuntimeSelectorChoice(t *testing.T) {
	t.Parallel()

	document := map[string]any{
		"route": map[string]any{"final": "Proxy"},
		"outbounds": []any{
			map[string]any{"type": "selector", "tag": "Proxy", "outbounds": []any{"unused", "Fallback"}, "default": "unused"},
			map[string]any{"type": "urltest", "tag": "Fallback", "outbounds": []any{"unused", "active"}},
			map[string]any{"type": "vmess", "tag": "unused", "server": "broken.example"},
			map[string]any{"type": "vmess", "tag": "active", "server": "active.example"},
		},
	}
	selections := map[string]string{"Proxy": "Fallback", "Fallback": "active"}
	got := activeNodeDomainsFromDocument(document, func(group string) (string, error) {
		return selections[group], nil
	})
	want := []string{"active.example"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("domains = %v, want %v", got, want)
	}
	if got := activeNodeDomainsFromDocument(document, nil); !reflect.DeepEqual(got, []string{"broken.example"}) {
		t.Fatalf("default selector domains = %v", got)
	}
}

func TestActiveNodeDomainsIgnoreAnActiveIPLiteral(t *testing.T) {
	t.Parallel()
	document := map[string]any{"outbounds": []any{
		map[string]any{"type": "selector", "tag": "Proxy", "outbounds": []any{"active"}},
		map[string]any{"type": "vmess", "tag": "active", "server": "192.0.2.10"},
	}}
	if got := activeNodeDomainsFromDocument(document, nil); got != nil {
		t.Fatalf("IP-literal active node domains = %v", got)
	}
}

func TestActiveNodeDomainsDoNotGuessWhenLiveSelectorLookupFails(t *testing.T) {
	t.Parallel()
	document := map[string]any{"outbounds": []any{
		map[string]any{"type": "selector", "tag": "Proxy", "outbounds": []any{"unused"}, "default": "unused"},
		map[string]any{"type": "vmess", "tag": "unused", "server": "broken.example"},
	}}
	got := activeNodeDomainsFromDocument(document, func(string) (string, error) {
		return "", context.DeadlineExceeded
	})
	if got != nil {
		t.Fatalf("failed live selector lookup guessed domains = %v", got)
	}
}

func TestAnnotateBootstrapDegradedCopiesResults(t *testing.T) {
	t.Parallel()

	report := HealthReport{Healthy: true, Failure: FailureNone, Results: []ProbeResult{
		{Layer: ProbePhysicalUplink, OK: true},
		{Layer: ProbeBootstrapDNS, OK: true},
	}}
	unchanged := annotateBootstrapDegraded(report, false)
	if !reflect.DeepEqual(unchanged, report) {
		t.Fatalf("non-degraded report changed: %#v", unchanged)
	}
	annotated := annotateBootstrapDegraded(report, true)
	if annotated.Results[1].Detail != "bootstrap_dns_degraded: fallback upstream active" {
		t.Fatalf("detail = %q", annotated.Results[1].Detail)
	}
	if report.Results[1].Detail != "" {
		t.Fatal("annotation mutated caller-owned report")
	}
}

func TestBootstrapTargetLabelsPreserveConfiguredOrder(t *testing.T) {
	t.Parallel()
	upstreams := []config.BootstrapDNSUpstream{
		{Type: "tcp", Server: "223.5.5.5", Port: 53},
		{Type: "https", Server: "1.1.1.1", Port: 443},
	}
	want := []string{"tcp://223.5.5.5:53", "https://1.1.1.1:443"}
	if got := bootstrapTargetLabels(upstreams); !reflect.DeepEqual(got, want) {
		t.Fatalf("labels = %v, want %v", got, want)
	}
}

func TestRequireInterfaceUpRejectsMissingInterface(t *testing.T) {
	t.Parallel()
	if err := requireInterfaceUp("interface-that-does-not-exist"); err == nil {
		t.Fatal("missing interface accepted")
	}
}

func TestResolveHealthDomainRejectsMissingForwarder(t *testing.T) {
	t.Parallel()
	_, err := resolveHealthDomain(nil, "example.com", 0)
	if err == nil || !strings.Contains(err.Error(), "forwarder unavailable") {
		t.Fatalf("error = %v", err)
	}
}

func TestResolveHealthDomainUsesForwarderAndRequiresCallerValidation(t *testing.T) {
	t.Parallel()
	forwarder, err := bootstrapdns.NewForwarder(bootstrapdns.Config{
		Upstreams:       []bootstrapdns.Upstream{{Name: "stub", Network: "udp", Address: "192.0.2.1:53"}},
		AttemptTimeout:  time.Second,
		FailureCooldown: time.Second,
		MaxRequestBytes: 4096,
		MaxConcurrent:   1,
		Exchange: func(_ context.Context, _ bootstrapdns.Upstream, query []byte) ([]byte, error) {
			response := append([]byte(nil), query...)
			response[2] |= 0x80
			binary.BigEndian.PutUint16(response[6:8], 1)
			return response, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	response, err := resolveHealthDomain(forwarder, "node.example", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := requireSuccessfulDNSResponse(response, true); err != nil {
		t.Fatal(err)
	}
}

func TestProbeBootstrapUpstreamsFallsBackInConfiguredOrder(t *testing.T) {
	t.Parallel()
	primary, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	primaryAddress := primary.LocalAddr().(*net.UDPAddr)
	_ = primary.Close() // deterministic local connection-refused path
	fallback, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer fallback.Close()
	go func() {
		buffer := make([]byte, 4096)
		n, address, readErr := fallback.ReadFrom(buffer)
		if readErr == nil {
			response := append([]byte(nil), buffer[:n]...)
			response[2] |= 0x80
			_, _ = fallback.WriteTo(response, address)
		}
	}()
	upstreams := []config.BootstrapDNSUpstream{
		{Provider: "primary", Type: "udp", Server: "127.0.0.1", Port: primaryAddress.Port},
		{Provider: "fallback", Type: "udp", Server: "127.0.0.1", Port: fallback.LocalAddr().(*net.UDPAddr).Port},
	}
	forwarder, degraded, err := probeBootstrapUpstreams(upstreams, "", 300*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if forwarder == nil || !degraded {
		t.Fatalf("fallback result = forwarder %v, degraded %v", forwarder, degraded)
	}
}

func TestProductionHealthProbesHaveStableDependencyOrder(t *testing.T) {
	t.Parallel()
	probes, runtime := productionHealthProbes("missing-service", "singbox", "127.0.0.1:2080", "http://127.0.0.1/", 1, time.Millisecond, 0)
	if runtime == nil {
		t.Fatal("runtime = nil")
	}
	want := []ProbeLayer{ProbePhysicalUplink, ProbeBootstrapDNS, ProbeNodeResolution, ProbeMixedPort, ProbeEndToEnd}
	got := make([]ProbeLayer, len(probes))
	for index, probe := range probes {
		got[index] = probe.Layer
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("probe order = %v, want %v", got, want)
	}
	if got := runtimeActiveNodeDomains("service-that-does-not-exist"); got != nil {
		t.Fatalf("missing runtime domains = %v", got)
	}
}
