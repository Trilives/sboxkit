// 订阅生成的候选流程：拉取或读取原文 → 在内存中转换为候选配置 → 用状态目录现用的
// 内核与规则集执行 `sing-box check` → 通过后才经 deploy 事务写入订阅原文、生成配置、
// meta，以及（生效订阅）state/config.json 与 active 指针并同步服务。任一步失败都
// 保留原配置、原订阅与原服务；服务同步时还会用即将部署的内核再校验一次（见 sysd）。
package subscription

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Trilives/sboxkit/internal/compat"
	"github.com/Trilives/sboxkit/internal/config"
	"github.com/Trilives/sboxkit/internal/converter"
	"github.com/Trilives/sboxkit/internal/deploy"
	"github.com/Trilives/sboxkit/internal/execx"
	"github.com/Trilives/sboxkit/internal/i18n"
	"github.com/Trilives/sboxkit/internal/jsonx"
	"github.com/Trilives/sboxkit/internal/paths"
	"github.com/Trilives/sboxkit/internal/txn"
)

// candidate 一份已校验、尚未落盘的订阅配置。raw 为 nil 表示订阅文件不变、只切换生效（Switch）。
type candidate struct {
	sub    *Subscription
	raw    []byte
	config []byte
}

// prepareCandidate 在内存中转换并校验，不写盘。
func prepareCandidate(p paths.Paths, sub *Subscription, raw []byte, cfg config.Config) (*candidate, error) {
	cfgJSON, err := convert(p, sub, raw, cfg)
	if err != nil {
		return nil, err
	}
	if err := checkConfig(p, cfgJSON); err != nil {
		return nil, fmt.Errorf(i18n.T("订阅「%s」新生成的配置未通过校验，原配置、订阅与服务均保持不变：%w"), sub.Name, err)
	}
	return &candidate{sub: sub, raw: raw, config: cfgJSON}, nil
}

// checkConfig 用状态目录现用的内核与规则集校验；尚无内核（首次初始化下载前）时
// 推迟到注册服务时校验。
func checkConfig(p paths.Paths, cfgJSON []byte) error {
	execx.Info(i18n.T("用当前内核校验生成的配置…"))
	err := compat.Check(p, cfgJSON, compat.StateBundle(p))
	if errors.Is(err, compat.ErrKernelMissing) {
		execx.Warn(i18n.T("尚无 sing-box 内核，暂无法校验配置；注册服务前会再次校验。"))
		return nil
	}
	return err
}

// deploy 写入候选；activate 时同时替换生效配置与 active 指针并同步服务。
func (c *candidate) deploy(p paths.Paths, activate bool) error {
	return deploy.Apply(p, deploy.Change{
		Name:     fmt.Sprintf(i18n.T("部署订阅「%s」"), c.sub.Name),
		Snapshot: []string{p.SubscriptionDir(c.sub.Name), p.ConfigFile, p.ActiveFile},
		Commit:   func(*txn.Transaction) error { return c.write(p, activate) },
		Sync:     activate,
	})
}

func (c *candidate) write(p paths.Paths, activate bool) error {
	if err := p.EnsureStateDirs(); err != nil {
		return err
	}
	if c.raw != nil {
		if err := c.writeSubscription(p); err != nil {
			return err
		}
	}
	if !activate {
		return nil
	}
	if err := writeAtomic(p.ConfigFile, c.config); err != nil {
		return err
	}
	return writeAtomic(p.ActiveFile, []byte(c.sub.Name+"\n"))
}

func (c *candidate) writeSubscription(p paths.Paths) error {
	if err := os.MkdirAll(p.SubscriptionDir(c.sub.Name), 0o755); err != nil {
		return err
	}
	if err := writeAtomic(rawFile(p, c.sub), c.raw); err != nil {
		return err
	}
	if err := writeAtomic(configFile(p, c.sub.Name), c.config); err != nil {
		return err
	}
	metaJSON, err := jsonx.MarshalPretty(c.sub)
	if err != nil {
		return err
	}
	return writeAtomic(metaFile(p, c.sub.Name), metaJSON)
}

// Generate 用当前生成器从本地已存原文重新生成订阅配置（不写盘、不校验），供更新
// 内核 / 规则集 / sboxkit 前检查「生成器 + 内核 + 规则集 + 生效订阅」组合。
func Generate(p paths.Paths, name string) ([]byte, error) {
	sub := Get(p, name)
	if sub == nil {
		return nil, fmt.Errorf(i18n.T("订阅不存在: %s"), name)
	}
	raw, err := os.ReadFile(rawFile(p, sub))
	if err != nil {
		return nil, fmt.Errorf(i18n.T("读取订阅「%s」的本地原文：%w"), name, err)
	}
	return convert(p, sub, raw, config.Load(p))
}

// StoreGenerated 把已校验的重新生成结果写为订阅配置与生效配置；事务与服务同步由调用方负责。
func StoreGenerated(p paths.Paths, name string, cfgJSON []byte) error {
	if err := writeAtomic(configFile(p, name), cfgJSON); err != nil {
		return err
	}
	return writeAtomic(p.ConfigFile, cfgJSON)
}

// fetchRaw 拉取（或读本地文件）订阅原文；cfg.DownloadProxy 决定本次拉取是否走下载代理
// （调用方按场景决定传入原样加载的 cfg 还是清空代理后的）。
func fetchRaw(sub *Subscription, cfg config.Config) ([]byte, error) {
	if sub.SourceType == "local" {
		// URL 字段复用为本地文件绝对路径（由 flows.resolveLocalPath 校验过）；
		// 不联网，也不做 WarnIfMismatch——那是给"URL 拉取内容 vs 声明类型"的
		// 探测，本地文件是用户直接指定的，没有这个歧义。
		execx.Info(fmt.Sprintf(i18n.T("读取本地文件生成订阅「%s」…"), sub.Name))
		raw, err := os.ReadFile(sub.URL)
		if err != nil {
			return nil, fmt.Errorf(i18n.T("读取本地文件: %w"), err)
		}
		return raw, nil
	}
	execx.Info(fmt.Sprintf(i18n.T("拉取订阅「%s」…"), sub.Name))
	raw, err := Fetch(sub.URL, sub.SourceType, cfg.DownloadProxy)
	if err != nil {
		return nil, err
	}
	if msg := WarnIfMismatch(SourceKind(sub.SourceType), raw); msg != "" {
		execx.Warn(msg)
	}
	return raw, nil
}

// convert 把订阅原文转换为 sing-box 配置 JSON，并更新 sub.LastNodeCount。
//
// 来源类型决定走哪条转换路径：
//   - clash：converter.ClashToSingBox 现场生成整份 sing-box 配置（inbounds / dns /
//     route / outbounds 分组，含 AI / 流媒体 / 地区自动测速组，均一次性生成）。
//   - sing-box：converter.SingBoxDirect；ApplyOverlay 复用为「是否用 sboxkit 统一
//     规则重建」开关——关闭则仅信任用户原配置、只修补 clash_api；开启则提取
//     节点重新走一遍与 clash 来源一致的生成管线（分组 / TUN / DNS 与其它订阅一致）。
//   - base64：先经 ToClashDict 转成 Clash 字典，序列化后走 ClashToSingBox。
//
// "local" 来源只是拉取方式的旁路标记（URL 字段复用为本地文件路径，fetchRaw 时
// os.ReadFile 而非联网拉取），并不代表内容格式——本地文件既可以是 Clash YAML
// 也可以是 sing-box JSON（见 askNewSubscription 的提示文案）。sub.SourceType
// 本身继续保留字面量 "local"（Refresh 依赖它判断要不要重新按本地路径读取，
// 而不是当成 URL 去发起 HTTP 请求），真正决定走哪条转换路径的是这里按内容
// 现场探测出的 kind，无法判断时回退按 Clash YAML 处理。
func convert(p paths.Paths, sub *Subscription, raw []byte, cfg config.Config) ([]byte, error) {
	result, info, err := convertSource(p, sub, raw, cfg)
	if err != nil {
		return nil, err
	}
	warnSkipped(info)
	if n, ok := info["converted"].(int); ok {
		sub.LastNodeCount = n
	} else if n, ok := info["auto_count"].(int); ok {
		sub.LastNodeCount = n
	}
	return jsonx.MarshalPretty(result)
}

func convertSource(p paths.Paths, sub *Subscription, raw []byte, cfg config.Config) (converter.Config, converter.Info, error) {
	text := string(raw)
	kind := SourceKind(sub.SourceType)
	if sub.SourceType == "local" {
		kind = SourceClash
		if detected := Detect(raw); detected != SourceUnknown {
			kind = detected
		}
	}
	switch kind {
	case SourceSingBox:
		execx.Info(i18n.T("生成 sing-box 配置（原生订阅）…"))
		return converter.SingBoxDirect(text, cfg, p, sub.ApplyOverlay)
	case SourceBase64:
		execx.Info(i18n.T("经 subconverter/本地解析将 base64 转为 Clash…"))
		clash, err := ToClashDict(text, cfg)
		if err != nil {
			return converter.Config{}, nil, err
		}
		clashJSON, err := json.Marshal(clash)
		if err != nil {
			return converter.Config{}, nil, err
		}
		execx.Info(i18n.T("生成 sing-box 配置…"))
		return converter.ClashToSingBox(string(clashJSON), cfg, p)
	default: // clash（含未探测出具体格式的本地文件，回退按 Clash YAML 处理）
		execx.Info(i18n.T("生成 sing-box 配置…"))
		return converter.ClashToSingBox(text, cfg, p)
	}
}

// writeAtomic 同目录临时文件 + 改名：中途中断不会留下截断的状态文件。
func writeAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func warnSkipped(info converter.Info) {
	for _, item := range []struct{ key, msg string }{
		{"fake_ip_filter_skipped", "以下 fake-ip-filter 依赖外部规则集，尚未转换：%s"},
		{"custom_fake_ip_filter_skipped", "以下用户追加的 fake-ip-filter 依赖外部规则集，尚未转换：%s"},
		{"host_entries_skipped", "以下 hosts 条目无法安全转换，已跳过：%s"},
	} {
		if skipped, ok := info[item.key].([]string); ok && len(skipped) > 0 {
			execx.Warn(fmt.Sprintf(i18n.T(item.msg), strings.Join(skipped, ", ")))
		}
	}
}
