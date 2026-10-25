package flows

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Trilives/sboxkit/internal/config"
	"github.com/Trilives/sboxkit/internal/firewall"
	"github.com/Trilives/sboxkit/internal/i18n"
	"github.com/Trilives/sboxkit/internal/paths"
	"github.com/Trilives/sboxkit/internal/proxyenv"
	"github.com/Trilives/sboxkit/internal/tui"
	"github.com/Trilives/sboxkit/internal/txn"
)

type initFormPlan struct {
	Original      config.Config
	Config        config.Config
	AllowFirewall bool
	WriteProxyEnv bool
	Subscription  *newSub
}

func collectInitPlan(p paths.Paths) (initFormPlan, error) {
	cfg := config.Load(p)
	values, err := tui.Form(i18n.T("sboxkit 初始化"), initFormFields(p, cfg), tui.FormOpts{
		SubmitLabel: i18n.T("开始初始化"),
		Hint:        i18n.T("详细配置请在初始化完成后通过“配置变更”修改"),
	})
	if err != nil {
		return initFormPlan{}, err
	}

	updated := cfg
	updated.DownloadProxy = downloadProxyFromForm(values)
	if err := config.SetField(&updated, "mixed_port", values["mixed_port"]); err != nil {
		return initFormPlan{}, err
	}
	updated.EnableTun = values.Bool("enable_tun")
	updated.LanProxy = values.Bool("lan_proxy")
	if updated.EnableTun {
		updated.TunExcludeUIDs, _ = parseUIDs(values["tun_uids"])
		updated.BypassProcessNames = config.SplitList(values["process_names"])
	}

	info, err := initSubscriptionFromForm(values)
	if err != nil {
		return initFormPlan{}, err
	}
	return initFormPlan{
		Original: cfg, Config: updated, AllowFirewall: values.Bool("allow_firewall") && !p.UserMode,
		WriteProxyEnv: values.Bool("write_proxy_env"), Subscription: info,
	}, nil
}

// initFormFields 下载代理先问「是否使用」，选是才需要填写（且必须填写）代理地址；
// 订阅「使用代理拉取」同样只在启用下载代理后才有意义。
func initFormFields(p paths.Paths, cfg config.Config) []tui.FormField {
	sourceValues, sourceLabels := initSourceOptions()
	return []tui.FormField{
		{Key: "use_download_proxy", Label: i18n.T("使用下载代理"), Kind: tui.FormBool, Value: boolString(cfg.DownloadProxy != "")},
		{
			Key: "download_proxy", Label: i18n.T("下载代理地址"), Kind: tui.FormText, Value: cfg.DownloadProxy, Placeholder: "http://127.0.0.1:7890",
			Enabled: enabledWhen("use_download_proxy", true), Required: true, Validate: validateDownloadProxy,
		},
		{Key: "mixed_port", Label: i18n.T("本地代理 mixed 端口"), Kind: tui.FormText, Value: strconv.Itoa(config.EffectiveMixedPort(cfg)), Validate: validateMixedPort},
		{Key: "enable_tun", Label: tunFieldLabel(p), Kind: tui.FormBool, Value: boolString(cfg.EnableTun)},
		{Key: "lan_proxy", Label: i18n.T("局域网代理"), Kind: tui.FormBool, Value: boolString(cfg.LanProxy)},
		systemModeOnly(p, tui.FormField{Key: "allow_firewall", Label: i18n.T("放行代理端口"), Kind: tui.FormBool, Value: "true", Enabled: enabledWhen("lan_proxy", true)}),
		{Key: "write_proxy_env", Label: i18n.T("写入 shell 代理变量"), Kind: tui.FormBool, Value: "true", Enabled: enabledWhen("enable_tun", false)},
		{Key: "tun_uids", Label: i18n.T("TUN 直连 UID"), Kind: tui.FormText, Value: joinInts(cfg.TunExcludeUIDs), Placeholder: "1000, 1001", Enabled: enabledWhen("enable_tun", true), Validate: validateUIDs},
		{Key: "process_names", Label: i18n.T("TUN 直连进程名"), Kind: tui.FormText, Value: strings.Join(cfg.BypassProcessNames, ", "), Placeholder: "qbittorrent-nox, aria2c", Enabled: enabledWhen("enable_tun", true)},
		{Key: "sub_name", Label: i18n.T("订阅名称"), Kind: tui.FormText, Value: time.Now().Format("sub-20060102-150405")},
		{Key: "source", Label: i18n.T("订阅类型"), Kind: tui.FormChoice, Value: sourceValues[0], Options: sourceValues, OptionLabels: sourceLabels},
		{Key: "sub_url", Label: i18n.T("订阅地址 / 本地路径"), Kind: tui.FormText, Placeholder: "https://example.com/sub", Validate: validateSubscriptionInput},
		{Key: "fetch_proxy", Label: i18n.T("使用代理拉取"), Kind: tui.FormBool, Value: "false", Enabled: fetchThroughProxyAvailable},
		{Key: "customize", Label: i18n.T("按 sboxkit 规则重建"), Kind: tui.FormBool, Value: "true", Enabled: func(v tui.FormResult) bool { return v["source"] == "sing-box" }},
	}
}

func fetchThroughProxyAvailable(values tui.FormResult) bool {
	return values.Bool("use_download_proxy") && values["source"] != "local"
}

// downloadProxyFromForm 未启用下载代理时清空已保存的地址，避免残留值继续生效。
func downloadProxyFromForm(values tui.FormResult) string {
	if !values.Bool("use_download_proxy") {
		return ""
	}
	return strings.TrimSpace(values["download_proxy"])
}

func initSubscriptionFromForm(values tui.FormResult) (*newSub, error) {
	rawURL := strings.TrimSpace(values["sub_url"])
	if rawURL == "" {
		return nil, nil
	}
	name := strings.TrimSpace(values["sub_name"])
	if name == "" {
		return nil, fmt.Errorf("%s", i18n.T("订阅名称不能为空"))
	}
	chosen := values["source"]
	if !knownSourceType(chosen) {
		chosen = subscriptionSources[0].Type
	}
	sourceType, err := subscriptionSourceType(chosen, rawURL)
	if err != nil {
		return nil, err
	}
	if sourceType == "local" {
		resolved, err := resolveLocalPath(rawURL)
		if err != nil {
			return nil, err
		}
		rawURL = resolved
	}
	rebuild := true
	if chosen == "sing-box" {
		rebuild = values.Bool("customize")
	}
	fetchViaProxy := sourceType != "local" && fetchThroughProxyAvailable(values) && values.Bool("fetch_proxy")
	return &newSub{
		Name: name, URL: rawURL, SourceType: sourceType,
		ApplyOverlay: rebuild, FetchViaProxy: fetchViaProxy,
	}, nil
}

// validateSubscriptionInput 远程订阅只支持 http(s)；不带 scheme 的输入按本地文件校验。
func validateSubscriptionInput(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if !isRemoteSource(value) {
		if _, err := resolveLocalPath(value); err != nil {
			return fmt.Errorf(i18n.T("%w（远程订阅地址须以 http:// 或 https:// 开头）"), err)
		}
		return nil
	}
	u, err := url.Parse(value)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("%s", i18n.T("订阅地址须以 http:// 或 https:// 开头，本地文件请直接填写路径"))
	}
	return nil
}

func applyInitDeployment(p paths.Paths, plan initFormPlan) error {
	return txn.Run(i18n.T("部署设置"), func(t *txn.Transaction) error {
		if err := t.BackupFile(p.CustomizeFile); err != nil {
			return err
		}
		if err := config.Save(p, plan.Config); err != nil {
			return err
		}
		oldPort := config.EffectiveMixedPort(plan.Original)
		newPort := config.EffectiveMixedPort(plan.Config)
		if !p.UserMode && plan.Original.LanProxy && (!plan.Config.LanProxy || oldPort != newPort) {
			firewall.Revoke(oldPort)
		}
		if plan.Config.LanProxy && plan.AllowFirewall {
			t.AddUndo(fmt.Sprintf(i18n.T("撤销防火墙放行 %d"), newPort), func() error { firewall.Revoke(newPort); return nil })
			firewall.Allow(newPort)
		}
		if !plan.Config.EnableTun && plan.WriteProxyEnv {
			if err := t.BackupFile(proxyenv.TargetBashrc()); err != nil {
				return err
			}
			_, err := proxyenv.Write(newPort)
			return err
		}
		return nil
	})
}

// tunFieldLabel 用户模式下 TUN 仍可选，但应用时会请求一次 sudo（setcap），标签提前说明。
func tunFieldLabel(p paths.Paths) string {
	if p.UserMode {
		return i18n.T("启用 TUN") + i18n.T("（需 sudo 授权）")
	}
	return i18n.T("启用 TUN")
}

// systemModeOnly 用户模式不修改防火墙（需要 root）：保持字段可见以免用户找不到，
// 但禁用并在标签上说明原因。
func systemModeOnly(p paths.Paths, field tui.FormField) tui.FormField {
	if !p.UserMode {
		return field
	}
	field.Label += i18n.T("（用户模式不可用）")
	field.Value = "false"
	field.Enabled = func(tui.FormResult) bool { return false }
	return field
}

func initSourceOptions() ([]string, map[string]string) {
	values := make([]string, len(subscriptionSources))
	labels := make(map[string]string, len(subscriptionSources))
	for i, source := range subscriptionSources {
		values[i] = source.Type
		labels[source.Type] = i18n.T(source.ShortLabel)
	}
	return values, labels
}

func knownSourceType(value string) bool {
	for _, source := range subscriptionSources {
		if source.Type == value {
			return true
		}
	}
	return false
}

func enabledWhen(key string, want bool) func(tui.FormResult) bool {
	return func(values tui.FormResult) bool { return values.Bool(key) == want }
}

func validateMixedPort(value string) error {
	_, err := config.ParsePort(value)
	return err
}

func validateDownloadProxy(value string) error {
	_, err := config.ParseDownloadProxy(value)
	return err
}

func validateUIDs(value string) error {
	_, err := parseUIDs(value)
	return err
}

func parseUIDs(value string) ([]int, error) {
	parts := config.SplitList(value)
	out := make([]int, 0, len(parts))
	for _, part := range parts {
		uid, err := strconv.Atoi(part)
		if err != nil || uid < 0 {
			return nil, fmt.Errorf(i18n.T("UID 必须是非负整数：%s"), part)
		}
		out = append(out, uid)
	}
	return out, nil
}

func joinInts(values []int) string {
	parts := make([]string, len(values))
	for i, value := range values {
		parts[i] = strconv.Itoa(value)
	}
	return strings.Join(parts, ", ")
}

func boolString(value bool) string {
	if value {
		return "true"
	}
	return "false"
}
