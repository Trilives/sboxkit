// 交互式切换 / 固定首选节点（对应 node_select.py）。
//
// sing-box 的运行时配置把节点和分组混在同一个 outbounds 数组里（不像 Clash 的
// proxies / proxy-groups 分开两个顶层字段）：分组是 type=selector/urltest 的
// outbound，条目里的 tag 是分组名、outbounds 字段是成员 tag 列表；真实节点是
// 其余类型（vmess/trojan/vless/ss/...）的 outbound。目标分组的挑选也更简单：
// converter 生成配置时把主选择组固定命名为 cfg.DefaultOutbound（默认
// "Proxy"），不需要再像 mihomo 版那样按用户自定义关键词逐个猜测分组名。
//
// 把选中项设为目标分组（默认 cfg.DefaultOutbound）的第一个成员，使重启后稳定
// 停在该节点；服务在跑时还经 Clash API 实时切换，并并发实测延迟。选组持久化由
// cache_file（experimental.cache_file）负责；改写成员顺序作为跨重启兜底。
package flows

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/term"

	"github.com/Trilives/sboxkit/internal/clashapi"
	"github.com/Trilives/sboxkit/internal/config"
	"github.com/Trilives/sboxkit/internal/configfile"
	"github.com/Trilives/sboxkit/internal/errs"
	"github.com/Trilives/sboxkit/internal/execx"
	"github.com/Trilives/sboxkit/internal/i18n"
	"github.com/Trilives/sboxkit/internal/jsonx"
	"github.com/Trilives/sboxkit/internal/paths"
	"github.com/Trilives/sboxkit/internal/subscription"
	"github.com/Trilives/sboxkit/internal/sysd"
	"github.com/Trilives/sboxkit/internal/tui"
)

// sing-box 分组类型（可作为「子组」展示；对应 mihomo 的 select/url-test 等）。
var groupTypes = map[string]bool{"selector": true, "urltest": true}

// 非可选节点的伪 outbound（分组本身之外的内置端点）。
var builtinNodes = map[string]bool{"DIRECT": true, "BLOCK": true}

var infoKeywords = []string{"Traffic:", "Expire:", "剩余流量", "过期时间", "剩余", "套餐", "官网", "订阅", "重置"}

type region struct {
	key   string
	label string
	kws   []string
}

var regions = []region{
	{"hk", "🇭🇰 香港", []string{"香港", "hong kong", "hongkong"}},
	{"tw", "🇹🇼 台湾", []string{"台湾", "臺灣", "taiwan"}},
	{"jp", "🇯🇵 日本", []string{"日本", "japan", "东京", "大阪"}},
	{"kr", "🇰🇷 韩国", []string{"韩国", "韓國", "korea", "首尔"}},
	{"sg", "🇸🇬 新加坡", []string{"新加坡", "singapore", "狮城", "獅城"}},
	{"us", "🇺🇸 美国", []string{"美国", "united states", "america", "硅谷", "洛杉矶", "圣何塞"}},
}

const otherKey, otherLabel = "other", "🌐 其他地区"

func groupsOf(cfg map[string]any) []map[string]any {
	gs, ok := cfg["outbounds"].([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(gs))
	for _, g := range gs {
		if m, ok := g.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// pickGroup 定位目标分组：forced 指定时精确匹配 tag；否则用 defaultTag
// （通常是 cfg.DefaultOutbound，即 converter 生成时固定命名的主选择组，例如
// "Proxy"）；两者都找不到则退化为成员数最多的 selector 分组。
func pickGroup(cfg map[string]any, forced, defaultTag string) (map[string]any, error) {
	var selects []map[string]any
	for _, g := range groupsOf(cfg) {
		if t, _ := g["type"].(string); t == "selector" {
			selects = append(selects, g)
		}
	}
	if len(selects) == 0 {
		return nil, fmt.Errorf("%s", i18n.T("配置里没有 selector 分组，无法切换节点"))
	}
	if forced != "" {
		for _, g := range selects {
			if g["tag"] == forced {
				return g, nil
			}
		}
		return nil, fmt.Errorf(i18n.T("指定分组 '%s' 不存在"), forced)
	}
	if defaultTag != "" {
		for _, g := range selects {
			if g["tag"] == defaultTag {
				return g, nil
			}
		}
	}
	best := selects[0]
	for _, g := range selects[1:] {
		if lenAnyList(g["outbounds"]) > lenAnyList(best["outbounds"]) {
			best = g
		}
	}
	return best, nil
}

func lenAnyList(v any) int {
	switch x := v.(type) {
	case []any:
		return len(x)
	case []string:
		return len(x)
	}
	return 0
}

func classify(name string) string {
	low := strings.ToLower(name)
	for _, r := range regions {
		for _, kw := range r.kws {
			if strings.Contains(name, kw) || strings.Contains(low, kw) {
				return r.key
			}
		}
	}
	return otherKey
}

func isInfo(name string) bool {
	for _, kw := range infoKeywords {
		if strings.Contains(name, kw) {
			return true
		}
	}
	return false
}

// outboundsOf 统一取一个 outbound 条目的成员 tag 列表（[]any 或 []string 均可）。
func outboundsOf(v any) []string {
	switch x := v.(type) {
	case []any:
		out := make([]string, 0, len(x))
		for _, e := range x {
			out = append(out, fmt.Sprint(e))
		}
		return out
	case []string:
		return append([]string(nil), x...)
	}
	return nil
}

// collectMembers 把分组成员分为「按地区分桶的真实节点」与「子组」。
func collectMembers(cfg, group map[string]any) (map[string][]string, []string) {
	typeByTag := map[string]string{}
	for _, g := range groupsOf(cfg) {
		tag, _ := g["tag"].(string)
		t, _ := g["type"].(string)
		typeByTag[tag] = t
	}
	buckets := map[string][]string{}
	var subgroups []string
	for _, name := range outboundsOf(group["outbounds"]) {
		switch {
		case groupTypes[typeByTag[name]]:
			subgroups = append(subgroups, name)
		case builtinNodes[name] || isInfo(name):
		default:
			buckets[classify(name)] = append(buckets[classify(name)], name)
		}
	}
	return buckets, subgroups
}

// measure 并发实测延迟，带 TTY 进度。
func measure(api *clashapi.Client, names []string) map[string]int {
	if len(names) == 0 {
		return nil
	}
	tty := term.IsTerminal(int(os.Stdout.Fd()))
	if !tty {
		execx.Info(fmt.Sprintf(i18n.T("测速中（%d 个节点）…"), len(names)))
	}
	results := make(map[string]int, len(names))
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, min(16, len(names)))
	done := 0
	for _, name := range names {
		wg.Add(1)
		go func(n string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			ms, ok := api.Delay(n)
			mu.Lock()
			if ok {
				results[n] = ms
			}
			done++
			if tty {
				fmt.Printf(i18n.T("\r\033[K  测速中… %d/%d"), done, len(names))
			}
			mu.Unlock()
		}(name)
	}
	wg.Wait()
	if tty {
		fmt.Print("\r\033[K")
	}
	execx.Ok(fmt.Sprintf(i18n.T("测速完成：%d/%d 可用"), len(results), len(names)))
	return results
}

func fmtDelay(results map[string]int, name string) string {
	if ms, ok := results[name]; ok {
		return fmt.Sprintf("%dms", ms)
	}
	return i18n.T("超时")
}

// persistFirst 把选中节点提为目标分组首成员，双写生效配置与订阅配置（跨重启兜底）。
func persistFirst(cfg map[string]any, groupTag, node string, files []string) error {
	for _, g := range groupsOf(cfg) {
		if t, _ := g["type"].(string); t == "selector" && g["tag"] == groupTag {
			members := outboundsOf(g["outbounds"])
			out := make([]string, 0, len(members)+1)
			out = append(out, node)
			for _, m := range members {
				if m != node {
					out = append(out, m)
				}
			}
			g["outbounds"] = out
			break
		}
	}
	payload, err := jsonx.MarshalPretty(cfg)
	if err != nil {
		return err
	}
	for _, f := range files {
		tmp := f + ".tmp"
		if err := os.WriteFile(tmp, payload, 0o644); err != nil {
			return err
		}
		if err := os.Rename(tmp, f); err != nil {
			return err
		}
	}
	return nil
}

// pickResult 两级菜单选完节点后的结果，供临时切换 / 固定切换两个流程各自处理。
type pickResult struct {
	cfg       map[string]any
	groupName string
	node      string
	api       *clashapi.Client
	apiOK     bool
}

// pickNode 两级菜单（地区/分组 → 节点）交互选择，不做任何写盘/切换——
// 是「节点切换」与「固定节点」两个流程共用的选择器。
func pickNode(p paths.Paths, configPath, group string) (*pickResult, error) {
	cfg, err := configfile.Read(configPath)
	if err != nil {
		return nil, err
	}
	defaultTag := config.Load(p).DefaultOutbound
	target, err := pickGroup(cfg, group, defaultTag)
	if err != nil {
		return nil, err
	}
	groupName := fmt.Sprint(target["tag"])
	buckets, subgroups := collectMembers(cfg, target)
	if len(buckets) == 0 && len(subgroups) == 0 {
		return nil, fmt.Errorf(i18n.T("分组 '%s' 下没有可选项"), groupName)
	}

	// 节点切换走 Clash API 热切换，直接连 API 实时测速/切换
	api := clashapi.FromConfig(cfg)
	apiOK := api != nil && api.Reachable()
	if apiOK {
		execx.Info(i18n.T("已连上 Clash API，列表将实时测速。"))
	} else {
		execx.Info(i18n.T("Clash API 不可达，跳过测速。"))
	}

	type menuEntry struct {
		label string
		items []string
	}
	var firstMenu []menuEntry
	for _, r := range regions {
		if len(buckets[r.key]) > 0 {
			firstMenu = append(firstMenu, menuEntry{i18n.T(r.label), buckets[r.key]})
		}
	}
	if len(buckets[otherKey]) > 0 {
		firstMenu = append(firstMenu, menuEntry{i18n.T(otherLabel), buckets[otherKey]})
	}
	if len(subgroups) > 0 {
		firstMenu = append(firstMenu, menuEntry{i18n.T("🧭 子组（自动测速 / 故障转移）"), subgroups})
	}

	// esc 在第二步只退回第一步；^R 才穿透放弃本次切换
	var selected string
	idx := 0
	for {
		labels := make([]string, len(firstMenu))
		for i, e := range firstMenu {
			labels[i] = fmt.Sprintf(i18n.T("%s（%d）"), e.label, len(e.items))
		}
		i, err := tui.Select(i18n.T("选择地区 / 分组"), labels, tui.SelectOpts{BackLabel: i18n.T("退出切换节点"), Initial: idx})
		if err != nil {
			return nil, err
		}
		idx = i
		entry := firstMenu[i]

		var delays map[string]int
		if apiOK {
			delays = measure(api, entry.items)
		}
		nodeLabels := make([]string, len(entry.items))
		for j, name := range entry.items {
			if apiOK {
				nodeLabels[j] = fmt.Sprintf("%s   %s", name, fmtDelay(delays, name))
			} else {
				nodeLabels[j] = name
			}
		}
		nidx, err := tui.Select(entry.label, nodeLabels, tui.SelectOpts{SaveLabel: i18n.T("返回地区/分组"), BackLabel: i18n.T("放弃并退出")})
		if err != nil {
			if errors.Is(err, errs.ErrSaveExit) {
				continue // 返回地区/分组选择，重新选
			}
			return nil, err
		}
		selected = entry.items[nidx]
		break
	}
	return &pickResult{cfg: cfg, groupName: groupName, node: selected, api: api, apiOK: apiOK}, nil
}

// NodeSwitchLive 临时切换节点：仅经 Clash API 热切换，不写盘、不重启——
// 服务重启或切换/刷新订阅后失效，适合"先试试看"的场景。需要服务正在运行。
func NodeSwitchLive(p paths.Paths, configPath, group string) error {
	if configPath == "" {
		configPath = p.ConfigFile
	}
	r, err := pickNode(p, configPath, group)
	if err != nil {
		return err
	}
	if !r.apiOK {
		return fmt.Errorf("%s", i18n.T("Clash API 不可达，临时切换需要服务正在运行（如需跨重启保留，请改用「固定节点」）"))
	}
	if err := r.api.Switch(r.groupName, r.node); err != nil {
		return err
	}
	execx.Ok(fmt.Sprintf(i18n.T("已临时切换 %s → %s（不写盘，重启/切换订阅后失效）"), r.groupName, r.node))
	return nil
}

// NodeSelect 两级菜单（地区/分组 → 节点）切换节点；是否固定为首选（写盘，
// 跨重启/服务重建后仍保留）由用户显式确认，只有选择固定时才会问是否重启服务。
func NodeSelect(p paths.Paths, configPath, group string) error {
	if configPath == "" {
		configPath = p.ConfigFile
	}
	r, err := pickNode(p, configPath, group)
	if err != nil {
		return err
	}

	if r.apiOK {
		if err := r.api.Switch(r.groupName, r.node); err != nil {
			execx.Warn(fmt.Sprintf(i18n.T("Clash API 实时切换失败：%v"), err))
		} else {
			execx.Ok(fmt.Sprintf(i18n.T("已通过 Clash API 实时切换 %s → %s"), r.groupName, r.node))
		}
	}

	pin, err := tui.Confirm(i18n.T("固定为该分组首选节点？（写入配置，跨重启/切换订阅后仍保留；否则仅本次生效）"), true)
	if err != nil || !pin {
		return err
	}

	// 写生效配置 + 当前 active 订阅的 config.json（双写以跨重启持久）
	targets := []string{configPath}
	if active := subscription.GetActive(p); active != nil {
		subCfg := filepath.Join(p.SubscriptionDir(active.Name), "config.json")
		if _, err := os.Stat(subCfg); err == nil && subCfg != configPath {
			targets = append(targets, subCfg)
		}
	}
	if err := persistFirst(r.cfg, r.groupName, r.node, targets); err != nil {
		return err
	}
	execx.Ok(fmt.Sprintf(i18n.T("已固定 %s 首选 = %s"), r.groupName, r.node))

	if sysd.IsInstalled(sysd.DefaultName) {
		ok, err := tui.Confirm(i18n.T("重启服务以确保生效？"), false)
		if err == nil && ok {
			return sysd.SyncAndRestart(p, sysd.DefaultName)
		}
	}
	return nil
}
