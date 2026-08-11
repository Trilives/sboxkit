package flows

import (
	"fmt"
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
	sourceValues, sourceLabels := initSourceOptions()
	fields := []tui.FormField{
		{Key: "download_proxy", Label: i18n.T("下载代理"), Kind: tui.FormText, Value: cfg.DownloadProxy, Placeholder: "http://127.0.0.1:7890"},
		{Key: "mixed_port", Label: i18n.T("本地代理 mixed 端口"), Kind: tui.FormText, Value: strconv.Itoa(config.EffectiveMixedPort(cfg)), Validate: validateMixedPort},
		{Key: "enable_tun", Label: i18n.T("启用 TUN"), Kind: tui.FormBool, Value: boolString(cfg.EnableTun)},
		{Key: "lan_proxy", Label: i18n.T("局域网代理"), Kind: tui.FormBool, Value: boolString(cfg.LanProxy)},
		{Key: "allow_firewall", Label: i18n.T("放行代理端口"), Kind: tui.FormBool, Value: "true", Enabled: enabledWhen("lan_proxy", true)},
		{Key: "write_proxy_env", Label: i18n.T("写入 shell 代理变量"), Kind: tui.FormBool, Value: "true", Enabled: enabledWhen("enable_tun", false)},
		{Key: "tun_uids", Label: i18n.T("TUN 直连 UID"), Kind: tui.FormText, Value: joinInts(cfg.TunExcludeUIDs), Placeholder: "1000, 1001", Enabled: enabledWhen("enable_tun", true), Validate: validateUIDs},
		{Key: "process_names", Label: i18n.T("TUN 直连进程名"), Kind: tui.FormText, Value: strings.Join(cfg.BypassProcessNames, ", "), Placeholder: "tailscale, tailscaled", Enabled: enabledWhen("enable_tun", true)},
		{Key: "sub_name", Label: i18n.T("订阅名称"), Kind: tui.FormText, Value: time.Now().Format("sub-20060102-150405")},
		{Key: "source", Label: i18n.T("订阅类型"), Kind: tui.FormChoice, Value: sourceValues[0], Options: sourceValues, OptionLabels: sourceLabels},
		{Key: "sub_url", Label: i18n.T("订阅地址 / 本地路径"), Kind: tui.FormText, Placeholder: "https://example.com/sub"},
		{Key: "fetch_proxy", Label: i18n.T("使用代理拉取"), Kind: tui.FormBool, Value: "false", Enabled: func(v tui.FormResult) bool { return v["source"] != "local" }},
		{Key: "customize", Label: i18n.T("按 sboxkit 规则重建"), Kind: tui.FormBool, Value: "true", Enabled: func(v tui.FormResult) bool { return v["source"] == "sing-box" }},
	}
	values, err := tui.Form(i18n.T("sboxkit 初始化"), fields, tui.FormOpts{
		SubmitLabel: i18n.T("开始初始化"),
		Hint:        i18n.T("详细配置请在初始化完成后通过“配置变更”修改"),
	})
	if err != nil {
		return initFormPlan{}, err
	}

	updated := cfg
	updated.DownloadProxy = strings.TrimSpace(values["download_proxy"])
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
		Original: cfg, Config: updated, AllowFirewall: values.Bool("allow_firewall"),
		WriteProxyEnv: values.Bool("write_proxy_env"), Subscription: info,
	}, nil
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
	sourceType := values["source"]
	if !knownSourceType(sourceType) {
		sourceType = subscriptionSources[0].Type
	}
	if sourceType == "local" {
		resolved, err := resolveLocalPath(rawURL)
		if err != nil {
			return nil, err
		}
		rawURL = resolved
	}
	rebuild := true
	if sourceType == "sing-box" {
		rebuild = values.Bool("customize")
	}
	return &newSub{
		Name: name, URL: rawURL, SourceType: sourceType,
		ApplyOverlay: rebuild, FetchViaProxy: sourceType != "local" && values.Bool("fetch_proxy"),
	}, nil
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
		if plan.Original.LanProxy && (!plan.Config.LanProxy || oldPort != newPort) {
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
