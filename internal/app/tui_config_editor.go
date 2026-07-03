package app

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/Trilives/sboxkit/internal/config"
	"github.com/Trilives/sboxkit/internal/paths"
	"github.com/Trilives/sboxkit/internal/subscription"
	ui "github.com/Trilives/sboxkit/internal/tui"
)

func editConfigAction() tuiAction {
	return commandAction("编辑定制层", func(s *tuiSession) int {
		changed, err := s.editCustomize()
		if err != nil {
			fmt.Fprintf(s.stderr, "编辑配置失败：%v\n", err)
			return 1
		}
		if !changed {
			return 0
		}
		p := paths.FromRoot("")
		manager := subscription.NewManager(p, loadConfigOrDefault(p.CustomizeFile))
		active, _ := manager.Active()
		if active == nil || !s.confirm("是否立即用本地原文重建当前订阅？", true) {
			return 0
		}
		code := runSub([]string{"rebuild", "--name", active.Name}, s.stdout, s.stderr)
		if code == 0 && s.confirm("是否同步到服务并重启？", true) && s.confirmServiceTrafficRisk("同步配置并重启 sboxkit.service") {
			code = runService([]string{"sync"}, s.stdout, s.stderr)
		}
		return code
	})
}

func loadConfigOrDefault(path string) config.Config {
	cfg, err := config.Load(path)
	if err != nil {
		return config.Defaults()
	}
	return cfg
}

func (s *tuiSession) editCustomize() (bool, error) {
	p := paths.FromRoot("")
	original, err := config.Load(p.CustomizeFile)
	if err != nil {
		return false, err
	}
	cfg := cloneConfig(original)
	changed := false
	idx := 0
	for {
		selected, err := s.selectF("编辑定制层", configSectionLabels(cfg), ui.SelectOpts{
			BackLabel: "放弃修改并退出",
			SaveLabel: "保存并退出",
			Initial:   idx,
		})
		if err != nil {
			if errors.Is(err, ui.ErrSaveExit) {
				if !changed {
					fmt.Fprintln(s.stdout, "未做修改。")
					return false, nil
				}
				if err := config.Save(p.CustomizeFile, cfg); err != nil {
					return false, err
				}
				fmt.Fprintln(s.stdout, "定制层已保存。")
				return true, nil
			}
			if changed {
				fmt.Fprintln(s.stdout, "已放弃本次修改（未写盘）。")
			}
			return false, nil
		}
		idx = selected
		changed = s.editConfigSection(&cfg, configSections[selected]) || changed
	}
}

type configSection struct {
	Title string
	Keys  []string
}

var configSections = []configSection{
	{
		Title: "常用部署",
		Keys:  []string{"enable_tun", "lan_proxy", "lan_panel", "download_proxy", "github_mirror", "github_token"},
	},
	{
		Title: "订阅与后端",
		Keys:  []string{"subconverter_backend", "base64_local_fallback"},
	},
	{
		Title: "DNS 与出站",
		Keys:  []string{"bootstrap_dns_server", "bootstrap_dns_port", "default_outbound"},
	},
	{
		Title: "TUN 与绕过",
		Keys:  []string{"route_exclude_ip_cidrs", "tun_exclude_uids", "bypass_process_names", "local_bypass_domains"},
	},
	{
		Title: "地区与分流",
		Keys: []string{
			"generate_sg_groups", "generate_hk_groups", "prefer_keywords", "hk_prefer_keywords",
			"ai_domain_suffixes", "streaming_domain_suffixes", "direct_domain_suffixes",
		},
	},
}

func configSectionLabels(cfg config.Config) []string {
	labels := make([]string, len(configSections))
	for i, section := range configSections {
		summary := sectionFieldLabels(cfg, section)
		labels[i] = fmt.Sprintf("%s（%d 项，%s）", section.Title, len(section.Keys), summary[0])
	}
	return labels
}

func (s *tuiSession) editConfigSection(cfg *config.Config, section configSection) bool {
	changed := false
	idx := 0
	for {
		selected, err := s.selectF("配置区块 · "+section.Title, sectionFieldLabels(*cfg, section), ui.SelectOpts{
			BackLabel: "返回区块列表",
			Initial:   idx,
		})
		if err != nil {
			return changed
		}
		idx = selected
		key := section.Keys[selected]
		switch {
		case config.BoolFields[key] != "":
			toggleBoolField(cfg, key)
			changed = true
		case config.ListFields[key] != "":
			changed = s.editListField(cfg, key, config.ListFields[key]) || changed
		default:
			changed = s.editScalarField(cfg, key, config.ScalarFields[key]) || changed
		}
	}
}

func configFieldLabels(cfg config.Config) []string {
	labels := make([]string, len(config.FieldOrder))
	for i, key := range config.FieldOrder {
		labels[i] = config.FieldLabel(cfg, key)
	}
	return labels
}

func sectionFieldLabels(cfg config.Config, section configSection) []string {
	labels := make([]string, len(section.Keys))
	for i, key := range section.Keys {
		labels[i] = config.FieldLabel(cfg, key)
	}
	return labels
}

func (s *tuiSession) editListField(cfg *config.Config, key string, label string) bool {
	changed := false
	action := 0
	for {
		items := stringListField(*cfg, key)
		if key == "tun_exclude_uids" {
			items = intListToStrings(cfg.TunExcludeUIDs)
		}
		s.printListSummary(label, items)
		selected, err := s.selectF("编辑 · "+label,
			[]string{"添加一条", "删除一条", "批量粘贴替换（逗号/空格分隔）", "恢复默认", "清空"},
			ui.SelectOpts{BackLabel: "返回字段列表", SaveLabel: "返回字段列表", Initial: action})
		if err != nil {
			return changed
		}
		action = selected

		next := append([]string(nil), items...)
		ok := true
		switch selected {
		case 0:
			value, err := s.askF("新增值", ui.AskOpts{AllowEmpty: false})
			if err != nil {
				ok = false
				break
			}
			next = append(next, value)
		case 1:
			if len(next) == 0 {
				continue
			}
			del, err := s.selectF("删除哪一条", next, ui.SelectOpts{BackLabel: "返回", SaveLabel: "返回"})
			if err != nil {
				ok = false
				break
			}
			next = append(next[:del], next[del+1:]...)
		case 2:
			raw, err := s.askF("粘贴（逗号或空格分隔）", ui.AskOpts{AllowEmpty: true})
			if err != nil {
				ok = false
				break
			}
			next = strings.Fields(strings.ReplaceAll(raw, ",", " "))
		case 3:
			next = defaultStringList(key)
			if key == "tun_exclude_uids" {
				next = intListToStrings(config.Defaults().TunExcludeUIDs)
			}
		case 4:
			next = []string{}
		}
		if ok && key == "tun_exclude_uids" && !allInts(next) {
			ok = false
		}
		if !ok {
			fmt.Fprintln(s.stdout, "输入无效，已跳过。")
			continue
		}
		setListField(cfg, key, next)
		changed = true
	}
}

func (s *tuiSession) printListSummary(label string, items []string) {
	summary := ""
	if len(items) > 0 {
		summary = "：" + strings.Join(items, ", ")
	}
	fmt.Fprintf(s.stdout, "%s：当前 %d 条%s\n", label, len(items), summary)
}

func (s *tuiSession) editScalarField(cfg *config.Config, key string, label string) bool {
	current := scalarField(*cfg, key)
	display := ""
	if config.SensitiveFields[key] && current != "" {
		display = config.MaskSecret(current)
	}
	value, err := s.askF(label+"（留空清除）", ui.AskOpts{
		Default:        current,
		DisplayDefault: display,
		AllowEmpty:     true,
	})
	if err != nil {
		return false
	}
	if key == "bootstrap_dns_port" {
		if _, err := strconv.Atoi(value); err != nil {
			fmt.Fprintln(s.stdout, "端口需为整数，未修改。")
			return false
		}
	}
	if err := config.SetField(cfg, key, value); err != nil {
		fmt.Fprintf(s.stdout, "输入无效，未修改：%v\n", err)
		return false
	}
	return true
}

func cloneConfig(cfg config.Config) config.Config {
	out := cfg
	out.AIDomainSuffixes = append([]string(nil), cfg.AIDomainSuffixes...)
	out.StreamingDomainSuffixes = append([]string(nil), cfg.StreamingDomainSuffixes...)
	out.DirectDomainSuffixes = append([]string(nil), cfg.DirectDomainSuffixes...)
	out.LocalBypassDomains = append([]string(nil), cfg.LocalBypassDomains...)
	out.RouteExcludeIPCidrs = append([]string(nil), cfg.RouteExcludeIPCidrs...)
	out.BypassProcessNames = append([]string(nil), cfg.BypassProcessNames...)
	out.TunExcludeUIDs = append([]int(nil), cfg.TunExcludeUIDs...)
	out.PreferKeywords = append([]string(nil), cfg.PreferKeywords...)
	out.HKPreferKeywords = append([]string(nil), cfg.HKPreferKeywords...)
	return out
}

func toggleBoolField(cfg *config.Config, key string) {
	switch key {
	case "enable_tun":
		cfg.EnableTun = !cfg.EnableTun
	case "lan_panel":
		cfg.LanPanel = !cfg.LanPanel
	case "lan_proxy":
		cfg.LanProxy = !cfg.LanProxy
	case "generate_sg_groups":
		cfg.GenerateSGGroups = !cfg.GenerateSGGroups
	case "generate_hk_groups":
		cfg.GenerateHKGroups = !cfg.GenerateHKGroups
	case "base64_local_fallback":
		cfg.Base64LocalFallback = !cfg.Base64LocalFallback
	}
}

func stringListField(cfg config.Config, key string) []string {
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

func defaultStringList(key string) []string {
	return stringListField(config.Defaults(), key)
}

func setListField(cfg *config.Config, key string, values []string) {
	switch key {
	case "tun_exclude_uids":
		cfg.TunExcludeUIDs = stringsToIntList(values)
	case "ai_domain_suffixes":
		cfg.AIDomainSuffixes = values
	case "streaming_domain_suffixes":
		cfg.StreamingDomainSuffixes = values
	case "direct_domain_suffixes":
		cfg.DirectDomainSuffixes = values
	case "local_bypass_domains":
		cfg.LocalBypassDomains = values
	case "route_exclude_ip_cidrs":
		cfg.RouteExcludeIPCidrs = values
	case "bypass_process_names":
		cfg.BypassProcessNames = values
	case "prefer_keywords":
		cfg.PreferKeywords = values
	case "hk_prefer_keywords":
		cfg.HKPreferKeywords = values
	}
}

func scalarField(cfg config.Config, key string) string {
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
	default:
		return ""
	}
}

func allInts(values []string) bool {
	for _, value := range values {
		if _, err := strconv.Atoi(value); err != nil {
			return false
		}
	}
	return true
}

func stringsToIntList(values []string) []int {
	out := make([]int, 0, len(values))
	for _, value := range values {
		n, _ := strconv.Atoi(value)
		out = append(out, n)
	}
	return out
}

func intListToStrings(values []int) []string {
	out := make([]string, len(values))
	for i, value := range values {
		out[i] = strconv.Itoa(value)
	}
	return out
}
