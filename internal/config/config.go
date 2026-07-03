package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	AIDomainSuffixes        []string `json:"ai_domain_suffixes"`
	StreamingDomainSuffixes []string `json:"streaming_domain_suffixes"`
	DirectDomainSuffixes    []string `json:"direct_domain_suffixes"`
	LocalBypassDomains      []string `json:"local_bypass_domains"`
	RouteExcludeIPCidrs     []string `json:"route_exclude_ip_cidrs"`
	BypassProcessNames      []string `json:"bypass_process_names"`
	TunExcludeUIDs          []int    `json:"tun_exclude_uids"`
	EnableTun               bool     `json:"enable_tun"`
	LanPanel                bool     `json:"lan_panel"`
	LanProxy                bool     `json:"lan_proxy"`
	BootstrapDNSServer      string   `json:"bootstrap_dns_server"`
	BootstrapDNSPort        int      `json:"bootstrap_dns_port"`
	GenerateSGGroups        bool     `json:"generate_sg_groups"`
	GenerateHKGroups        bool     `json:"generate_hk_groups"`
	PreferKeywords          []string `json:"prefer_keywords"`
	HKPreferKeywords        []string `json:"hk_prefer_keywords"`
	DefaultOutbound         string   `json:"default_outbound"`
	SubconverterBackend     string   `json:"subconverter_backend"`
	Base64LocalFallback     bool     `json:"base64_local_fallback"`
	GitHubMirror            string   `json:"github_mirror"`
	DownloadProxy           string   `json:"download_proxy"`
	GitHubToken             string   `json:"github_token"`
}

func Defaults() Config {
	return Config{
		AIDomainSuffixes: []string{
			"openai.com", "chatgpt.com", "oaistatic.com", "oaiusercontent.com",
			"anthropic.com", "claude.ai", "github.com", "githubusercontent.com",
			"githubassets.com", "github.io", "huggingface.co", "hf.co",
			"npmjs.com", "npmjs.org", "pypi.org", "pythonhosted.org",
			"files.pythonhosted.org", "docker.com", "docker.io", "ghcr.io",
		},
		StreamingDomainSuffixes: []string{
			"netflix.com", "nflxvideo.net", "nflximg.net", "nflxso.net",
			"disneyplus.com", "disney-plus.net", "dssott.com", "hulu.com",
			"huluim.com", "hbomax.com", "max.com", "primevideo.com",
			"amazonvideo.com", "youtube.com", "googlevideo.com", "ytimg.com",
			"spotify.com", "scdn.co",
		},
		DirectDomainSuffixes: []string{},
		LocalBypassDomains:   []string{"localhost"},
		RouteExcludeIPCidrs: []string{
			"127.0.0.0/8", "0.0.0.0/8", "::1/128",
			"10.0.0.0/8", "192.168.0.0/16", "169.254.0.0/16", "fc00::/7", "fe80::/10",
			"100.64.0.0/10", "fd7a:115c:a1e0::/48", "10.126.126.0/24", "10.14.14.0/24",
		},
		BypassProcessNames:  []string{"tailscale", "tailscaled"},
		TunExcludeUIDs:      []int{},
		EnableTun:           true,
		LanPanel:            false,
		LanProxy:            false,
		BootstrapDNSServer:  "223.5.5.5",
		BootstrapDNSPort:    53,
		GenerateSGGroups:    false,
		GenerateHKGroups:    false,
		PreferKeywords:      []string{"Singapore", "SG", "新加坡", "狮城"},
		HKPreferKeywords:    []string{"Hong Kong", "HongKong", "HK", "香港"},
		DefaultOutbound:     "Proxy",
		SubconverterBackend: "https://sub.v1.mk",
		Base64LocalFallback: false,
		GitHubMirror:        "",
		DownloadProxy:       "",
		GitHubToken:         "",
	}
}

func Load(path string) (Config, error) {
	cfg := Defaults()
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}

	var overrides struct {
		AIDomainSuffixes        *[]string `json:"ai_domain_suffixes"`
		StreamingDomainSuffixes *[]string `json:"streaming_domain_suffixes"`
		DirectDomainSuffixes    *[]string `json:"direct_domain_suffixes"`
		LocalBypassDomains      *[]string `json:"local_bypass_domains"`
		RouteExcludeIPCidrs     *[]string `json:"route_exclude_ip_cidrs"`
		BypassProcessNames      *[]string `json:"bypass_process_names"`
		TunExcludeUIDs          *[]int    `json:"tun_exclude_uids"`
		EnableTun               *bool     `json:"enable_tun"`
		LanPanel                *bool     `json:"lan_panel"`
		LanProxy                *bool     `json:"lan_proxy"`
		BootstrapDNSServer      *string   `json:"bootstrap_dns_server"`
		BootstrapDNSPort        *int      `json:"bootstrap_dns_port"`
		GenerateSGGroups        *bool     `json:"generate_sg_groups"`
		GenerateHKGroups        *bool     `json:"generate_hk_groups"`
		PreferKeywords          *[]string `json:"prefer_keywords"`
		HKPreferKeywords        *[]string `json:"hk_prefer_keywords"`
		DefaultOutbound         *string   `json:"default_outbound"`
		SubconverterBackend     *string   `json:"subconverter_backend"`
		Base64LocalFallback     *bool     `json:"base64_local_fallback"`
		GitHubMirror            *string   `json:"github_mirror"`
		DownloadProxy           *string   `json:"download_proxy"`
		GitHubToken             *string   `json:"github_token"`
	}
	if err := json.Unmarshal(data, &overrides); err != nil {
		return cfg, err
	}

	applyOverrides(&cfg, overrides)
	return cfg, nil
}

func Save(path string, cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(filepath.Dir(path), ".customize-*.json")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func applyOverrides(cfg *Config, o struct {
	AIDomainSuffixes        *[]string `json:"ai_domain_suffixes"`
	StreamingDomainSuffixes *[]string `json:"streaming_domain_suffixes"`
	DirectDomainSuffixes    *[]string `json:"direct_domain_suffixes"`
	LocalBypassDomains      *[]string `json:"local_bypass_domains"`
	RouteExcludeIPCidrs     *[]string `json:"route_exclude_ip_cidrs"`
	BypassProcessNames      *[]string `json:"bypass_process_names"`
	TunExcludeUIDs          *[]int    `json:"tun_exclude_uids"`
	EnableTun               *bool     `json:"enable_tun"`
	LanPanel                *bool     `json:"lan_panel"`
	LanProxy                *bool     `json:"lan_proxy"`
	BootstrapDNSServer      *string   `json:"bootstrap_dns_server"`
	BootstrapDNSPort        *int      `json:"bootstrap_dns_port"`
	GenerateSGGroups        *bool     `json:"generate_sg_groups"`
	GenerateHKGroups        *bool     `json:"generate_hk_groups"`
	PreferKeywords          *[]string `json:"prefer_keywords"`
	HKPreferKeywords        *[]string `json:"hk_prefer_keywords"`
	DefaultOutbound         *string   `json:"default_outbound"`
	SubconverterBackend     *string   `json:"subconverter_backend"`
	Base64LocalFallback     *bool     `json:"base64_local_fallback"`
	GitHubMirror            *string   `json:"github_mirror"`
	DownloadProxy           *string   `json:"download_proxy"`
	GitHubToken             *string   `json:"github_token"`
}) {
	if o.AIDomainSuffixes != nil {
		cfg.AIDomainSuffixes = *o.AIDomainSuffixes
	}
	if o.StreamingDomainSuffixes != nil {
		cfg.StreamingDomainSuffixes = *o.StreamingDomainSuffixes
	}
	if o.DirectDomainSuffixes != nil {
		cfg.DirectDomainSuffixes = *o.DirectDomainSuffixes
	}
	if o.LocalBypassDomains != nil {
		cfg.LocalBypassDomains = *o.LocalBypassDomains
	}
	if o.RouteExcludeIPCidrs != nil {
		cfg.RouteExcludeIPCidrs = *o.RouteExcludeIPCidrs
	}
	if o.BypassProcessNames != nil {
		cfg.BypassProcessNames = *o.BypassProcessNames
	}
	if o.TunExcludeUIDs != nil {
		cfg.TunExcludeUIDs = *o.TunExcludeUIDs
	}
	if o.EnableTun != nil {
		cfg.EnableTun = *o.EnableTun
	}
	if o.LanPanel != nil {
		cfg.LanPanel = *o.LanPanel
	}
	if o.LanProxy != nil {
		cfg.LanProxy = *o.LanProxy
	}
	if o.BootstrapDNSServer != nil {
		cfg.BootstrapDNSServer = *o.BootstrapDNSServer
	}
	if o.BootstrapDNSPort != nil {
		cfg.BootstrapDNSPort = *o.BootstrapDNSPort
	}
	if o.GenerateSGGroups != nil {
		cfg.GenerateSGGroups = *o.GenerateSGGroups
	}
	if o.GenerateHKGroups != nil {
		cfg.GenerateHKGroups = *o.GenerateHKGroups
	}
	if o.PreferKeywords != nil {
		cfg.PreferKeywords = *o.PreferKeywords
	}
	if o.HKPreferKeywords != nil {
		cfg.HKPreferKeywords = *o.HKPreferKeywords
	}
	if o.DefaultOutbound != nil {
		cfg.DefaultOutbound = *o.DefaultOutbound
	}
	if o.SubconverterBackend != nil {
		cfg.SubconverterBackend = *o.SubconverterBackend
	}
	if o.Base64LocalFallback != nil {
		cfg.Base64LocalFallback = *o.Base64LocalFallback
	}
	if o.GitHubMirror != nil {
		cfg.GitHubMirror = *o.GitHubMirror
	}
	if o.DownloadProxy != nil {
		cfg.DownloadProxy = *o.DownloadProxy
	}
	if o.GitHubToken != nil {
		cfg.GitHubToken = *o.GitHubToken
	}
}

func SetField(cfg *Config, key string, value string) error {
	switch key {
	case "enable_tun":
		cfg.EnableTun = parseBool(value)
	case "lan_panel":
		cfg.LanPanel = parseBool(value)
	case "lan_proxy":
		cfg.LanProxy = parseBool(value)
	case "generate_sg_groups":
		cfg.GenerateSGGroups = parseBool(value)
	case "generate_hk_groups":
		cfg.GenerateHKGroups = parseBool(value)
	case "base64_local_fallback":
		cfg.Base64LocalFallback = parseBool(value)
	case "bootstrap_dns_server":
		cfg.BootstrapDNSServer = value
	case "bootstrap_dns_port":
		port, err := strconv.Atoi(value)
		if err != nil {
			return err
		}
		cfg.BootstrapDNSPort = port
	case "default_outbound":
		cfg.DefaultOutbound = value
	case "subconverter_backend":
		cfg.SubconverterBackend = value
	case "github_mirror":
		cfg.GitHubMirror = value
	case "download_proxy":
		cfg.DownloadProxy = value
	case "github_token":
		cfg.GitHubToken = value
	case "ai_domain_suffixes":
		cfg.AIDomainSuffixes = splitList(value)
	case "streaming_domain_suffixes":
		cfg.StreamingDomainSuffixes = splitList(value)
	case "direct_domain_suffixes":
		cfg.DirectDomainSuffixes = splitList(value)
	case "local_bypass_domains":
		cfg.LocalBypassDomains = splitList(value)
	case "route_exclude_ip_cidrs":
		cfg.RouteExcludeIPCidrs = splitList(value)
	case "bypass_process_names":
		cfg.BypassProcessNames = splitList(value)
	case "prefer_keywords":
		cfg.PreferKeywords = splitList(value)
	case "hk_prefer_keywords":
		cfg.HKPreferKeywords = splitList(value)
	default:
		return errors.New("unknown config field: " + key)
	}
	return nil
}

func parseBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func splitList(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
