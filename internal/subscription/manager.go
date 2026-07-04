// 命名订阅管理（对应 manager.py）：增 / 删 / 改名 / 切换 active / 刷新 / 列表。
//
// 每个订阅存于 state/subscriptions/<name>/：meta.json + raw.* + config.json。
// active 指针（state/active）决定哪份部署生效；切换会同步 state/config.json 并重启服务。
//
// 与 mihomo 版最大的不同：sing-box 不能直接解析 Clash 配置，因此这里按来源类型
// 分流到 internal/converter：clash → ClashToSingBox；sing-box 原生 → SingBoxDirect；
// base64 → 先经 ToClashDict 转成 Clash 字典，再走 ClashToSingBox。分流分组
// （AI / 流媒体 / 地区自动测速组）与部署字段覆写全部在 converter 内一次性生成，
// 不再有独立的 patch / overlay / regiongroups 步骤。
package subscription

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Trilives/sboxkit/internal/config"
	"github.com/Trilives/sboxkit/internal/converter"
	"github.com/Trilives/sboxkit/internal/execx"
	"github.com/Trilives/sboxkit/internal/i18n"
	"github.com/Trilives/sboxkit/internal/jsonx"
	"github.com/Trilives/sboxkit/internal/paths"
	"github.com/Trilives/sboxkit/internal/sysd"
	"github.com/Trilives/sboxkit/internal/uiassets"
)

// Subscription 元数据；JSON 字段与 Python 版 meta.json 完全一致（老数据直读）。
type Subscription struct {
	Name          string `json:"name"`
	URL           string `json:"url"`
	SourceType    string `json:"source_type"`
	ApplyOverlay  bool   `json:"apply_overlay"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
	LastNodeCount int    `json:"last_node_count"`
}

var rawExt = map[string]string{"clash": "yaml", "sing-box": "json", "base64": "txt", "local": "yaml"}

// now 对应 Python isoformat(timespec="seconds")：2026-07-03T12:34:56+00:00。
func now() string {
	return time.Now().UTC().Format("2006-01-02T15:04:05+00:00")
}

// Slug 订阅名清洗：折叠空白、去路径分隔符与 ".."，空名回退 "sub"。
func Slug(name string) string {
	name = strings.TrimSpace(name)
	name = strings.ReplaceAll(name, "/", "-")
	name = strings.ReplaceAll(name, "\\", "-")
	name = strings.ReplaceAll(name, "..", "-")
	name = strings.Join(strings.Fields(name), "-")
	name = strings.Trim(name, ". ")
	if name == "" {
		return "sub"
	}
	return name
}

func metaFile(p paths.Paths, name string) string {
	return filepath.Join(p.SubscriptionDir(name), "meta.json")
}

func configFile(p paths.Paths, name string) string {
	return filepath.Join(p.SubscriptionDir(name), "config.json")
}

func rawFile(p paths.Paths, sub *Subscription) string {
	ext, ok := rawExt[sub.SourceType]
	if !ok {
		ext = "txt"
	}
	return filepath.Join(p.SubscriptionDir(sub.Name), "raw."+ext)
}

// --------------------------------------------------------------------------
// 读取
// --------------------------------------------------------------------------

func ListAll(p paths.Paths) []Subscription {
	var subs []Subscription
	entries, err := os.ReadDir(p.Subscriptions)
	if err != nil {
		return subs
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, e := range entries {
		if sub := Get(p, e.Name()); sub != nil {
			subs = append(subs, *sub)
		}
	}
	return subs
}

func Get(p paths.Paths, name string) *Subscription {
	raw, err := os.ReadFile(metaFile(p, name))
	if err != nil {
		return nil
	}
	var sub Subscription
	if err := json.Unmarshal(raw, &sub); err != nil {
		return nil
	}
	return &sub
}

func GetActive(p paths.Paths) *Subscription {
	raw, err := os.ReadFile(p.ActiveFile)
	if err != nil {
		return nil
	}
	return Get(p, strings.TrimSpace(string(raw)))
}

// --------------------------------------------------------------------------
// 增 / 改
// --------------------------------------------------------------------------

// Add 新增订阅（拉取 → 生成配置）；setActive 时切换生效。fetchViaProxy 为 false
// （默认）时本次拉取强制直连，忽略 customize.json 里配置的下载代理——避免像
// api.github.com 这类被单独封锁的域名因为下载代理默认启用而不必要地绕路，
// 也让用户能按订阅逐个选择是否需要代理。
func Add(p paths.Paths, name, subURL, sourceType string, applyOverlay, setActive, fetchViaProxy bool) (*Subscription, error) {
	name = Slug(name)
	if _, err := os.Stat(metaFile(p, name)); err == nil {
		return nil, fmt.Errorf(i18n.T("订阅「%s」已存在，请改名或先删除"), name)
	}
	sub := &Subscription{
		Name: name, URL: subURL, SourceType: sourceType, ApplyOverlay: applyOverlay,
		CreatedAt: now(), UpdatedAt: now(),
	}
	cfg := config.Load(p)
	if !fetchViaProxy {
		cfg.DownloadProxy = ""
	}
	if err := build(p, sub, cfg); err != nil {
		return nil, err
	}
	if setActive {
		if err := applyActive(p, name); err != nil {
			return sub, err
		}
	}
	return sub, nil
}

// Refresh 联网重新拉取订阅原文并重建（用于「刷新订阅」/ 定时更新）；
// 沿用 customize.json 当前的下载代理设置（不像 Add 那样可按次选择）。
func Refresh(p paths.Paths, name string) (*Subscription, error) {
	sub := Get(p, name)
	if sub == nil {
		return nil, fmt.Errorf(i18n.T("订阅不存在: %s"), name)
	}
	sub.UpdatedAt = now()
	if err := build(p, sub, config.Load(p)); err != nil {
		return nil, err
	}
	if active := GetActive(p); active != nil && active.Name == name {
		if err := applyActive(p, name); err != nil {
			return sub, err
		}
	}
	return sub, nil
}

// Rebuild 基于本地已存订阅原文重新生成（不联网），用于应用定制层等本地改动；
// 本地无原文（异常情况）时回退为联网刷新。
func Rebuild(p paths.Paths, name string) (*Subscription, error) {
	sub := Get(p, name)
	if sub == nil {
		return nil, fmt.Errorf(i18n.T("订阅不存在: %s"), name)
	}
	raw, err := os.ReadFile(rawFile(p, sub))
	if err != nil {
		execx.Warn(i18n.T("本地缺少订阅原文，改为联网刷新。"))
		return Refresh(p, name)
	}
	sub.UpdatedAt = now()
	execx.Info(fmt.Sprintf(i18n.T("用本地原文重新生成「%s」（不重新拉取）…"), sub.Name))
	if err := convertAndWrite(p, sub, raw, config.Load(p)); err != nil {
		return nil, err
	}
	if active := GetActive(p); active != nil && active.Name == name {
		if err := applyActive(p, name); err != nil {
			return sub, err
		}
	}
	return sub, nil
}

// build 拉取（或读本地文件）→ 写 raw → 生成配置写盘；cfg.DownloadProxy 决定
// 本次拉取是否走下载代理（调用方按场景决定传入原样加载的 cfg 还是清空代理后的）。
func build(p paths.Paths, sub *Subscription, cfg config.Config) error {
	var raw []byte
	var err error
	if sub.SourceType == "local" {
		// URL 字段复用为本地文件绝对路径（由 flows.resolveLocalPath 校验过）；
		// 不联网，也不做 WarnIfMismatch——那是给"URL 拉取内容 vs 声明类型"的
		// 探测，本地文件是用户直接指定的，没有这个歧义。
		execx.Info(fmt.Sprintf(i18n.T("读取本地文件生成订阅「%s」…"), sub.Name))
		raw, err = os.ReadFile(sub.URL)
		if err != nil {
			return fmt.Errorf(i18n.T("读取本地文件: %w"), err)
		}
	} else {
		execx.Info(fmt.Sprintf(i18n.T("拉取订阅「%s」…"), sub.Name))
		raw, err = Fetch(sub.URL, sub.SourceType, cfg.DownloadProxy)
		if err != nil {
			return err
		}
		if msg := WarnIfMismatch(SourceKind(sub.SourceType), raw); msg != "" {
			execx.Warn(msg)
		}
	}
	if err := os.MkdirAll(p.SubscriptionDir(sub.Name), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(rawFile(p, sub), raw, 0o644); err != nil {
		return err
	}
	return convertAndWrite(p, sub, raw, cfg)
}

// convertAndWrite 把订阅原文转换为 sing-box 配置，写 config.json/meta。
//
// 来源类型决定走哪条转换路径：
//   - clash：converter.ClashToSingBox 现场生成整份 sing-box 配置（inbounds / dns /
//     route / outbounds 分组，含 AI / 流媒体 / 地区自动测速组，均一次性生成）。
//   - sing-box：converter.SingBoxDirect；ApplyOverlay 复用为「是否用 sboxkit 统一
//     规则重建」开关——关闭则仅信任用户原配置、只патч clash_api；开启则提取
//     节点重新走一遍与 clash 来源一致的生成管线（分组 / TUN / DNS 与其它订阅一致）。
//   - base64：先经 ToClashDict 转成 Clash 字典，序列化后走 ClashToSingBox。
//
// "local" 来源只是拉取方式的旁路标记（URL 字段复用为本地文件路径，build 时
// os.ReadFile 而非联网拉取），并不代表内容格式——本地文件既可以是 Clash YAML
// 也可以是 sing-box JSON（见 askNewSubscription 的提示文案）。sub.SourceType
// 本身继续保留字面量 "local"（Refresh/build 依赖它判断要不要重新按本地路径
// 读取，而不是当成 URL 去发起 HTTP 请求），真正决定走哪条转换路径的是这里
// 按内容现场探测出的 kind，无法判断时回退按 Clash YAML 处理。
func convertAndWrite(p paths.Paths, sub *Subscription, raw []byte, cfg config.Config) error {
	// 内置面板始终物化到 state/ui（sysd 部署运行时会把它连同内核一起暂存到
	// runtime 目录），与 mihomo 版当年下载 metacubexd 到同一位置的角色一致。
	if err := uiassets.Write(p.UI); err != nil {
		return err
	}

	text := string(raw)

	kind := SourceKind(sub.SourceType)
	if sub.SourceType == "local" {
		kind = SourceClash
		if detected := Detect(raw); detected != SourceUnknown {
			kind = detected
		}
	}

	var (
		result converter.Config
		info   converter.Info
		err    error
	)
	switch kind {
	case SourceSingBox:
		execx.Info(i18n.T("生成 sing-box 配置（原生订阅）…"))
		result, info, err = converter.SingBoxDirect(text, cfg, p, sub.ApplyOverlay)
	case SourceBase64:
		execx.Info(i18n.T("经 subconverter/本地解析将 base64 转为 Clash…"))
		clash, cerr := ToClashDict(text, cfg)
		if cerr != nil {
			return cerr
		}
		clashJSON, merr := json.Marshal(clash)
		if merr != nil {
			return merr
		}
		execx.Info(i18n.T("生成 sing-box 配置…"))
		result, info, err = converter.ClashToSingBox(string(clashJSON), cfg, p)
	default: // clash（含未探测出具体格式的本地文件，回退按 Clash YAML 处理）
		execx.Info(i18n.T("生成 sing-box 配置…"))
		result, info, err = converter.ClashToSingBox(text, cfg, p)
	}
	if err != nil {
		return err
	}

	if n, ok := info["converted"].(int); ok {
		sub.LastNodeCount = n
	} else if n, ok := info["auto_count"].(int); ok {
		sub.LastNodeCount = n
	}

	cfgJSON, err := jsonx.MarshalPretty(result)
	if err != nil {
		return err
	}
	if err := os.WriteFile(configFile(p, sub.Name), cfgJSON, 0o644); err != nil {
		return err
	}
	metaJSON, err := jsonx.MarshalPretty(sub)
	if err != nil {
		return err
	}
	if err := os.WriteFile(metaFile(p, sub.Name), metaJSON, 0o644); err != nil {
		return err
	}
	execx.Ok(fmt.Sprintf(i18n.T("订阅「%s」就绪：%v 个节点"), sub.Name, sub.LastNodeCount))
	return nil
}

// --------------------------------------------------------------------------
// 切换 / 删除 / 改名
// --------------------------------------------------------------------------

// Switch 切换生效订阅。
func Switch(p paths.Paths, name string) error {
	if _, err := os.Stat(metaFile(p, name)); err != nil {
		return fmt.Errorf(i18n.T("订阅不存在: %s"), name)
	}
	if err := applyActive(p, name); err != nil {
		return err
	}
	execx.Ok(i18n.T("已切换生效订阅: ") + name)
	return nil
}

func applyActive(p paths.Paths, name string) error {
	if err := p.EnsureStateDirs(); err != nil {
		return err
	}
	data, err := os.ReadFile(configFile(p, name))
	if err != nil {
		return err
	}
	if err := os.WriteFile(p.ConfigFile, data, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(p.ActiveFile, []byte(name+"\n"), 0o644); err != nil {
		return err
	}
	if sysd.IsInstalled(sysd.DefaultName) {
		if err := sysd.SyncAndRestart(p, sysd.DefaultName); err != nil {
			var ce *execx.CommandError
			if errors.As(err, &ce) || err != nil {
				execx.Warn(fmt.Sprintf(i18n.T("配置已切换，但同步到服务失败：%v"), err))
			}
		}
	}
	return nil
}

// RemoveSub 删除订阅目录；删除生效订阅时清掉 active 指针并提醒。
func RemoveSub(p paths.Paths, name string) error {
	d := p.SubscriptionDir(name)
	if _, err := os.Stat(d); err != nil {
		return fmt.Errorf(i18n.T("订阅不存在: %s"), name)
	}
	active := GetActive(p)
	wasActive := active != nil && active.Name == name
	os.RemoveAll(d)
	if wasActive {
		os.Remove(p.ActiveFile)
		execx.Warn(i18n.T("已删除当前生效订阅；请切换到其它订阅或重新添加。"))
	}
	execx.Ok(i18n.T("已删除订阅: ") + name)
	return nil
}

// Rename 订阅改名（目录改名 + meta/active 同步）。
func Rename(p paths.Paths, oldName, newName string) error {
	newName = Slug(newName)
	if _, err := os.Stat(metaFile(p, oldName)); err != nil {
		return fmt.Errorf(i18n.T("订阅不存在: %s"), oldName)
	}
	if _, err := os.Stat(metaFile(p, newName)); err == nil {
		return fmt.Errorf(i18n.T("目标名已存在: %s"), newName)
	}
	if err := os.Rename(p.SubscriptionDir(oldName), p.SubscriptionDir(newName)); err != nil {
		return err
	}
	if sub := Get(p, newName); sub != nil {
		sub.Name = newName
		sub.UpdatedAt = now()
		if metaJSON, err := jsonx.MarshalPretty(sub); err == nil {
			os.WriteFile(metaFile(p, newName), metaJSON, 0o644)
		}
	}
	if active := GetActive(p); active == nil {
		// active 指针指向旧名时 GetActive 已经找不到 meta —— 直接检查指针文件
		if raw, err := os.ReadFile(p.ActiveFile); err == nil && strings.TrimSpace(string(raw)) == oldName {
			os.WriteFile(p.ActiveFile, []byte(newName+"\n"), 0o644)
		}
	}
	execx.Ok(fmt.Sprintf(i18n.T("已改名: %s → %s"), oldName, newName))
	return nil
}
