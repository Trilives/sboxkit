package converter

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Trilives/sboxkit/internal/config"
	"github.com/Trilives/sboxkit/internal/paths"
)

const hostsAndFakeIPSource = `
dns: {enhanced-mode: fake-ip}
hosts:
  router.lan: 192.168.1.1
  agent.example: origin.example
proxies:
  - {name: ss-01, type: ss, server: edge.provider.example, port: 443, cipher: aes-128-gcm, password: pw}
`

// sing-box 1.14 rejects legacy address filters (ip_accept_any, ip_cidr,
// ip_is_private, IP-only rule-sets) once any DNS rule uses query_type, which
// the hosts CNAME and FakeIP rules always do.
func TestBuildDNSMatchesAddressFiltersAgainstEvaluatedResponse(t *testing.T) {
	p := paths.FromRoot(t.TempDir())
	writePlaceholderRuleSets(t, p)

	result, _, err := ClashToSingBox(hostsAndFakeIPSource, config.Defaults(), p)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	rules, ok := result.DNS["rules"].([]map[string]any)
	if !ok {
		t.Fatalf("dns rules have unexpected type %T", result.DNS["rules"])
	}

	assertRespondsAfterEvaluate(t, rules, "ip_accept_any", true, hostsDNSTag)
	assertRespondsAfterEvaluate(t, rules, "rule_set", geoipCNRuleSetTag, bootstrapDNSTag)
	for i, rule := range rules {
		_, acceptAny := rule["ip_accept_any"]
		_, cidr := rule["ip_cidr"]
		_, private := rule["ip_is_private"]
		ipRuleSet := rule["rule_set"] == geoipCNRuleSetTag
		if (acceptAny || cidr || private || ipRuleSet) && rule["match_response"] != true {
			t.Fatalf("dns rule[%d] uses a legacy address filter: %#v", i, rule)
		}
	}
}

func assertRespondsAfterEvaluate(t *testing.T, rules []map[string]any, field string, value any, server string) {
	t.Helper()
	for i, rule := range rules {
		if !reflect.DeepEqual(rule[field], value) {
			continue
		}
		if rule["match_response"] != true || rule["action"] != "respond" {
			t.Fatalf("dns rule[%d] must respond with the matched evaluated response: %#v", i, rule)
		}
		want := map[string]any{"query_type": []string{"A", "AAAA", "HTTPS"}, "action": "evaluate", "server": server}
		if i == 0 || !reflect.DeepEqual(rules[i-1], want) {
			t.Fatalf("dns rule[%d] must follow an address-query evaluate via %s: %#v", i, server, rules)
		}
		return
	}
	t.Fatalf("no dns rule with %s=%v: %#v", field, value, rules)
}

func writePlaceholderRuleSets(t *testing.T, p paths.Paths) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p.GeositeCN), 0o755); err != nil {
		t.Fatalf("create ruleset dir: %v", err)
	}
	for _, file := range []string{p.GeositeCN, p.GeoIPCN} {
		if err := os.WriteFile(file, []byte(filepath.Base(file)), 0o644); err != nil {
			t.Fatalf("write %s: %v", file, err)
		}
	}
}
