package config

import (
	"encoding/json"
	"errors"
	"fmt"
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
	EnableFileLog           bool     `json:"enable_file_log"`
	LogMaxMB                int      `json:"log_max_mb"`
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
		EnableFileLog:       false,
		LogMaxMB:            10,
	}
}

var ListFields = map[string]string{
	"ai_domain_suffixes":        "AI 域名后缀",
	"streaming_domain_suffixes": "流媒体域名后缀",
	"direct_domain_suffixes":    "直连域名后缀",
	"local_bypass_domains":      "本地域名绕过",
	"route_exclude_ip_cidrs":    "TUN 排除网段",
	"bypass_process_names":      "绕过进程名",
	"tun_exclude_uids":          "TUN 排除 UID",
	"prefer_keywords":           "新加坡关键词",
	"hk_prefer_keywords":        "香港关键词",
}

var BoolFields = map[string]string{
	"enable_tun":            "TUN 模式（全局透明代理）",
	"lan_proxy":             "局域网代理（其他主机可用本机代理）",
	"lan_panel":             "内置 WebUI",
	"generate_sg_groups":    "生成新加坡自动测速聚合组",
	"generate_hk_groups":    "生成香港自动测速聚合组",
	"base64_local_fallback": "base64 应急本地解析",
	"enable_file_log":       "保存文件日志",
}

var ScalarFields = map[string]string{
	"bootstrap_dns_server": "引导 DNS 服务器",
	"bootstrap_dns_port":   "引导 DNS 端口",
	"default_outbound":     "默认出站",
	"subconverter_backend": "subconverter 后端",
	"github_mirror":        "GitHub 加速前缀",
	"download_proxy":       "下载代理",
	"github_token":         "GitHub Token",
	"log_max_mb":           "日志大小上限（MB）",
}

var FieldOrder = []string{
	"enable_tun",
	"lan_proxy",
	"lan_panel",
	"download_proxy",
	"github_mirror",
	"github_token",
	"enable_file_log",
	"log_max_mb",
	"subconverter_backend",
	"base64_local_fallback",
	"bootstrap_dns_server",
	"bootstrap_dns_port",
	"default_outbound",
	"route_exclude_ip_cidrs",
	"tun_exclude_uids",
	"bypass_process_names",
	"local_bypass_domains",
	"generate_sg_groups",
	"generate_hk_groups",
	"prefer_keywords",
	"hk_prefer_keywords",
	"ai_domain_suffixes",
	"streaming_domain_suffixes",
	"direct_domain_suffixes",
}

var SensitiveFields = map[string]bool{"github_token": true}

func MaskSecret(value string) string {
	if value == "" {
		return "未设置"
	}
	runes := []rune(value)
	if len(runes) > 4 {
		return "已设置（***" + string(runes[len(runes)-4:]) + "）"
	}
	return "已设置（***）"
}

func FieldLabel(cfg Config, key string) string {
	if label, ok := ListFields[key]; ok {
		return fmt.Sprintf("%s（%s）", label, FieldSummary(cfg, key))
	}
	if label, ok := BoolFields[key]; ok {
		return fmt.Sprintf("%s：%s", label, FieldSummary(cfg, key))
	}
	return fmt.Sprintf("%s：%s", ScalarFields[key], FieldSummary(cfg, key))
}

func FieldSummary(cfg Config, key string) string {
	switch key {
	case "enable_tun", "lan_panel", "lan_proxy", "generate_sg_groups", "generate_hk_groups", "base64_local_fallback", "enable_file_log":
		if getBool(cfg, key) {
			return "开"
		}
		return "关"
	case "ai_domain_suffixes", "streaming_domain_suffixes", "direct_domain_suffixes", "local_bypass_domains", "route_exclude_ip_cidrs", "bypass_process_names", "prefer_keywords", "hk_prefer_keywords":
		return listSummary(getStringList(cfg, key))
	case "tun_exclude_uids":
		return intListSummary(cfg.TunExcludeUIDs)
	case "github_token":
		return MaskSecret(cfg.GitHubToken)
	default:
		value := getString(cfg, key)
		if value == "" {
			return "未设置"
		}
		return value
	}
}

func listSummary(items []string) string {
	if len(items) == 0 {
		return "空"
	}
	return fmt.Sprintf("%d 条", len(items))
}

func intListSummary(items []int) string {
	if len(items) == 0 {
		return "空"
	}
	return fmt.Sprintf("%d 条", len(items))
}

func getBool(cfg Config, key string) bool {
	switch key {
	case "enable_tun":
		return cfg.EnableTun
	case "lan_panel":
		return cfg.LanPanel
	case "lan_proxy":
		return cfg.LanProxy
	case "generate_sg_groups":
		return cfg.GenerateSGGroups
	case "generate_hk_groups":
		return cfg.GenerateHKGroups
	case "base64_local_fallback":
		return cfg.Base64LocalFallback
	case "enable_file_log":
		return cfg.EnableFileLog
	default:
		return false
	}
}

func getString(cfg Config, key string) string {
	switch key {
	case "bootstrap_dns_server":
		return cfg.BootstrapDNSServer
	case "bootstrap_dns_port":
		return strconv.Itoa(cfg.BootstrapDNSPort)
	case "default_outbound":
		return cfg.DefaultOutbound
	case "subconverter_backend":
		return cfg.SubconverterBackend
	case "github_mirror":
		return cfg.GitHubMirror
	case "download_proxy":
		return cfg.DownloadProxy
	case "github_token":
		return cfg.GitHubToken
	case "log_max_mb":
		return strconv.Itoa(cfg.LogMaxMB)
	default:
		return ""
	}
}

func getStringList(cfg Config, key string) []string {
	switch key {
	case "ai_domain_suffixes":
		return append([]string(nil), cfg.AIDomainSuffixes...)
	case "streaming_domain_suffixes":
		return append([]string(nil), cfg.StreamingDomainSuffixes...)
	case "direct_domain_suffixes":
		return append([]string(nil), cfg.DirectDomainSuffixes...)
	case "local_bypass_domains":
		return append([]string(nil), cfg.LocalBypassDomains...)
	case "route_exclude_ip_cidrs":
		return append([]string(nil), cfg.RouteExcludeIPCidrs...)
	case "bypass_process_names":
		return append([]string(nil), cfg.BypassProcessNames...)
	case "prefer_keywords":
		return append([]string(nil), cfg.PreferKeywords...)
	case "hk_prefer_keywords":
		return append([]string(nil), cfg.HKPreferKeywords...)
	default:
		return nil
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
		EnableFileLog           *bool     `json:"enable_file_log"`
		LogMaxMB                *int      `json:"log_max_mb"`
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
	EnableFileLog           *bool     `json:"enable_file_log"`
	LogMaxMB                *int      `json:"log_max_mb"`
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
	if o.EnableFileLog != nil {
		cfg.EnableFileLog = *o.EnableFileLog
	}
	if o.LogMaxMB != nil {
		cfg.LogMaxMB = clampLogMaxMB(*o.LogMaxMB)
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
	case "enable_file_log":
		cfg.EnableFileLog = parseBool(value)
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
	case "log_max_mb":
		maxMB, err := strconv.Atoi(value)
		if err != nil {
			return err
		}
		cfg.LogMaxMB = clampLogMaxMB(maxMB)
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

func clampLogMaxMB(value int) int {
	if value <= 0 {
		return 10
	}
	if value > 100 {
		return 100
	}
	return value
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
