package converter

import (
	"encoding/json"
	"os"
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

func TestClashToSingBoxOmitsExternalUIByDefault(t *testing.T) {
	result, _, err := ClashToSingBox(testkit.ReadFixture(t, "testdata/converter/clash-basic.yaml"), config.Defaults(), paths.FromRoot(t.TempDir()))
	if err != nil {
		t.Fatalf("convert clash: %v", err)
	}
	if result.Experimental.ClashAPI.ExternalUI != "" {
		t.Fatalf("expected external UI disabled by default, got %q", result.Experimental.ClashAPI.ExternalUI)
	}
}

func TestClashToSingBoxEnablesExternalUIWhenLanPanelEnabled(t *testing.T) {
	p := paths.FromRoot(t.TempDir())
	cfg := config.Defaults()
	cfg.LanPanel = true

	result, _, err := ClashToSingBox(testkit.ReadFixture(t, "testdata/converter/clash-basic.yaml"), cfg, p)
	if err != nil {
		t.Fatalf("convert clash: %v", err)
	}
	if result.Experimental.ClashAPI.ExternalUI != p.UIDir {
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

func outboundTags(outbounds []map[string]any) map[string]bool {
	tags := make(map[string]bool, len(outbounds))
	for _, outbound := range outbounds {
		tag, _ := outbound["tag"].(string)
		tags[tag] = true
	}
	return tags
}
