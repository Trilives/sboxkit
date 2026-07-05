package converter

import (
	"os"
	"strings"

	"github.com/Trilives/sboxkit/internal/config"
	"github.com/Trilives/sboxkit/internal/paths"
)

const (
	directGroupTag      = "Direct"
	geositeCNRuleSetTag = "geosite-cn"
	geoipCNRuleSetTag   = "geoip-cn"
	bootstrapDNSTag     = "dns-direct"
	remoteDNSTag        = "dns-proxy"
	bootstrapDNSDHCP    = "dhcp"
	generate204URL      = "https://www.google.com/generate_204"
	defaultController   = "127.0.0.1:9090"
	lanController       = "0.0.0.0:9090"
)

var (
	infoNodePrefixes  = []string{"traffic:", "expire:", "剩余流量", "过期时间"}
	sgExcludeKeywords = []string{"实验"}
	hkExcludeKeywords = []string{"实验"}
)

func buildInbounds(cfg config.Config) []map[string]any {
	listen := "127.0.0.1"
	if cfg.LanProxy {
		listen = "0.0.0.0"
	}
	mixed := map[string]any{"type": "mixed", "tag": "mixed-in", "listen": listen, "listen_port": 7890}
	if !cfg.EnableTun {
		return []map[string]any{mixed}
	}
	tun := map[string]any{
		"type": "tun", "tag": "tun-in", "interface_name": "singbox",
		"address": []string{"172.19.0.1/30"}, "mtu": 1400, "auto_route": true,
		"strict_route": true, "route_exclude_address": cfg.RouteExcludeIPCidrs, "stack": "gvisor",
	}
	if len(cfg.TunExcludeUIDs) > 0 {
		tun["exclude_uid"] = cfg.TunExcludeUIDs
	}
	return []map[string]any{tun, mixed}
}

func buildDNS(cfg config.Config, p paths.Paths) map[string]any {
	rules := []map[string]any{{"domain": cfg.LocalBypassDomains, "action": "route", "server": bootstrapDNSTag}}
	if len(cfg.DirectDomainSuffixes) > 0 {
		rules = append(rules, map[string]any{"domain_suffix": cfg.DirectDomainSuffixes, "action": "route", "server": bootstrapDNSTag})
	}
	rules = append(rules,
		map[string]any{"domain_suffix": cfg.AIDomainSuffixes, "action": "route", "server": remoteDNSTag},
		map[string]any{"domain_suffix": cfg.StreamingDomainSuffixes, "action": "route", "server": remoteDNSTag},
	)
	if localRuleSetsReady(p) {
		rules = append(rules,
			map[string]any{"rule_set": geositeCNRuleSetTag, "action": "route", "server": bootstrapDNSTag},
			map[string]any{"rule_set": geoipCNRuleSetTag, "action": "route", "server": bootstrapDNSTag},
		)
	}

	bootstrap := map[string]any{"type": "udp", "tag": bootstrapDNSTag, "server": cfg.BootstrapDNSServer, "server_port": cfg.BootstrapDNSPort, "detour": "DIRECT"}
	if strings.EqualFold(cfg.BootstrapDNSServer, bootstrapDNSDHCP) {
		bootstrap = map[string]any{"type": "dhcp", "tag": bootstrapDNSTag, "detour": "DIRECT"}
	}

	return map[string]any{
		"servers": []map[string]any{
			bootstrap,
			{"type": "udp", "tag": "dns-dnspod", "server": "119.29.29.29", "server_port": 53, "detour": "DIRECT"},
			{"type": "https", "tag": remoteDNSTag, "server": "1.1.1.1", "server_port": 443, "path": "/dns-query", "tls": map[string]any{"server_name": "cloudflare-dns.com"}, "detour": "Proxy"},
		},
		"rules": rules, "final": remoteDNSTag, "strategy": "prefer_ipv4", "cache_capacity": 4096,
	}
}

func buildRoute(final string, cfg config.Config, p paths.Paths) Route {
	rules := []map[string]any{
		{"process_name": cfg.BypassProcessNames, "action": "route", "outbound": "DIRECT"},
		{"domain": cfg.LocalBypassDomains, "action": "route", "outbound": "DIRECT"},
		// sniff + hijack-dns 必须排在 route_exclude_ip_cidrs 的 ip_cidr 直连规则之前：
		// sing-box auto_route 会把系统 DNS 指向 TUN 自身子网内的一个合成地址
		// （如 172.19.0.1/30 场景下的 172.19.0.2），若该地址恰好落在用户配置的
		// 排除网段内（典型如 172.16.0.0/12 这类大范围私网段），DNS 查询会在到达
		// hijack-dns 之前就被 ip_cidr 规则直接判给 DIRECT，实际拨号一个只存在于
		// TUN 虚拟网段内、外部不可达的地址，导致全局 DNS 解析全部超时、断网。
		// hijack-dns 按协议匹配、与目标地址无关，提前到 ip_cidr 规则之前可以
		// 保证所有 DNS 流量总是被内部接管，不受排除网段配置影响。
		{"action": "sniff"},
		{"protocol": "dns", "action": "hijack-dns"},
		{"ip_cidr": cfg.RouteExcludeIPCidrs, "action": "route", "outbound": "DIRECT"},
		{"ip_is_private": true, "action": "route", "outbound": "DIRECT"},
	}
	if len(cfg.DirectDomainSuffixes) > 0 {
		rules = append(rules, map[string]any{"domain_suffix": cfg.DirectDomainSuffixes, "action": "route", "outbound": directGroupTag})
	}
	rules = append(rules,
		map[string]any{"domain_suffix": cfg.AIDomainSuffixes, "action": "route", "outbound": "AI"},
		map[string]any{"domain_suffix": cfg.StreamingDomainSuffixes, "action": "route", "outbound": "Streaming"},
	)
	ruleSets := []map[string]any{}
	if localRuleSetsReady(p) {
		rules = append(rules, map[string]any{"rule_set": []string{geositeCNRuleSetTag, geoipCNRuleSetTag}, "action": "route", "outbound": "DIRECT"})
		ruleSets = []map[string]any{
			{"type": "local", "tag": geositeCNRuleSetTag, "format": "binary", "path": p.GeositeCN},
			{"type": "local", "tag": geoipCNRuleSetTag, "format": "binary", "path": p.GeoIPCN},
		}
	}
	return Route{
		AutoDetectInterface: true,
		DefaultResolver:     bootstrapResolver(),
		Rules:               rules,
		RuleSet:             ruleSets,
		Final:               final,
	}
}

func localRuleSetsReady(p paths.Paths) bool {
	for _, file := range []string{p.GeositeCN, p.GeoIPCN} {
		info, err := os.Stat(file)
		if err != nil || info.IsDir() || info.Size() == 0 {
			return false
		}
	}
	return true
}

func buildOutbounds(nodes []map[string]any, cfg config.Config) ([]map[string]any, Info) {
	nodeTags := make([]string, 0, len(nodes))
	for _, node := range nodes {
		tag, _ := node["tag"].(string)
		if tag != "" {
			nodeTags = append(nodeTags, tag)
		}
	}
	selectable := make([]string, 0, len(nodeTags))
	for _, tag := range nodeTags {
		if !isInformationalNode(tag) {
			selectable = append(selectable, tag)
		}
	}

	sgTags := preferredTags(selectable, cfg.PreferKeywords, sgExcludeKeywords, cfg.GenerateSGGroups)
	hkTags := preferredTags(selectable, cfg.HKPreferKeywords, hkExcludeKeywords, cfg.GenerateHKGroups)
	regionGroups := []string{}

	outbounds := append([]map[string]any{}, nodes...)
	if len(sgTags) > 0 {
		regionGroups = append(regionGroups, "SG-Auto", "SG-Fallback")
		outbounds = append(outbounds,
			urlTestOutbound("SG-Auto", sgTags),
			map[string]any{"type": "selector", "tag": "SG-Fallback", "outbounds": sgTags, "default": sgTags[0]},
		)
	}
	if len(hkTags) > 0 {
		regionGroups = append(regionGroups, "HK-Auto", "HK-Fallback")
		outbounds = append(outbounds,
			urlTestOutbound("HK-Auto", hkTags),
			map[string]any{"type": "selector", "tag": "HK-Fallback", "outbounds": hkTags, "default": hkTags[0]},
		)
	}

	outbounds = append(outbounds,
		map[string]any{"type": "selector", "tag": "AI", "outbounds": appendMany([]string{"Proxy"}, regionGroups, []string{"Auto", "DIRECT"}), "default": "Proxy"},
		map[string]any{"type": "selector", "tag": "Streaming", "outbounds": appendMany([]string{"Proxy"}, regionGroups, []string{"Auto", "DIRECT"}), "default": "Proxy"},
	)

	proxyDefault := "Auto"
	if len(sgTags) > 0 {
		proxyDefault = "SG-Auto"
	} else if len(hkTags) > 0 {
		proxyDefault = "HK-Auto"
	}
	outbounds = append(outbounds,
		map[string]any{"type": "selector", "tag": "Proxy", "outbounds": appendMany(regionGroups, []string{"Auto"}, selectable, []string{"DIRECT"}), "default": proxyDefault},
		urlTestOutbound("Auto", selectable),
		map[string]any{"type": "direct", "tag": "DIRECT", "domain_resolver": bootstrapResolver()},
		map[string]any{"type": "block", "tag": "BLOCK"},
		map[string]any{"type": "selector", "tag": "Fallback", "outbounds": []string{"Proxy", "Auto", "DIRECT"}, "default": "Proxy"},
	)
	if len(cfg.DirectDomainSuffixes) > 0 {
		outbounds = append(outbounds, map[string]any{"type": "selector", "tag": directGroupTag, "outbounds": []string{"DIRECT", "Proxy", "Auto"}, "default": "DIRECT"})
	}

	return outbounds, Info{
		"has_sg_auto":      len(sgTags) > 0,
		"sg_count":         len(sgTags),
		"has_hk_auto":      len(hkTags) > 0,
		"hk_count":         len(hkTags),
		"auto_count":       len(selectable),
		"proxy_default":    proxyDefault,
		"has_direct_group": len(cfg.DirectDomainSuffixes) > 0,
		"direct_count":     len(cfg.DirectDomainSuffixes),
	}
}

func buildExperimental(cfg config.Config, p paths.Paths) Experimental {
	controller := defaultController
	if cfg.LanPanel {
		controller = lanController
	}
	api := ClashAPI{
		ExternalController:               controller,
		ExternalUI:                       p.UI,
		DefaultMode:                      "rule",
		AccessControlAllowPrivateNetwork: cfg.LanPanel,
	}
	return Experimental{ClashAPI: api}
}

func ensureClashAPI(doc map[string]any, cfg config.Config, p paths.Paths) {
	experimental, ok := normalizeMap(doc["experimental"])
	if !ok {
		experimental = map[string]any{}
		doc["experimental"] = experimental
	}
	if _, ok := normalizeMap(experimental["clash_api"]); ok {
		return
	}
	api := buildExperimental(cfg, p).ClashAPI
	experimental["clash_api"] = map[string]any{
		"external_controller": api.ExternalController,
		"default_mode":        api.DefaultMode,
	}
	if api.ExternalUI != "" {
		experimental["clash_api"].(map[string]any)["external_ui"] = api.ExternalUI
	}
	if api.AccessControlAllowPrivateNetwork {
		experimental["clash_api"].(map[string]any)["access_control_allow_private_network"] = true
	}
}

func isGeneratedOutboundType(typ string) bool {
	switch typ {
	case "selector", "urltest", "direct", "block", "dns":
		return true
	default:
		return false
	}
}

func preferredTags(tags []string, keywords []string, excludes []string, enabled bool) []string {
	if !enabled {
		return nil
	}
	out := []string{}
	for _, tag := range tags {
		if containsKeyword(tag, keywords) && !containsKeyword(tag, excludes) {
			out = append(out, tag)
		}
	}
	return out
}

func containsKeyword(text string, keywords []string) bool {
	lowered := strings.ToLower(text)
	for _, keyword := range keywords {
		if strings.Contains(lowered, strings.ToLower(keyword)) {
			return true
		}
	}
	return false
}

func isInformationalNode(tag string) bool {
	lowered := strings.ToLower(strings.TrimSpace(tag))
	for _, prefix := range infoNodePrefixes {
		if strings.HasPrefix(lowered, strings.ToLower(prefix)) {
			return true
		}
	}
	return false
}

func urlTestOutbound(tag string, outbounds []string) map[string]any {
	return map[string]any{"type": "urltest", "tag": tag, "outbounds": outbounds, "url": generate204URL, "interval": "5m", "tolerance": 50}
}

func appendMany(parts ...[]string) []string {
	size := 0
	for _, part := range parts {
		size += len(part)
	}
	out := make([]string, 0, size)
	for _, part := range parts {
		out = append(out, part...)
	}
	return out
}

func bootstrapResolver() map[string]any {
	return map[string]any{"server": bootstrapDNSTag, "strategy": "prefer_ipv4"}
}

func boolInfo(info Info, key string) bool {
	value, _ := info[key].(bool)
	return value
}
