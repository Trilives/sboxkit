// Package config 定制层（对应 Python 版 customize.py）：
// customize.json 的默认值、加载/保存与字段元数据。
//
// 与 mihomo 版最大的不同：mihomo 直用订阅 + 最小改写，因此定制层只需覆写少量
// 部署字段；sing-box 不能解析 Clash 配置，internal/converter 要用这些字段
// 现场构造整份 inbounds/outbounds/route/dns，所以这里改用类型化的 Config
// struct（而非 map[string]any）供 converter 直接按字段访问，字段集合与
// internal/converter、internal/kernel、internal/subscription 的实际用量一一对应。
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/Trilives/sboxkit/internal/execx"
	"github.com/Trilives/sboxkit/internal/i18n"
	"github.com/Trilives/sboxkit/internal/paths"
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
	Language                string   `json:"language"`
}

// Defaults 返回一份全新的默认配置（列表均为拷贝，可放心修改）。
func Defaults() Config {
	return Config{
		AIDomainSuffixes: []string{
			"openai.com", "chatgpt.com", "oaistatic.com", "oaiusercontent.com",
			"anthropic.com", "claude.ai", "gemini.google.com", "huggingface.co",
			"github.com", "githubusercontent.com", "githubassets.com", "github.io",
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
			"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "169.254.0.0/16", "fc00::/7", "fe80::/10",
			"100.64.0.0/10",
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
		Language:            "en",
	}
}

// --------------------------------------------------------------------------
// 字段元数据（交互式编辑器与展示用）
// --------------------------------------------------------------------------

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
	"lan_panel":             "面板对外监听（0.0.0.0，否则仅本机 127.0.0.1）",
	"generate_sg_groups":    "生成新加坡自动测速聚合组",
	"generate_hk_groups":    "生成香港自动测速聚合组",
	"base64_local_fallback": "base64 应急本地解析",
	"enable_file_log":       "保存文件日志",
}

var ScalarFields = map[string]string{
	"bootstrap_dns_server": "引导 DNS 服务器",
	"bootstrap_dns_port":   "引导 DNS 端口",
	"default_outbound":     "默认出站（节点切换的目标分组）",
	"subconverter_backend": "subconverter 后端",
	"github_mirror":        "GitHub 加速前缀",
	"download_proxy":       "下载代理",
	"github_token":         "GitHub Token",
	"log_max_mb":           "日志大小上限（MB）",
}

// DeploymentFields 部署字段编辑分组（始终生效：TUN / 网络 / 面板 / 下载设置）。
var DeploymentFields = []string{
	"enable_tun",
	"lan_proxy",
	"lan_panel",
	"default_outbound",
	"download_proxy",
	"github_mirror",
	"github_token",
	"subconverter_backend",
	"base64_local_fallback",
	"bootstrap_dns_server",
	"bootstrap_dns_port",
	"route_exclude_ip_cidrs",
	"tun_exclude_uids",
	"bypass_process_names",
	"local_bypass_domains",
	"enable_file_log",
	"log_max_mb",
}

// OverlayFields 分流增强字段编辑分组（地区自动测速聚合组 + AI / 流媒体分流，
// 始终按各自开关/名单生效，不再有统一的 enable_overlay 总开关——
// sing-box 路径下配置整份由 converter 现场生成，没有"保留订阅原状"的顾虑）。
var OverlayFields = []string{
	"generate_sg_groups",
	"generate_hk_groups",
	"prefer_keywords",
	"hk_prefer_keywords",
	"ai_domain_suffixes",
	"streaming_domain_suffixes",
	"direct_domain_suffixes",
}

// FieldOrder 全部字段顺序（两个编辑分组拼接而成）。
var FieldOrder = append(append([]string{}, DeploymentFields...), OverlayFields...)

// SensitiveFields 涉密字段：菜单展示与编辑提示里都不出现明文。
var SensitiveFields = map[string]bool{"github_token": true}

// MaskSecret 已设置密钥的脱敏展示（保留末 4 位）。
func MaskSecret(value string) string {
	if value == "" {
		return i18n.T("未设置")
	}
	r := []rune(value)
	if len(r) > 4 {
		return fmt.Sprintf(i18n.T("已设置（***%s）"), string(r[len(r)-4:]))
	}
	return i18n.T("已设置（***）")
}

// FieldLabel 编辑器里的整行标签（名称 + 摘要）。
func FieldLabel(cfg Config, key string) string {
	if label, ok := ListFields[key]; ok {
		return fmt.Sprintf(i18n.T("%s（%s）"), i18n.T(label), FieldSummary(cfg, key))
	}
	if label, ok := BoolFields[key]; ok {
		return fmt.Sprintf(i18n.T("%s：%s"), i18n.T(label), FieldSummary(cfg, key))
	}
	return fmt.Sprintf(i18n.T("%s：%s"), i18n.T(ScalarFields[key]), FieldSummary(cfg, key))
}

// FieldSummary 字段值的单行摘要（列表→条数，布尔→开/关，涉密→脱敏）。
func FieldSummary(cfg Config, key string) string {
	switch key {
	case "enable_tun", "lan_panel", "lan_proxy", "generate_sg_groups", "generate_hk_groups", "base64_local_fallback", "enable_file_log":
		if Bool(cfg, key) {
			return i18n.T("开")
		}
		return i18n.T("关")
	case "ai_domain_suffixes", "streaming_domain_suffixes", "direct_domain_suffixes", "local_bypass_domains", "route_exclude_ip_cidrs", "bypass_process_names", "prefer_keywords", "hk_prefer_keywords":
		return listSummary(StrList(cfg, key))
	case "tun_exclude_uids":
		return listSummary(intListToStr(cfg.TunExcludeUIDs))
	case "github_token":
		return MaskSecret(cfg.GitHubToken)
	default:
		value := Str(cfg, key)
		if value == "" {
			return i18n.T("未设置")
		}
		return value
	}
}

func listSummary(items []string) string {
	if len(items) == 0 {
		return i18n.T("空")
	}
	return fmt.Sprintf(i18n.T("%d 条"), len(items))
}

func intListToStr(items []int) []string {
	out := make([]string, len(items))
	for i, v := range items {
		out[i] = strconv.Itoa(v)
	}
	return out
}

// --------------------------------------------------------------------------
// 容错取值（供编辑器/展示统一按字段名取值）
// --------------------------------------------------------------------------

func Bool(cfg Config, key string) bool {
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

func Str(cfg Config, key string) string {
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
	case "language":
		return cfg.Language
	default:
		return ""
	}
}

func StrList(cfg Config, key string) []string {
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

func IntList(cfg Config, key string) []int {
	if key == "tun_exclude_uids" {
		return append([]int(nil), cfg.TunExcludeUIDs...)
	}
	return nil
}

// ToggleBool 原地翻转一个布尔字段。
func ToggleBool(cfg *Config, key string) {
	SetField(cfg, key, boolStr(!Bool(*cfg, key)))
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// SetStrList 原地写入一个字符串列表字段。
func SetStrList(cfg *Config, key string, items []string) {
	switch key {
	case "ai_domain_suffixes":
		cfg.AIDomainSuffixes = items
	case "streaming_domain_suffixes":
		cfg.StreamingDomainSuffixes = items
	case "direct_domain_suffixes":
		cfg.DirectDomainSuffixes = items
	case "local_bypass_domains":
		cfg.LocalBypassDomains = items
	case "route_exclude_ip_cidrs":
		cfg.RouteExcludeIPCidrs = items
	case "bypass_process_names":
		cfg.BypassProcessNames = items
	case "prefer_keywords":
		cfg.PreferKeywords = items
	case "hk_prefer_keywords":
		cfg.HKPreferKeywords = items
	}
}

// SetIntList 原地写入一个整数列表字段。
func SetIntList(cfg *Config, key string, items []int) {
	if key == "tun_exclude_uids" {
		cfg.TunExcludeUIDs = items
	}
}

// SetField 按字段名写入标量字段（编辑器文本输入用）。
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
	case "language":
		cfg.Language = value
	case "log_max_mb":
		maxMB, err := strconv.Atoi(value)
		if err != nil {
			return err
		}
		cfg.LogMaxMB = clampLogMaxMB(maxMB)
	default:
		return errors.New("unknown scalar config field: " + key)
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

func SplitList(value string) []string {
	parts := strings.Split(value, ",")
	if len(parts) == 1 {
		parts = strings.Fields(value)
	}
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// --------------------------------------------------------------------------
// 读写 customize.json
// --------------------------------------------------------------------------

// Load 读 customize.json：缺失/损坏回退默认值并告警。
func Load(p paths.Paths) Config {
	cfg := Defaults()
	data, err := os.ReadFile(p.CustomizeFile)
	if err != nil {
		if !os.IsNotExist(err) {
			execx.Warn(i18n.T("customize.json 读取失败，使用默认值：") + err.Error())
		}
		return cfg
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		execx.Warn(i18n.T("customize.json 解析失败，使用默认值：") + err.Error())
		return Defaults()
	}
	return cfg
}

// Save 写盘（2 空格缩进、非 ASCII 不转义）。
func Save(p paths.Paths, cfg Config) error {
	if err := p.EnsureStateDirs(); err != nil {
		return err
	}
	var buf []byte
	buf, err := marshalNoEscape(cfg)
	if err != nil {
		return fmt.Errorf(i18n.T("序列化 customize.json: %w"), err)
	}
	tmp, err := os.CreateTemp(p.State, ".customize-*.json")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(buf); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, p.CustomizeFile)
}

// EnsureExists 首次运行时落地默认 customize.json。
func EnsureExists(p paths.Paths) (Config, error) {
	cfg := Load(p)
	if _, err := os.Stat(p.CustomizeFile); os.IsNotExist(err) {
		if err := Save(p, cfg); err != nil {
			return cfg, err
		}
	}
	return cfg, nil
}

func marshalNoEscape(v any) ([]byte, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}
