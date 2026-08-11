package converter

import (
	"net/netip"
	"regexp"
	"sort"
	"strings"
)

const (
	fakeIPDNSTag = "dns-fakeip"
	hostsDNSTag  = "dns-hosts"
)

type sourceOptions struct {
	FakeIP               *fakeIPOptions
	Hosts                map[string]any
	HostAliases          []hostAlias
	SkippedHosts         []string
	SkippedFakeIPFilters []string
}

type hostAlias struct {
	Domain string
	Target string
}

type fakeIPOptions struct {
	Inet4Range     string
	Inet6Range     string
	ExclusionRules []map[string]any
}

// applyNodeHostAliases applies Clash hosts domain aliases to proxy server
// addresses before the generated outbound uses an explicit domain_resolver.
// A targeted domain_resolver bypasses DNS rule matching in sing-box, so the
// predefined CNAME rules alone cannot affect an outbound's own server lookup.
func applyNodeHostAliases(nodes []map[string]any, aliases []hostAlias) int {
	if len(nodes) == 0 || len(aliases) == 0 {
		return 0
	}
	targets := make(map[string]string, len(aliases))
	for _, alias := range aliases {
		targets[strings.ToLower(alias.Domain)] = alias.Target
	}

	applied := 0
	for _, node := range nodes {
		original := normalizeDNSName(asString(node["server"]))
		if original == "" {
			continue
		}
		current := original
		visited := map[string]bool{}
		for {
			key := strings.ToLower(current)
			if visited[key] {
				current = original
				break
			}
			visited[key] = true
			target, ok := targets[key]
			if !ok {
				break
			}
			current = target
		}
		if strings.EqualFold(current, original) {
			continue
		}
		node["server"] = current
		preserveTLSServerName(node, original)
		applied++
	}
	return applied
}

func preserveTLSServerName(node map[string]any, original string) {
	tls, ok := normalizeMap(node["tls"])
	if !ok || !parseBool(tls["enabled"]) || asString(tls["server_name"]) != "" {
		return
	}
	tls["server_name"] = original
}

func clashSourceOptions(root map[string]any) sourceOptions {
	var out sourceOptions
	if hosts, ok := normalizeMap(root["hosts"]); ok {
		out.Hosts, out.HostAliases, out.SkippedHosts = normalizeHostEntries(hosts)
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
				out.Hosts, out.HostAliases, out.SkippedHosts = normalizeHostEntries(predefined)
			}
		}
	}
	if out.FakeIP != nil {
		out.FakeIP.ExclusionRules = singBoxFakeIPExclusions(dns, fakeTag)
	}
	return out
}

// normalizeHostEntries splits Clash/Mihomo hosts into the two forms sing-box
// can safely represent. IP values belong to a hosts DNS server's predefined
// map; domain-to-domain values are emitted later as predefined CNAME answers.
// Passing a domain string to hosts.predefined is invalid in sing-box 1.12+.
func normalizeHostEntries(entries map[string]any) (map[string]any, []hostAlias, []string) {
	addresses := make(map[string]any)
	aliases := []hostAlias{}
	skipped := []string{}
	keys := make([]string, 0, len(entries))
	for key := range entries {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, rawDomain := range keys {
		domain := normalizeDNSName(rawDomain)
		values := stringSlice(entries[rawDomain])
		if domain == "" || len(values) == 0 {
			skipped = append(skipped, rawDomain)
			continue
		}

		ips := make([]string, 0, len(values))
		allIPs := true
		for _, value := range values {
			address, err := netip.ParseAddr(strings.TrimSpace(value))
			if err != nil {
				allIPs = false
				break
			}
			ips = append(ips, address.String())
		}
		if allIPs {
			if len(ips) == 1 {
				addresses[domain] = ips[0]
			} else {
				addresses[domain] = ips
			}
			continue
		}

		if len(values) == 1 {
			target := normalizeDNSName(values[0])
			if target != "" {
				aliases = append(aliases, hostAlias{Domain: domain, Target: target})
				continue
			}
		}
		skipped = append(skipped, rawDomain)
	}
	return addresses, aliases, skipped
}

func normalizeDNSName(value string) string {
	name := strings.TrimSuffix(strings.TrimSpace(value), ".")
	if name == "" || len(name) > 253 || strings.ContainsAny(name, "*+ /\\\t\r\n") {
		return ""
	}
	for _, label := range strings.Split(name, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return ""
		}
		for _, char := range label {
			if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') && (char < '0' || char > '9') && char != '-' && char != '_' {
				return ""
			}
		}
	}
	return name
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
