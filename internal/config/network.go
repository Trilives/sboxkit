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
	DefaultBootstrapDNSRelayServer   = "127.0.0.1"
	DefaultBootstrapDNSRelayPort     = 6450
	MaxBootstrapDNSUpstreams         = 8
)

// BootstrapDNSUpstream describes one ordered, direct bootstrap resolver used
// by the local loopback relay. Server must be an IP literal so establishing a
// connection to an upstream never recursively depends on DNS.
type BootstrapDNSUpstream struct {
	Provider      string `json:"provider"`
	Type          string `json:"type"`
	Server        string `json:"server"`
	Port          int    `json:"port"`
	Path          string `json:"path,omitempty"`
	TLSServerName string `json:"tls_server_name,omitempty"`
}

// DefaultBootstrapDNSUpstreams returns fresh storage on every call. The two
// defaults intentionally use different operators and transports so failure of
// one endpoint or one transport does not remove bootstrap resolution entirely.
func DefaultBootstrapDNSUpstreams() []BootstrapDNSUpstream {
	return []BootstrapDNSUpstream{
		{
			Provider: "AliDNS",
			Type:     "tcp",
			Server:   DefaultBootstrapDNSServer,
			Port:     53,
		},
		{
			Provider:      "Cloudflare",
			Type:          "https",
			Server:        "1.1.1.1",
			Port:          443,
			Path:          DefaultBootstrapDNSPath,
			TLSServerName: "cloudflare-dns.com",
		},
	}
}

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
	if normalized.BootstrapDNSType != "dhcp" {
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
		if normalized.BootstrapDNSType == "https" && normalized.BootstrapDNSTLSServerName == "" {
			normalized.BootstrapDNSTLSServerName = DefaultBootstrapDNSTLSServerName
		}
	}
	normalized.BootstrapDNSUpstreams = normalizeBootstrapDNSUpstreams(normalized.BootstrapDNSUpstreams)
	return normalized
}

func normalizeBootstrapDNSUpstreams(upstreams []BootstrapDNSUpstream) []BootstrapDNSUpstream {
	normalized := make([]BootstrapDNSUpstream, 0, len(upstreams)+2)
	seen := make(map[string]struct{}, len(upstreams)+2)
	for _, candidate := range upstreams {
		upstream, ok := normalizeBootstrapDNSUpstream(candidate)
		if !ok {
			continue
		}
		endpoint := bootstrapDNSUpstreamEndpoint(upstream)
		if _, duplicate := seen[endpoint]; duplicate {
			continue
		}
		seen[endpoint] = struct{}{}
		normalized = append(normalized, upstream)
		if len(normalized) == MaxBootstrapDNSUpstreams {
			break
		}
	}
	for len(normalized) < 2 {
		fallback, ok := bestBootstrapDNSFallback(normalized, seen)
		if !ok {
			break
		}
		seen[bootstrapDNSUpstreamEndpoint(fallback)] = struct{}{}
		normalized = append(normalized, fallback)
	}
	return normalized
}

func bestBootstrapDNSFallback(existing []BootstrapDNSUpstream, seen map[string]struct{}) (BootstrapDNSUpstream, bool) {
	providers := make(map[string]struct{}, len(existing))
	transports := make(map[string]struct{}, len(existing))
	for _, upstream := range existing {
		providers[strings.ToLower(upstream.Provider)] = struct{}{}
		transports[upstream.Type] = struct{}{}
	}

	bestScore := -1
	var best BootstrapDNSUpstream
	for _, candidate := range DefaultBootstrapDNSUpstreams() {
		if _, duplicate := seen[bootstrapDNSUpstreamEndpoint(candidate)]; duplicate {
			continue
		}
		score := 0
		if _, duplicate := providers[strings.ToLower(candidate.Provider)]; !duplicate {
			score++
		}
		if _, duplicate := transports[candidate.Type]; !duplicate {
			score++
		}
		if score > bestScore {
			bestScore = score
			best = candidate
		}
	}
	return best, bestScore >= 0
}

func normalizeBootstrapDNSUpstream(upstream BootstrapDNSUpstream) (BootstrapDNSUpstream, bool) {
	normalized := upstream
	normalized.Provider = strings.TrimSpace(normalized.Provider)
	if normalized.Provider == "" {
		normalized.Provider = "custom"
	}
	if len(normalized.Provider) > 64 || strings.IndexFunc(normalized.Provider, func(r rune) bool { return r < 0x20 || r == 0x7f }) >= 0 {
		return BootstrapDNSUpstream{}, false
	}
	normalized.Type = strings.ToLower(strings.TrimSpace(normalized.Type))
	normalized.Server = strings.TrimSpace(normalized.Server)
	normalized.Path = strings.TrimSpace(normalized.Path)
	normalized.TLSServerName = strings.TrimSpace(normalized.TLSServerName)
	ip := net.ParseIP(normalized.Server)
	if ip == nil || normalized.Port < 1 || normalized.Port > 65535 {
		return BootstrapDNSUpstream{}, false
	}
	normalized.Server = ip.String()
	switch normalized.Type {
	case "udp", "tcp":
		normalized.Path = ""
		normalized.TLSServerName = ""
	case "https":
		if !strings.HasPrefix(normalized.Path, "/") || normalized.TLSServerName == "" {
			return BootstrapDNSUpstream{}, false
		}
	default:
		return BootstrapDNSUpstream{}, false
	}
	return normalized, true
}

func bootstrapDNSUpstreamEndpoint(upstream BootstrapDNSUpstream) string {
	return upstream.Type + "://" + upstream.Server + ":" + strconv.Itoa(upstream.Port)
}

func legacyBootstrapDNSUpstream(cfg Config) BootstrapDNSUpstream {
	return BootstrapDNSUpstream{
		Provider:      "legacy",
		Type:          EffectiveBootstrapDNSType(cfg),
		Server:        strings.TrimSpace(cfg.BootstrapDNSServer),
		Port:          cfg.BootstrapDNSPort,
		Path:          strings.TrimSpace(cfg.BootstrapDNSPath),
		TLSServerName: strings.TrimSpace(cfg.BootstrapDNSTLSServerName),
	}
}

// syncLegacyBootstrapDNSUpstream keeps the existing scalar editor useful
// during the migration period. Copying the slice prevents a Config value that
// was copied before SetField from observing an unexpected in-place mutation.
func syncLegacyBootstrapDNSUpstream(cfg *Config) {
	primary := legacyBootstrapDNSUpstream(*cfg)
	primary.Provider = "custom"
	upstreams := append([]BootstrapDNSUpstream(nil), cfg.BootstrapDNSUpstreams...)
	if len(upstreams) == 0 {
		upstreams = []BootstrapDNSUpstream{primary}
	} else {
		upstreams[0] = primary
	}
	cfg.BootstrapDNSUpstreams = upstreams
}

func syncLegacyBootstrapDNSFields(cfg *Config) {
	if len(cfg.BootstrapDNSUpstreams) == 0 {
		return
	}
	primary := cfg.BootstrapDNSUpstreams[0]
	cfg.BootstrapDNSType = primary.Type
	cfg.BootstrapDNSServer = primary.Server
	cfg.BootstrapDNSPort = primary.Port
	cfg.BootstrapDNSPath = primary.Path
	cfg.BootstrapDNSTLSServerName = primary.TLSServerName
}
