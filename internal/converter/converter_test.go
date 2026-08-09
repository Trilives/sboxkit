package converter

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Trilives/sboxkit/internal/config"
	"github.com/Trilives/sboxkit/internal/paths"
	"github.com/Trilives/sboxkit/internal/testkit"
)

func TestClashToSingBoxConvertsBasicFixture(t *testing.T) {
	p := paths.FromRoot("/opt/sboxkit")
	cfg := config.Defaults()

	result, info, err := ClashToSingBox(testkit.ReadFixture(t, "testdata/converter/clash-basic.yaml"), cfg, p)
	if err != nil {
		t.Fatalf("convert clash: %v", err)
	}

	gotInfo, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("marshal info: %v", err)
	}
	testkit.AssertJSONEqual(t, testkit.ReadFixture(t, "testdata/converter/clash-basic.expected-info.json"), string(gotInfo))

	if len(result.Inbounds) != 2 {
		t.Fatalf("expected tun + mixed inbounds, got %d", len(result.Inbounds))
	}
	if result.Route.Final != "Proxy" {
		t.Fatalf("expected Proxy final route, got %q", result.Route.Final)
	}

	tags := outboundTags(result.Outbounds)
	for _, want := range []string{"hk-01", "sg-01", "Proxy", "Auto", "DIRECT", "BLOCK"} {
		if !tags[want] {
			t.Fatalf("missing outbound tag %q in %#v", want, tags)
		}
	}
}

func TestClashToSingBoxUsesConfiguredMixedPort(t *testing.T) {
	cfg := config.Defaults()
	cfg.MixedPort = 17890
	result, _, err := ClashToSingBox(testkit.ReadFixture(t, "testdata/converter/clash-basic.yaml"), cfg, paths.FromRoot(t.TempDir()))
	if err != nil {
		t.Fatalf("convert clash: %v", err)
	}
	for _, inbound := range result.Inbounds {
		if inbound["type"] == "mixed" {
			if got := inbound["listen_port"]; got != 17890 {
				t.Fatalf("mixed listen_port = %v, want 17890", got)
			}
			return
		}
	}
	t.Fatal("mixed inbound not found")
}

func TestClashToSingBoxAlwaysSetsExternalUI(t *testing.T) {
	// 面板走 sing-box 内置的 :9090/ui/ 路径（与 mihomo 版一致），因此 external_ui
	// 始终指向内置面板目录，不随 lan_panel 开关变化——lan_panel 只决定
	// external_controller 绑定 127.0.0.1 还是 0.0.0.0。
	p := paths.FromRoot(t.TempDir())
	result, _, err := ClashToSingBox(testkit.ReadFixture(t, "testdata/converter/clash-basic.yaml"), config.Defaults(), p)
	if err != nil {
		t.Fatalf("convert clash: %v", err)
	}
	if result.Experimental.ClashAPI.ExternalUI != p.UI {
		t.Fatalf("expected external UI to always point at %q, got %q", p.UI, result.Experimental.ClashAPI.ExternalUI)
	}
	if result.Experimental.ClashAPI.ExternalController != "127.0.0.1:9090" {
		t.Fatalf("unexpected controller %q", result.Experimental.ClashAPI.ExternalController)
	}
}

func TestClashToSingBoxBindsControllerToLANWhenLanPanelEnabled(t *testing.T) {
	p := paths.FromRoot(t.TempDir())
	cfg := config.Defaults()
	cfg.LanPanel = true

	result, _, err := ClashToSingBox(testkit.ReadFixture(t, "testdata/converter/clash-basic.yaml"), cfg, p)
	if err != nil {
		t.Fatalf("convert clash: %v", err)
	}
	if result.Experimental.ClashAPI.ExternalUI != p.UI {
		t.Fatalf("unexpected external UI path %q", result.Experimental.ClashAPI.ExternalUI)
	}
	if result.Experimental.ClashAPI.ExternalController != "0.0.0.0:9090" {
		t.Fatalf("unexpected controller %q", result.Experimental.ClashAPI.ExternalController)
	}
}

func TestClashToSingBoxOmitsRuleSetsBeforeAssetsExist(t *testing.T) {
	p := paths.FromRoot(t.TempDir())
	result, _, err := ClashToSingBox(testkit.ReadFixture(t, "testdata/converter/clash-basic.yaml"), config.Defaults(), p)
	if err != nil {
		t.Fatalf("convert clash: %v", err)
	}
	if len(result.Route.RuleSet) != 0 {
		t.Fatalf("expected no local rule sets before assets exist, got %#v", result.Route.RuleSet)
	}
	for _, rule := range result.Route.Rules {
		if _, ok := rule["rule_set"]; ok {
			t.Fatalf("expected no route rule_set before assets exist, got %#v", rule)
		}
	}
}

func TestClashToSingBoxEnablesRuleSetsAfterAssetsExist(t *testing.T) {
	p := paths.FromRoot(t.TempDir())
	if err := os.MkdirAll(filepath.Dir(p.GeositeCN), 0o755); err != nil {
		t.Fatalf("create ruleset dir: %v", err)
	}
	if err := os.WriteFile(p.GeositeCN, []byte("geosite"), 0o644); err != nil {
		t.Fatalf("write geosite: %v", err)
	}
	if err := os.WriteFile(p.GeoIPCN, []byte("geoip"), 0o644); err != nil {
		t.Fatalf("write geoip: %v", err)
	}

	result, _, err := ClashToSingBox(testkit.ReadFixture(t, "testdata/converter/clash-basic.yaml"), config.Defaults(), p)
	if err != nil {
		t.Fatalf("convert clash: %v", err)
	}
	if len(result.Route.RuleSet) != 2 {
		t.Fatalf("expected local rule sets after assets exist, got %#v", result.Route.RuleSet)
	}
	foundRouteRule := false
	for _, rule := range result.Route.Rules {
		if _, ok := rule["rule_set"]; ok {
			foundRouteRule = true
			break
		}
	}
	if !foundRouteRule {
		t.Fatal("expected route rule_set after assets exist")
	}
}

func TestClashToSingBoxRejectsEmptyProxyList(t *testing.T) {
	_, _, err := ClashToSingBox("proxies: []", config.Defaults(), paths.FromRoot(t.TempDir()))
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "proxies") {
		t.Fatalf("expected proxies error, got %v", err)
	}
}

func TestSingBoxDirectAddsClashAPIWhenPassthrough(t *testing.T) {
	raw := `{"inbounds":[{"type":"mixed","tag":"mixed-in"}],"outbounds":[{"type":"direct","tag":"DIRECT"}],"route":{"final":"DIRECT"},"dns":{"servers":[]}}`

	result, info, err := SingBoxDirect(raw, config.Defaults(), paths.FromRoot("/opt/sboxkit"), false)
	if err != nil {
		t.Fatalf("direct sing-box: %v", err)
	}
	if info["mode"] != "passthrough" {
		t.Fatalf("expected passthrough mode, got %#v", info)
	}
	if result.Experimental.ClashAPI.ExternalController != "127.0.0.1:9090" {
		t.Fatalf("unexpected controller %q", result.Experimental.ClashAPI.ExternalController)
	}
}

func TestClashToSingBoxPreservesFakeIPFilterAndHosts(t *testing.T) {
	raw := `
dns:
  enhanced-mode: fake-ip
  fake-ip-range: 198.19.0.0/16
  fake-ip-filter:
    - "*.lan"
    - "+.pool.ntp.org"
    - "exact.example"
    - "RULE-SET,private"
hosts:
  router.lan: 192.168.1.1
proxies:
  - {name: ss-01, type: ss, server: 1.2.3.4, port: 443, cipher: aes-128-gcm, password: pw}
`
	result, info, err := ClashToSingBox(raw, config.Defaults(), paths.FromRoot(t.TempDir()))
	if err != nil {
		t.Fatalf("convert clash fake-ip: %v", err)
	}
	assertDNSServerType(t, result.DNS, "fakeip")
	assertDNSServerType(t, result.DNS, "hosts")
	if result.Experimental.CacheFile["store_fakeip"] != true {
		t.Fatalf("fake-ip cache not enabled: %#v", result.Experimental.CacheFile)
	}
	if skipped, ok := info["fake_ip_filter_skipped"].([]string); !ok || len(skipped) != 1 {
		t.Fatalf("unsupported filter should be reported: %#v", info)
	}
	joined, _ := json.Marshal(result.DNS["rules"])
	for _, want := range []string{"lan", "pool.ntp.org", "exact.example", fakeIPDNSTag} {
		if !strings.Contains(string(joined), want) {
			t.Fatalf("DNS rules missing %q: %s", want, joined)
		}
	}
}

func TestSingBoxCustomizedPreservesFakeIPSemantics(t *testing.T) {
	raw := `{
  "dns": {
    "servers": [
      {"type":"udp","tag":"real","server":"1.1.1.1"},
      {"type":"fakeip","tag":"source-fake","inet4_range":"198.20.0.0/16"}
    ],
    "rules": [
      {"domain_suffix":["lan"],"action":"route","server":"real"},
      {"query_type":["A","AAAA"],"action":"route","server":"source-fake"}
    ]
  },
  "outbounds": [{"type":"shadowsocks","tag":"node","server":"1.2.3.4","server_port":443,"method":"aes-128-gcm","password":"pw"}]
}`
	result, _, err := SingBoxDirect(raw, config.Defaults(), paths.FromRoot(t.TempDir()), true)
	if err != nil {
		t.Fatalf("customize sing-box: %v", err)
	}
	assertDNSServerType(t, result.DNS, "fakeip")
	joined, _ := json.Marshal(result.DNS["rules"])
	if !strings.Contains(string(joined), "lan") {
		t.Fatalf("source fake-ip exclusion missing: %s", joined)
	}
}

func TestSingBoxPassthroughRetainsUnknownSections(t *testing.T) {
	raw := `{
  "ntp":{"enabled":true,"server":"time.example"},
  "services":[{"type":"resolved"}],
  "inbounds":[{"type":"mixed","tag":"mixed-in","listen_port":7890}],
  "outbounds":[{"type":"direct","tag":"DIRECT"}],
  "route":{"final":"DIRECT","rules":[{"action":"sniff"}]},
  "dns":{"servers":[]},
  "experimental":{"clash_api":{"external_controller":"127.0.0.1:9999","secret":"keep-me"}}
}`
	result, _, err := SingBoxDirect(raw, config.Defaults(), paths.FromRoot(t.TempDir()), false)
	if err != nil {
		t.Fatalf("passthrough: %v", err)
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal passthrough: %v", err)
	}
	for _, want := range []string{`"ntp"`, `"services"`, `"time.example"`, `"action":"sniff"`, `"secret":"keep-me"`, `"external_ui"`} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("passthrough lost %s: %s", want, data)
		}
	}
}

func TestGeneratedFakeIPConfigPassesBundledSingBoxCheck(t *testing.T) {
	bin := os.Getenv("SING_BOX_BIN")
	if bin == "" {
		t.Skip("SING_BOX_BIN is not set")
	}
	raw := `dns: {enhanced-mode: fake-ip, fake-ip-filter: ["*.lan"]}
proxies:
  - {name: ss-01, type: ss, server: 1.2.3.4, port: 443, cipher: aes-128-gcm, password: pw}`
	result, _, err := ClashToSingBox(raw, config.Defaults(), paths.FromRoot(t.TempDir()))
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(file, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command(bin, "check", "-c", file).CombinedOutput(); err != nil {
		t.Fatalf("sing-box check: %v\n%s", err, output)
	}
}

func assertDNSServerType(t *testing.T, dns map[string]any, want string) {
	t.Helper()
	servers, _ := dns["servers"].([]map[string]any)
	for _, server := range servers {
		if server["type"] == want {
			return
		}
	}
	t.Fatalf("DNS server type %q not found: %#v", want, servers)
}

func outboundTags(outbounds []map[string]any) map[string]bool {
	tags := make(map[string]bool, len(outbounds))
	for _, outbound := range outbounds {
		tag, _ := outbound["tag"].(string)
		tags[tag] = true
	}
	return tags
}
