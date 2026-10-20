package converter

import (
	"net"
	"testing"

	"github.com/Trilives/sboxkit/internal/config"
	"github.com/Trilives/sboxkit/internal/paths"
)

func TestGeneratedDNSUsesDirectLoopbackRelayAndProxiedRemoteDoH(t *testing.T) {
	result := failoverConvert(t, false, false)
	servers, ok := result.DNS["servers"].([]map[string]any)
	if !ok {
		t.Fatalf("DNS servers have unexpected type: %#v", result.DNS["servers"])
	}

	var direct, remote map[string]any
	for _, server := range servers {
		switch server["tag"] {
		case bootstrapDNSTag:
			if direct != nil {
				t.Fatalf("generated more than one %q DNS server: %#v", bootstrapDNSTag, servers)
			}
			direct = server
		case remoteDNSTag:
			remote = server
		}
	}
	if direct == nil {
		t.Fatalf("missing %q DNS server: %#v", bootstrapDNSTag, servers)
	}
	if direct["type"] != "udp" {
		t.Fatalf("dns-direct must use the local relay's UDP listener: %#v", direct)
	}
	ip := net.ParseIP(asString(direct["server"]))
	if ip == nil || !ip.IsLoopback() || asInt(direct["server_port"]) < 1 {
		t.Fatalf("dns-direct does not point to a concrete loopback relay endpoint: %#v", direct)
	}
	if direct["detour"] == "Proxy" {
		t.Fatalf("bootstrap relay must not depend on Proxy: %#v", direct)
	}
	if _, exists := direct["domain_resolver"]; exists {
		t.Fatalf("IP-literal loopback relay must not recursively resolve itself: %#v", direct)
	}

	if remote == nil {
		t.Fatalf("missing %q DNS server: %#v", remoteDNSTag, servers)
	}
	if remote["detour"] != "Proxy" {
		t.Fatalf("remote DoH must use Proxy when direct access is blocked: %#v", remote)
	}
	if net.ParseIP(asString(remote["server"])) == nil {
		assertFailoverResolver(t, remote["domain_resolver"], "remote DoH")
	}

	// The relay owns failover. Emitting each configured upstream as another
	// sing-box DNS server would look redundant but does not implement fallback.
	configured := config.Defaults().BootstrapDNSUpstreams
	for _, upstream := range configured {
		for _, server := range servers {
			if asString(server["server"]) == upstream.Server && asInt(server["server_port"]) == upstream.Port {
				t.Fatalf("relay upstream leaked into sing-box DNS servers: upstream=%#v servers=%#v", upstream, servers)
			}
		}
	}
}

func TestNodeAndRouteResolversUseDirectRelay(t *testing.T) {
	result := failoverConvert(t, true, true)
	assertFailoverResolver(t, result.Route.DefaultResolver, "route default")

	var node, directOutbound map[string]any
	for _, outbound := range result.Outbounds {
		switch outbound["tag"] {
		case "domain-node":
			node = outbound
		case "DIRECT":
			directOutbound = outbound
		}
	}
	if node == nil || directOutbound == nil {
		t.Fatalf("expected node and DIRECT outbound: %#v", result.Outbounds)
	}
	assertFailoverResolver(t, node["domain_resolver"], "node")
	assertFailoverResolver(t, directOutbound["domain_resolver"], "DIRECT outbound")
}

func TestBootstrapFailoverPreservesTunAndFakeIPSwitchSemantics(t *testing.T) {
	for _, enableTun := range []bool{false, true} {
		for _, enableFakeIP := range []bool{false, true} {
			name := "tun=" + boolName(enableTun) + "/fakeip=" + boolName(enableFakeIP)
			t.Run(name, func(t *testing.T) {
				result := failoverConvert(t, enableTun, enableFakeIP)
				if got := failoverHasType(result.Inbounds, "tun"); got != enableTun {
					t.Fatalf("TUN presence = %v, want %v; inbounds=%#v", got, enableTun, result.Inbounds)
				}
				servers, _ := result.DNS["servers"].([]map[string]any)
				if got := failoverHasType(servers, "fakeip"); got != enableFakeIP {
					t.Fatalf("FakeIP presence = %v, want %v; servers=%#v", got, enableFakeIP, servers)
				}
				if count := failoverCountTag(servers, bootstrapDNSTag); count != 1 {
					t.Fatalf("%s count = %d, want one local relay: %#v", bootstrapDNSTag, count, servers)
				}
			})
		}
	}
}

func failoverConvert(t *testing.T, enableTun, enableFakeIP bool) Config {
	t.Helper()
	raw := `proxies:
  - {name: domain-node, type: ss, server: edge.provider.example, port: 443, cipher: aes-128-gcm, password: pw}
`
	if enableFakeIP {
		raw = `dns:
  enhanced-mode: fake-ip
  fake-ip-filter: ["*.lan"]
` + raw
	}
	cfg := config.Defaults()
	cfg.EnableTun = enableTun
	result, _, err := ClashToSingBox(raw, cfg, paths.FromRoot(t.TempDir()))
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	return result
}

func assertFailoverResolver(t *testing.T, value any, owner string) {
	t.Helper()
	resolver, ok := value.(map[string]any)
	if !ok || resolver["server"] != bootstrapDNSTag {
		t.Fatalf("%s resolver must use %q: %#v", owner, bootstrapDNSTag, value)
	}
}

func failoverHasType(items []map[string]any, typ string) bool {
	for _, item := range items {
		if item["type"] == typ {
			return true
		}
	}
	return false
}

func failoverCountTag(items []map[string]any, tag string) int {
	count := 0
	for _, item := range items {
		if item["tag"] == tag {
			count++
		}
	}
	return count
}

func boolName(value bool) string {
	if value {
		return "on"
	}
	return "off"
}
