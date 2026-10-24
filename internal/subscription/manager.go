// 命名订阅管理（对应 manager.py）：增 / 删 / 改名 / 切换 active / 刷新 / 列表。
//
// 每个订阅存于 state/subscriptions/<name>/：meta.json + raw.* + config.json。
// active 指针（state/active）决定哪份部署生效；切换会同步 state/config.json 并重启服务。
// 生成、刷新、重建与切换都先校验候选配置再落盘（见 candidate.go）。
//
// 与 mihomo 版最大的不同：sing-box 不能直接解析 Clash 配置，因此这里按来源类型
// 分流到 internal/converter：clash → ClashToSingBox；sing-box 原生 → SingBoxDirect；
// base64 → 先经 ToClashDict 转成 Clash 字典，再走 ClashToSingBox。分流分组
// （AI / 流媒体 / 地区自动测速组）与部署字段覆写全部在 converter 内一次性生成，
// 不再有独立的 patch / overlay / regiongroups 步骤。
package subscription

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Trilives/sboxkit/internal/config"
	"github.com/Trilives/sboxkit/internal/execx"
	"github.com/Trilives/sboxkit/internal/i18n"
	"github.com/Trilives/sboxkit/internal/jsonx"
	"github.com/Trilives/sboxkit/internal/paths"
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

// Add 新增订阅（拉取 → 生成并校验配置 → 写盘）；setActive 时切换生效并同步服务。
// fetchViaProxy 为 false（默认）时本次拉取强制直连，忽略 customize.json 里配置的
// 下载代理——避免像 api.github.com 这类被单独封锁的域名因为下载代理默认启用而
// 不必要地绕路，也让用户能按订阅逐个选择是否需要代理。
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
	raw, err := fetchRaw(sub, cfg)
	if err != nil {
		return nil, err
	}
	return sub, regenerate(p, sub, raw, cfg, setActive)
}

// Refresh 联网重新拉取订阅原文并重建（用于「刷新订阅」/ 定时更新）；
// 沿用 customize.json 当前的下载代理设置（不像 Add 那样可按次选择）。
func Refresh(p paths.Paths, name string) (*Subscription, error) {
	sub := Get(p, name)
	if sub == nil {
		return nil, fmt.Errorf(i18n.T("订阅不存在: %s"), name)
	}
	sub.UpdatedAt = now()
	cfg := config.Load(p)
	raw, err := fetchRaw(sub, cfg)
	if err != nil {
		return nil, err
	}
	return sub, regenerate(p, sub, raw, cfg, isActive(p, name))
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
	return sub, regenerate(p, sub, raw, config.Load(p), isActive(p, name))
}

// regenerate 生成并校验候选配置，通过后写入（activate 时切换生效并同步服务）。
func regenerate(p paths.Paths, sub *Subscription, raw []byte, cfg config.Config, activate bool) error {
	c, err := prepareCandidate(p, sub, raw, cfg)
	if err != nil {
		return err
	}
	if err := c.deploy(p, activate); err != nil {
		return err
	}
	execx.Ok(fmt.Sprintf(i18n.T("订阅「%s」就绪：%v 个节点"), sub.Name, sub.LastNodeCount))
	return nil
}

func isActive(p paths.Paths, name string) bool {
	active := GetActive(p)
	return active != nil && active.Name == name
}

// --------------------------------------------------------------------------
// 切换 / 删除 / 改名
// --------------------------------------------------------------------------

// Switch 切换生效订阅：先用当前内核校验该订阅已存的配置，通过后才替换生效配置、
// active 指针并同步服务。
func Switch(p paths.Paths, name string) error {
	sub := Get(p, name)
	if sub == nil {
		return fmt.Errorf(i18n.T("订阅不存在: %s"), name)
	}
	data, err := os.ReadFile(configFile(p, name))
	if err != nil {
		return err
	}
	if err := checkConfig(p, data); err != nil {
		return fmt.Errorf(i18n.T("订阅「%s」的配置未通过校验，未切换（原配置与服务保持不变）；可先对它执行「重新生成配置」：%w"), name, err)
	}
	if err := (&candidate{sub: sub, config: data}).deploy(p, true); err != nil {
		return err
	}
	execx.Ok(i18n.T("已切换生效订阅: ") + name)
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
