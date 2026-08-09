package converter

import (
	"regexp"
	"strings"
)

const (
	fakeIPDNSTag = "dns-fakeip"
	hostsDNSTag  = "dns-hosts"
)

type sourceOptions struct {
	FakeIP               *fakeIPOptions
	Hosts                map[string]any
	SkippedFakeIPFilters []string
}

type fakeIPOptions struct {
	Inet4Range     string
	Inet6Range     string
	ExclusionRules []map[string]any
}

func clashSourceOptions(root map[string]any) sourceOptions {
	var out sourceOptions
	if hosts, ok := normalizeMap(root["hosts"]); ok {
		out.Hosts = hosts
	}
	dns, ok := normalizeMap(root["dns"])
	if !ok || !strings.EqualFold(asString(dns["enhanced-mode"]), "fake-ip") {
		return out
	}
	fake := &fakeIPOptions{
		Inet4Range: orString(asString(dns["fake-ip-range"]), "198.18.0.0/15"),
		Inet6Range: orString(asString(dns["fake-ip-range6"]), "fc00::/18"),
	}
	filters := stringSlice(firstValue(dns, "fake-ip-filter", "fake_ip_filter"))
	fake.ExclusionRules, out.SkippedFakeIPFilters = clashFakeIPFilterRules(filters)
	out.FakeIP = fake
	return out
}

func singBoxSourceOptions(doc map[string]any) sourceOptions {
	var out sourceOptions
	dns, ok := normalizeMap(doc["dns"])
	if !ok {
		return out
	}
	servers, _ := dns["servers"].([]any)
	fakeTag := ""
	for _, raw := range servers {
		server, ok := normalizeMap(raw)
		if !ok {
			continue
		}
		switch asString(server["type"]) {
		case "fakeip":
			fakeTag = asString(server["tag"])
			out.FakeIP = &fakeIPOptions{
				Inet4Range: orString(asString(server["inet4_range"]), "198.18.0.0/15"),
				Inet6Range: orString(asString(server["inet6_range"]), "fc00::/18"),
			}
		case "hosts":
			if predefined, ok := normalizeMap(server["predefined"]); ok {
				out.Hosts = predefined
			}
		}
	}
	if out.FakeIP != nil {
		out.FakeIP.ExclusionRules = singBoxFakeIPExclusions(dns, fakeTag)
	}
	return out
}

func clashFakeIPFilterRules(filters []string) ([]map[string]any, []string) {
	exact, suffix, regexes := []string{}, []string{}, []string{}
	skipped := []string{}
	for _, raw := range filters {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		if strings.HasPrefix(value, "RULE-SET,") || strings.HasPrefix(value, "GEOSITE,") {
			skipped = append(skipped, value)
			continue
		}
		switch {
		case strings.HasPrefix(value, "+."):
			suffix = append(suffix, strings.TrimPrefix(value, "+."))
		case strings.HasPrefix(value, "*."):
			suffix = append(suffix, strings.TrimPrefix(value, "*."))
		case strings.HasPrefix(value, "."):
			suffix = append(suffix, strings.TrimPrefix(value, "."))
		case strings.Contains(value, "*"):
			expr := regexp.QuoteMeta(value)
			regexes = append(regexes, "^"+strings.ReplaceAll(expr, `\*`, ".*")+"$")
		default:
			exact = append(exact, value)
		}
	}
	rule := map[string]any{"action": "route", "server": remoteDNSTag}
	if len(exact) > 0 {
		rule["domain"] = exact
	}
	if len(suffix) > 0 {
		rule["domain_suffix"] = suffix
	}
	if len(regexes) > 0 {
		rule["domain_regex"] = regexes
	}
	if len(rule) == 2 {
		return nil, skipped
	}
	return []map[string]any{rule}, skipped
}

func singBoxFakeIPExclusions(dns map[string]any, fakeTag string) []map[string]any {
	rules, _ := dns["rules"].([]any)
	out := []map[string]any{}
	for _, raw := range rules {
		rule, ok := normalizeMap(raw)
		if !ok || asString(rule["server"]) == fakeTag {
			continue
		}
		kept := map[string]any{"action": "route", "server": remoteDNSTag}
		for _, key := range []string{"domain", "domain_suffix", "domain_keyword", "domain_regex"} {
			if value, exists := rule[key]; exists {
				kept[key] = value
			}
		}
		if len(kept) > 2 {
			out = append(out, kept)
		}
	}
	return out
}

func orString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
