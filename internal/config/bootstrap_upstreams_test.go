package config

import (
	"net"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func TestDefaultBootstrapDNSUpstreamsAreIndependentAndDiverse(t *testing.T) {
	first := Defaults()
	second := Defaults()
	if len(first.BootstrapDNSUpstreams) < 2 {
		t.Fatalf("default bootstrap upstreams = %#v, want at least two", first.BootstrapDNSUpstreams)
	}

	providers := map[string]bool{}
	transports := map[string]bool{}
	endpoints := map[string]bool{}
	for _, upstream := range first.BootstrapDNSUpstreams {
		assertValidBootstrapUpstream(t, upstream)
		provider := strings.ToLower(strings.TrimSpace(upstream.Provider))
		if provider == "" {
			t.Fatalf("default upstream has no provider identity: %#v", upstream)
		}
		providers[provider] = true
		transports[strings.ToLower(upstream.Type)] = true
		endpoint := bootstrapUpstreamEndpoint(upstream)
		if endpoints[endpoint] {
			t.Fatalf("duplicate default bootstrap endpoint %q in %#v", endpoint, first.BootstrapDNSUpstreams)
		}
		endpoints[endpoint] = true
	}
	if len(providers) < 2 || len(transports) < 2 {
		t.Fatalf("defaults must span at least two providers and transports: providers=%v transports=%v upstreams=%#v", providers, transports, first.BootstrapDNSUpstreams)
	}

	first.BootstrapDNSUpstreams[0].Server = "192.0.2.1"
	if second.BootstrapDNSUpstreams[0].Server == "192.0.2.1" {
		t.Fatal("Defaults returned bootstrap upstream slices sharing mutable storage")
	}
}

func TestLoadMigratesLegacyBootstrapFieldsToOrderedFailover(t *testing.T) {
	p := testPaths(t)
	legacy := `{
  "bootstrap_dns_type":"https",
  "bootstrap_dns_server":"8.8.8.8",
  "bootstrap_dns_port":443,
  "bootstrap_dns_path":"/resolve",
  "bootstrap_dns_tls_server_name":"dns.google"
}`
	if err := os.WriteFile(p.CustomizeFile, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}

	got := Load(p)
	if len(got.BootstrapDNSUpstreams) < 2 {
		t.Fatalf("legacy single bootstrap was not given a failover: %#v", got.BootstrapDNSUpstreams)
	}
	primary := got.BootstrapDNSUpstreams[0]
	if primary.Type != "https" || primary.Server != "8.8.8.8" || primary.Port != 443 || primary.Path != "/resolve" || primary.TLSServerName != "dns.google" {
		t.Fatalf("legacy bootstrap was not preserved as the first upstream: %#v", primary)
	}
	if bootstrapUpstreamEndpoint(got.BootstrapDNSUpstreams[1]) == bootstrapUpstreamEndpoint(primary) {
		t.Fatalf("migration appended a duplicate failover: %#v", got.BootstrapDNSUpstreams)
	}
}

func TestNormalizeBootstrapDNSUpstreamsValidatesDeduplicatesAndNeverSelfResolves(t *testing.T) {
	cfg := Defaults()
	cfg.BootstrapDNSUpstreams = []BootstrapDNSUpstream{
		{Provider: "Cloudflare", Type: "https", Server: " 1.1.1.1 ", Port: 443, Path: "/dns-query", TLSServerName: "cloudflare-dns.com"},
		{Provider: "duplicate", Type: "HTTPS", Server: "1.1.1.1", Port: 443, Path: "/dns-query", TLSServerName: "cloudflare-dns.com"},
		{Provider: "recursive", Type: "https", Server: "resolver.example", Port: 443, Path: "/dns-query", TLSServerName: "resolver.example"},
		{Provider: "bad-port", Type: "udp", Server: "9.9.9.9", Port: 70000},
		{Provider: "AliDNS", Type: "tcp", Server: "223.5.5.5", Port: 53},
	}
	original := slices.Clone(cfg.BootstrapDNSUpstreams)

	got := NormalizeBootstrapDNS(cfg)
	if !slices.Equal(cfg.BootstrapDNSUpstreams, original) {
		t.Fatalf("NormalizeBootstrapDNS mutated its input: before=%#v after=%#v", original, cfg.BootstrapDNSUpstreams)
	}
	if len(got.BootstrapDNSUpstreams) != 2 {
		t.Fatalf("normalized upstreams = %#v, want the two valid unique entries", got.BootstrapDNSUpstreams)
	}
	if got.BootstrapDNSUpstreams[0].Server != "1.1.1.1" || got.BootstrapDNSUpstreams[1].Server != "223.5.5.5" {
		t.Fatalf("normalization did not preserve valid upstream order: %#v", got.BootstrapDNSUpstreams)
	}
	for _, upstream := range got.BootstrapDNSUpstreams {
		assertValidBootstrapUpstream(t, upstream)
	}
}

func TestNormalizeBootstrapDNSUpstreamsRepairsSingleUpstreamWithDistinctFallback(t *testing.T) {
	cfg := Defaults()
	cfg.BootstrapDNSUpstreams = []BootstrapDNSUpstream{
		{Provider: "Google", Type: "https", Server: "8.8.8.8", Port: 443, Path: "/dns-query", TLSServerName: "dns.google"},
	}

	got := NormalizeBootstrapDNS(cfg)
	if len(got.BootstrapDNSUpstreams) < 2 {
		t.Fatalf("single configured upstream must gain a safe fallback: %#v", got.BootstrapDNSUpstreams)
	}
	if got.BootstrapDNSUpstreams[0].Server != "8.8.8.8" {
		t.Fatalf("configured primary order was not preserved: %#v", got.BootstrapDNSUpstreams)
	}
	if bootstrapUpstreamEndpoint(got.BootstrapDNSUpstreams[0]) == bootstrapUpstreamEndpoint(got.BootstrapDNSUpstreams[1]) {
		t.Fatalf("automatic fallback duplicates primary: %#v", got.BootstrapDNSUpstreams)
	}
}

func assertValidBootstrapUpstream(t *testing.T, upstream BootstrapDNSUpstream) {
	t.Helper()
	if net.ParseIP(strings.TrimSpace(upstream.Server)) == nil {
		t.Fatalf("bootstrap upstream must use an IP literal and cannot self-resolve: %#v", upstream)
	}
	if upstream.Port < 1 || upstream.Port > 65535 {
		t.Fatalf("bootstrap upstream has invalid port: %#v", upstream)
	}
	switch upstream.Type {
	case "udp", "tcp":
	case "https":
		if !strings.HasPrefix(upstream.Path, "/") || strings.TrimSpace(upstream.TLSServerName) == "" {
			t.Fatalf("HTTPS bootstrap lacks a valid path or TLS name: %#v", upstream)
		}
	default:
		t.Fatalf("bootstrap failover upstream uses unsupported/non-deterministic transport: %#v", upstream)
	}
}

func bootstrapUpstreamEndpoint(upstream BootstrapDNSUpstream) string {
	return strings.ToLower(strings.TrimSpace(upstream.Type)) + "://" + strings.TrimSpace(upstream.Server) + ":" + strconv.Itoa(upstream.Port)
}
