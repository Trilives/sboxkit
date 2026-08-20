package config

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/Trilives/sboxkit/internal/i18n"
)

const (
	DefaultMixedPort                 = 7890
	DefaultBootstrapDNSType          = "tcp"
	DefaultBootstrapDNSServer        = "223.5.5.5"
	DefaultBootstrapDNSPath          = "/dns-query"
	DefaultBootstrapDNSTLSServerName = "dns.alidns.com"
)

var supportedBootstrapDNSTypes = map[string]struct{}{
	"udp":   {},
	"tcp":   {},
	"https": {},
	"dhcp":  {},
}

// EffectiveMixedPort keeps old customize.json files and invalid hand-edited
// values safe: a missing/out-of-range port always falls back to the historical
// default.
func EffectiveMixedPort(cfg Config) int {
	if cfg.MixedPort < 1 || cfg.MixedPort > 65535 {
		return DefaultMixedPort
	}
	return cfg.MixedPort
}

// ParsePort validates an interactive mixed-port value.
func ParsePort(value string) (int, error) {
	port, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("%s", i18n.T("端口必须是 1-65535 之间的整数"))
	}
	return port, nil
}

// ParseBootstrapDNSType validates and normalizes the modern sing-box DNS
// transports exposed by customize.json.
func ParseBootstrapDNSType(value string) (string, error) {
	dnsType := strings.ToLower(strings.TrimSpace(value))
	if _, ok := supportedBootstrapDNSTypes[dnsType]; !ok {
		return "", fmt.Errorf("%s", i18n.T("引导 DNS 类型必须是 udp、tcp、https 或 dhcp"))
	}
	return dnsType, nil
}

// EffectiveBootstrapDNSType keeps missing or hand-edited configuration safe
// by falling back to direct TCP. The historical server value "dhcp" remains
// supported for old customize.json files.
func EffectiveBootstrapDNSType(cfg Config) string {
	if strings.EqualFold(strings.TrimSpace(cfg.BootstrapDNSServer), "dhcp") {
		return "dhcp"
	}
	dnsType, err := ParseBootstrapDNSType(cfg.BootstrapDNSType)
	if err != nil {
		return DefaultBootstrapDNSType
	}
	return dnsType
}

// NormalizeBootstrapDNS returns a validated copy suitable for persistence or
// conversion. Bootstrap transports deliberately accept IP literals only, so
// resolving a DNS server can never depend on the DNS server itself.
func NormalizeBootstrapDNS(cfg Config) Config {
	normalized := cfg
	normalized.BootstrapDNSType = EffectiveBootstrapDNSType(normalized)
	if normalized.BootstrapDNSType == "dhcp" {
		return normalized
	}
	if net.ParseIP(strings.TrimSpace(normalized.BootstrapDNSServer)) == nil {
		normalized.BootstrapDNSServer = DefaultBootstrapDNSServer
		normalized.BootstrapDNSTLSServerName = DefaultBootstrapDNSTLSServerName
	} else {
		normalized.BootstrapDNSServer = strings.TrimSpace(normalized.BootstrapDNSServer)
	}
	if normalized.BootstrapDNSPort < 1 || normalized.BootstrapDNSPort > 65535 {
		if normalized.BootstrapDNSType == "https" {
			normalized.BootstrapDNSPort = 443
		} else {
			normalized.BootstrapDNSPort = 53
		}
	}
	path := strings.TrimSpace(normalized.BootstrapDNSPath)
	if path == "" || !strings.HasPrefix(path, "/") {
		path = DefaultBootstrapDNSPath
	}
	normalized.BootstrapDNSPath = path
	normalized.BootstrapDNSTLSServerName = strings.TrimSpace(normalized.BootstrapDNSTLSServerName)
	return normalized
}
