package converter

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/Trilives/sboxkit/internal/config"
	"github.com/Trilives/sboxkit/internal/paths"
	"gopkg.in/yaml.v3"
)

const (
	directGroupTag      = "Direct"
	geositeCNRuleSetTag = "geosite-cn"
	geoipCNRuleSetTag   = "geoip-cn"
	bootstrapDNSTag     = "dns-direct"
	remoteDNSTag        = "dns-proxy"
	bootstrapDNSDHCP    = "dhcp"
	generate204URL      = "https://www.gstatic.com/generate_204"
	defaultController   = "127.0.0.1:9090"
	lanController       = "0.0.0.0:9090"
)

var (
	reservedTags = map[string]bool{
		"Proxy": true, "AI": true, "Streaming": true, "Direct": true, "Auto": true,
		"SG-Auto": true, "SG-Fallback": true, "HK-Auto": true, "HK-Fallback": true,
		"Fallback": true, "DIRECT": true, "BLOCK": true, "DNS": true,
	}
	infoNodePrefixes  = []string{"traffic:", "expire:", "剩余流量", "过期时间"}
	sgExcludeKeywords = []string{"实验"}
	hkExcludeKeywords = []string{"实验"}
)

type Config struct {
	Log          map[string]any   `json:"log"`
	DNS          map[string]any   `json:"dns"`
	Inbounds     []map[string]any `json:"inbounds"`
	Outbounds    []map[string]any `json:"outbounds"`
	Route        Route            `json:"route"`
	Experimental Experimental     `json:"experimental"`
}

type Route struct {
	AutoDetectInterface bool             `json:"auto_detect_interface,omitempty"`
	DefaultResolver     map[string]any   `json:"default_domain_resolver,omitempty"`
	Rules               []map[string]any `json:"rules,omitempty"`
	RuleSet             []map[string]any `json:"rule_set,omitempty"`
	Final               string           `json:"final,omitempty"`
}

type Experimental struct {
	ClashAPI ClashAPI `json:"clash_api"`
}

type ClashAPI struct {
	ExternalController               string `json:"external_controller"`
	ExternalUI                       string `json:"external_ui,omitempty"`
	DefaultMode                      string `json:"default_mode"`
	AccessControlAllowPrivateNetwork bool   `json:"access_control_allow_private_network,omitempty"`
}

type Info map[string]any

func ClashToSingBox(yamlText string, cfg config.Config, p paths.Paths) (Config, Info, error) {
	var root map[string]any
	if err := yaml.Unmarshal([]byte(yamlText), &root); err != nil {
		return Config{}, nil, fmt.Errorf("parse clash YAML: %w", err)
	}

	rawProxies, ok := root["proxies"].([]any)
	if !ok || len(rawProxies) == 0 {
		return Config{}, nil, errors.New("subscription is missing non-empty proxies list")
	}

	used := cloneReservedTags()
	converted := make([]map[string]any, 0, len(rawProxies))
	skipped := map[string]int{}
	for _, raw := range rawProxies {
		proxy, ok := normalizeMap(raw)
		if !ok {
			skipped["proxy is not a mapping"]++
			continue
		}
		outbound, reason := convertProxy(proxy, used)
		if reason != "" {
			skipped[reason]++
			continue
		}
		converted = append(converted, outbound)
	}
	if len(converted) == 0 {
		return Config{}, nil, errors.New("no nodes converted successfully")
	}

	result, info, err := BuildSingBoxConfig(converted, cfg, p)
	if err != nil {
		return Config{}, nil, err
	}
	info["total"] = len(rawProxies)
	info["converted"] = len(converted)
	info["skipped"] = skipped
	return result, info, nil
}

func SingBoxDirect(raw string, cfg config.Config, p paths.Paths, customize bool) (Config, Info, error) {
	var doc map[string]any
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return Config{}, nil, fmt.Errorf("parse sing-box JSON: %w", err)
	}
	if !customize {
		ensureClashAPI(doc, cfg, p)
		var result Config
		data, _ := json.Marshal(doc)
		if err := json.Unmarshal(data, &result); err != nil {
			return Config{}, nil, fmt.Errorf("decode sing-box passthrough: %w", err)
		}
		return result, Info{"mode": "passthrough"}, nil
	}

	rawOutbounds, _ := doc["outbounds"].([]any)
	nodes := make([]map[string]any, 0, len(rawOutbounds))
	for _, rawOutbound := range rawOutbounds {
		outbound, ok := normalizeMap(rawOutbound)
		if !ok {
			continue
		}
		typ, _ := outbound["type"].(string)
		tag, _ := outbound["tag"].(string)
		if tag == "" || isGeneratedOutboundType(typ) {
			continue
		}
		nodes = append(nodes, outbound)
	}
	if len(nodes) == 0 {
		return Config{}, nil, errors.New("sing-box config has no reusable node outbounds")
	}
	result, info, err := BuildSingBoxConfig(nodes, cfg, p)
	if err != nil {
		return Config{}, nil, err
	}
	info["mode"] = "customized"
	return result, info, nil
}

func BuildSingBoxConfig(nodes []map[string]any, cfg config.Config, p paths.Paths) (Config, Info, error) {
	outbounds, info := buildOutbounds(nodes, cfg)
	final := cfg.DefaultOutbound
	if (final == "SG-Auto" || final == "SG-Fallback") && !boolInfo(info, "has_sg_auto") {
		final = "Proxy"
	}
	if (final == "HK-Auto" || final == "HK-Fallback") && !boolInfo(info, "has_hk_auto") {
		final = "Proxy"
	}
	result := Config{
		Log:          map[string]any{"level": "warning"},
		DNS:          buildDNS(cfg, p),
		Inbounds:     buildInbounds(cfg),
		Outbounds:    outbounds,
		Route:        buildRoute(final, cfg, p),
		Experimental: buildExperimental(cfg, p),
	}
	if err := Validate(result); err != nil {
		return Config{}, nil, err
	}
	return result, info, nil
}

func Validate(c Config) error {
	if len(c.Inbounds) == 0 {
		return errors.New("inbounds must be non-empty")
	}
	if len(c.Outbounds) == 0 {
		return errors.New("outbounds must be non-empty")
	}
	if c.Route.Final == "" {
		return errors.New("route final is required")
	}
	if c.Experimental.ClashAPI.ExternalController == "" {
		return errors.New("clash API controller is required")
	}

	tags := map[string]bool{}
	for _, outbound := range c.Outbounds {
		typ, _ := outbound["type"].(string)
		tag, _ := outbound["tag"].(string)
		if typ == "" || tag == "" {
			return errors.New("each outbound must have type and tag")
		}
		if tags[tag] {
			return fmt.Errorf("duplicate outbound tag %q", tag)
		}
		tags[tag] = true
	}
	for _, outbound := range c.Outbounds {
		typ, _ := outbound["type"].(string)
		if typ != "selector" && typ != "urltest" {
			continue
		}
		tag, _ := outbound["tag"].(string)
		refs := stringSlice(outbound["outbounds"])
		if len(refs) == 0 {
			return fmt.Errorf("%s outbounds must be non-empty", tag)
		}
		for _, ref := range refs {
			if !tags[ref] {
				return fmt.Errorf("%s references missing outbound %s", tag, ref)
			}
		}
	}
	if !tags[c.Route.Final] {
		return fmt.Errorf("route final references missing outbound %s", c.Route.Final)
	}
	return nil
}

func convertProxy(proxy map[string]any, used map[string]bool) (map[string]any, string) {
	name := strings.TrimSpace(asString(proxy["name"]))
	if name == "" {
		return nil, "missing name"
	}
	tag := makeSafeTag(name, used)
	typ := strings.ToLower(strings.TrimSpace(asString(proxy["type"])))

	var (
		out map[string]any
		err error
	)
	switch typ {
	case "anytls":
		out, err = convertAnyTLS(proxy, tag)
	case "trojan":
		out, err = convertTrojan(proxy, tag)
	case "ss", "shadowsocks":
		out, err = convertShadowsocks(proxy, tag)
	case "vmess":
		out, err = convertVMess(proxy, tag)
	case "vless":
		out, err = convertVLess(proxy, tag)
	case "hysteria2", "hy2":
		out, err = convertHysteria2(proxy, tag)
	case "tuic":
		out, err = convertTUIC(proxy, tag)
	case "socks", "socks5":
		out, err = convertSocks(proxy, tag)
	case "http":
		out, err = convertHTTP(proxy, tag)
	default:
		delete(used, tag)
		if typ == "" {
			typ = "unknown"
		}
		return nil, "unsupported type " + typ
	}
	if err != nil {
		delete(used, tag)
		return nil, err.Error()
	}
	return out, ""
}

func convertAnyTLS(proxy map[string]any, tag string) (map[string]any, error) {
	if err := requireFields(proxy, "server", "password"); err != nil {
		return nil, err
	}
	out, err := baseOutbound(proxy, tag, "anytls")
	if err != nil {
		return nil, err
	}
	out["password"] = asString(proxy["password"])
	out["tls"] = tlsConfig(proxy, true)
	return out, nil
}

func convertTrojan(proxy map[string]any, tag string) (map[string]any, error) {
	if err := requireFields(proxy, "server", "password"); err != nil {
		return nil, err
	}
	out, err := baseOutbound(proxy, tag, "trojan")
	if err != nil {
		return nil, err
	}
	out["password"] = asString(proxy["password"])
	if tls := tlsConfig(proxy, true); len(tls) > 0 {
		out["tls"] = tls
	}
	return out, addSupportedTransport(proxy, out)
}

func convertShadowsocks(proxy map[string]any, tag string) (map[string]any, error) {
	method := firstString(proxy, "cipher", "method")
	if method == "" {
		return nil, errors.New("missing cipher")
	}
	if err := requireFields(proxy, "server", "password"); err != nil {
		return nil, err
	}
	out, err := baseOutbound(proxy, tag, "shadowsocks")
	if err != nil {
		return nil, err
	}
	out["method"] = method
	out["password"] = asString(proxy["password"])
	if _, ok := proxy["udp"]; ok && !parseBool(proxy["udp"]) {
		out["network"] = "tcp"
	}

	plugin := strings.ToLower(asString(proxy["plugin"]))
	if plugin == "" {
		return out, nil
	}
	opts, _ := normalizeMap(proxy["plugin-opts"])
	if plugin == "v2ray-plugin" {
		mode := strings.ToLower(asString(defaultValue(opts["mode"], "websocket")))
		if mode != "websocket" && mode != "quic" {
			return nil, fmt.Errorf("unsupported shadowsocks v2ray-plugin mode %s", mode)
		}
		pluginOpts := []string{"mode=" + mode}
		if parseBool(opts["tls"]) {
			pluginOpts = append(pluginOpts, "tls")
		}
		for _, key := range []string{"host", "path"} {
			if value := asString(opts[key]); value != "" {
				pluginOpts = append(pluginOpts, key+"="+value)
			}
		}
		out["plugin"] = "v2ray-plugin"
		out["plugin_opts"] = strings.Join(pluginOpts, ";")
		return out, nil
	}
	if plugin != "obfs" {
		return nil, fmt.Errorf("unsupported shadowsocks plugin %s", plugin)
	}
	mode := strings.ToLower(asString(defaultValue(opts["mode"], "http")))
	if mode != "http" && mode != "tls" {
		return nil, fmt.Errorf("unsupported shadowsocks obfs mode %s", mode)
	}
	pluginOpts := "obfs=" + mode
	if host := asString(opts["host"]); host != "" {
		pluginOpts += ";obfs-host=" + host
	}
	out["plugin"] = "obfs-local"
	out["plugin_opts"] = pluginOpts
	return out, nil
}

func convertVMess(proxy map[string]any, tag string) (map[string]any, error) {
	if err := requireFields(proxy, "server", "uuid"); err != nil {
		return nil, err
	}
	out, err := baseOutbound(proxy, tag, "vmess")
	if err != nil {
		return nil, err
	}
	out["uuid"] = asString(proxy["uuid"])
	out["security"] = asString(defaultValue(proxy["cipher"], "auto"))
	out["alter_id"] = asInt(defaultValue(firstValue(proxy, "alterId", "alter-id"), 0))
	if tls := tlsConfig(proxy, false); len(tls) > 0 {
		out["tls"] = tls
	}
	return out, addSupportedTransport(proxy, out)
}

func convertVLess(proxy map[string]any, tag string) (map[string]any, error) {
	if err := requireFields(proxy, "server", "uuid"); err != nil {
		return nil, err
	}
	out, err := baseOutbound(proxy, tag, "vless")
	if err != nil {
		return nil, err
	}
	out["uuid"] = asString(proxy["uuid"])
	if flow := asString(proxy["flow"]); flow != "" {
		out["flow"] = flow
	}
	if tls := tlsConfig(proxy, false); len(tls) > 0 {
		out["tls"] = tls
	}
	return out, addSupportedTransport(proxy, out)
}

func convertHysteria2(proxy map[string]any, tag string) (map[string]any, error) {
	if err := requireFields(proxy, "server", "password"); err != nil {
		return nil, err
	}
	out, err := baseOutbound(proxy, tag, "hysteria2")
	if err != nil {
		return nil, err
	}
	out["password"] = asString(proxy["password"])
	if tls := tlsConfig(proxy, true); len(tls) > 0 {
		out["tls"] = tls
	}
	return out, nil
}

func convertTUIC(proxy map[string]any, tag string) (map[string]any, error) {
	if err := requireFields(proxy, "server", "uuid", "password"); err != nil {
		return nil, err
	}
	out, err := baseOutbound(proxy, tag, "tuic")
	if err != nil {
		return nil, err
	}
	out["uuid"] = asString(proxy["uuid"])
	out["password"] = asString(proxy["password"])
	if cc := firstString(proxy, "congestion-controller", "congestion_control"); cc != "" {
		out["congestion_control"] = cc
	}
	if tls := tlsConfig(proxy, true); len(tls) > 0 {
		out["tls"] = tls
	}
	return out, nil
}

func convertSocks(proxy map[string]any, tag string) (map[string]any, error) {
	if err := requireFields(proxy, "server"); err != nil {
		return nil, err
	}
	out, err := baseOutbound(proxy, tag, "socks")
	if err != nil {
		return nil, err
	}
	addAuth(proxy, out)
	return out, nil
}

func convertHTTP(proxy map[string]any, tag string) (map[string]any, error) {
	if err := requireFields(proxy, "server"); err != nil {
		return nil, err
	}
	out, err := baseOutbound(proxy, tag, "http")
	if err != nil {
		return nil, err
	}
	addAuth(proxy, out)
	if tls := tlsConfig(proxy, false); len(tls) > 0 {
		out["tls"] = tls
	}
	return out, nil
}

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
		{"ip_cidr": cfg.RouteExcludeIPCidrs, "action": "route", "outbound": "DIRECT"},
		{"action": "sniff"},
		{"protocol": "dns", "action": "hijack-dns"},
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
		DefaultMode:                      "rule",
		AccessControlAllowPrivateNetwork: cfg.LanPanel,
	}
	if cfg.LanPanel {
		api.ExternalUI = p.UIDir
	}
	return Experimental{ClashAPI: api}
}

func tlsConfig(proxy map[string]any, defaultEnabled bool) map[string]any {
	enabled := parseBool(defaultValue(proxy["tls"], defaultEnabled))
	serverName := firstString(proxy, "servername", "server_name", "sni")
	insecure, hasInsecure := proxy["skip-cert-verify"]
	alpnValue := proxy["alpn"]
	fingerprint := firstString(proxy, "client-fingerprint", "fingerprint")
	certificatePath := firstString(proxy, "ca", "certificate_path")
	certificate := firstString(proxy, "ca-str", "certificate")
	clientCertificatePath := firstString(proxy, "client-cert", "client_certificate_path")
	clientCertificate := firstString(proxy, "client-cert-str", "client_certificate")
	clientKeyPath := firstString(proxy, "client-key", "client_key_path")
	clientKey := firstString(proxy, "client-key-str", "client_key")

	if !enabled && serverName == "" && !hasInsecure && alpnValue == nil && fingerprint == "" &&
		certificatePath == "" && certificate == "" && clientCertificatePath == "" &&
		clientCertificate == "" && clientKeyPath == "" && clientKey == "" {
		return nil
	}

	tls := map[string]any{"enabled": enabled || serverName != "" || alpnValue != nil || fingerprint != "" || certificatePath != "" || certificate != ""}
	if serverName != "" {
		tls["server_name"] = serverName
	}
	if hasInsecure {
		tls["insecure"] = parseBool(insecure)
	}
	if alpn := stringSlice(alpnValue); len(alpn) > 0 {
		tls["alpn"] = alpn
	} else if text := asString(alpnValue); text != "" {
		tls["alpn"] = splitComma(text)
	}
	if fingerprint != "" {
		tls["utls"] = map[string]any{"enabled": true, "fingerprint": fingerprint}
	}
	addIf(tls, "certificate_path", certificatePath)
	addIf(tls, "certificate", certificate)
	addIf(tls, "client_certificate_path", clientCertificatePath)
	addIf(tls, "client_certificate", clientCertificate)
	addIf(tls, "client_key_path", clientKeyPath)
	addIf(tls, "client_key", clientKey)
	return tls
}

func addSupportedTransport(proxy map[string]any, outbound map[string]any) error {
	network := strings.ToLower(asString(proxy["network"]))
	if network == "" || network == "tcp" || network == "raw" {
		return nil
	}
	switch network {
	case "ws", "websocket":
		opts, _ := normalizeMap(proxy["ws-opts"])
		transport := map[string]any{"type": "ws"}
		addIf(transport, "path", asString(opts["path"]))
		if headers, ok := normalizeMap(opts["headers"]); ok && len(headers) > 0 {
			clean := map[string]string{}
			for k, v := range headers {
				clean[k] = asString(v)
			}
			transport["headers"] = clean
		}
		outbound["transport"] = transport
	case "grpc":
		opts, _ := normalizeMap(proxy["grpc-opts"])
		transport := map[string]any{"type": "grpc"}
		addIf(transport, "service_name", firstString(opts, "grpc-service-name", "serviceName", "service_name"))
		outbound["transport"] = transport
	case "httpupgrade", "http-upgrade":
		opts, _ := normalizeMap(proxy["httpupgrade-opts"])
		transport := map[string]any{"type": "httpupgrade"}
		addIf(transport, "path", asString(opts["path"]))
		if hosts := stringSlice(opts["host"]); len(hosts) > 0 {
			transport["host"] = hosts
		} else if host := asString(opts["host"]); host != "" {
			transport["host"] = []string{host}
		}
		outbound["transport"] = transport
	default:
		return fmt.Errorf("unsupported transport %s", network)
	}
	return nil
}

func baseOutbound(proxy map[string]any, tag string, typ string) (map[string]any, error) {
	server := asString(proxy["server"])
	port := normalizePort(firstValue(proxy, "server_port", "port"))
	if server == "" {
		return nil, errors.New("missing server")
	}
	if port == 0 {
		return nil, errors.New("missing or invalid port")
	}
	return map[string]any{"type": typ, "tag": tag, "server": server, "server_port": port}, nil
}

func requireFields(proxy map[string]any, fields ...string) error {
	for _, field := range fields {
		if asString(proxy[field]) == "" {
			return fmt.Errorf("missing %s", field)
		}
	}
	if normalizePort(firstValue(proxy, "server_port", "port")) == 0 {
		return errors.New("missing or invalid port")
	}
	return nil
}

func makeSafeTag(name string, used map[string]bool) string {
	tag := strings.TrimSpace(name)
	if tag == "" {
		tag = "node"
	}
	base := tag
	index := 1
	for used[tag] {
		tag = fmt.Sprintf("%s-%d", base, index)
		index++
	}
	used[tag] = true
	return tag
}

func normalizePort(value any) int {
	port := asInt(value)
	if port < 1 || port > 65535 {
		return 0
	}
	return port
}

func normalizeMap(value any) (map[string]any, bool) {
	switch typed := value.(type) {
	case map[string]any:
		return typed, true
	case map[any]any:
		out := make(map[string]any, len(typed))
		for k, v := range typed {
			out[asString(k)] = v
		}
		return out, true
	default:
		return nil, false
	}
}

func asString(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case float64:
		if typed == float64(int64(typed)) {
			return strconv.FormatInt(int64(typed), 10)
		}
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(typed)
	default:
		return fmt.Sprint(typed)
	}
}

func asInt(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case string:
		i, _ := strconv.Atoi(strings.TrimSpace(typed))
		return i
	default:
		return 0
	}
}

func parseBool(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "1", "true", "yes", "on", "tls":
			return true
		default:
			return false
		}
	default:
		return value != nil
	}
}

func firstValue(m map[string]any, keys ...string) any {
	for _, key := range keys {
		if value, ok := m[key]; ok && value != nil && asString(value) != "" {
			return value
		}
	}
	return nil
}

func firstString(m map[string]any, keys ...string) string {
	return asString(firstValue(m, keys...))
}

func defaultValue(value any, fallback any) any {
	if value == nil || asString(value) == "" {
		return fallback
	}
	return value
}

func stringSlice(value any) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := asString(item); text != "" {
				out = append(out, text)
			}
		}
		return out
	case string:
		if typed == "" {
			return nil
		}
		return []string{typed}
	default:
		return nil
	}
}

func splitComma(text string) []string {
	parts := strings.Split(text, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func addIf(m map[string]any, key string, value string) {
	if value != "" {
		m[key] = value
	}
}

func addAuth(proxy map[string]any, out map[string]any) {
	if username := asString(proxy["username"]); username != "" {
		out["username"] = username
	}
	if password := asString(proxy["password"]); password != "" {
		out["password"] = password
	}
}

func cloneReservedTags() map[string]bool {
	out := make(map[string]bool, len(reservedTags))
	for key, value := range reservedTags {
		out[key] = value
	}
	return out
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

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;?]*[A-Za-z]`)

func stripANSI(text string) string {
	return ansiPattern.ReplaceAllString(text, "")
}
