package flows

import (
	"fmt"
	"strings"

	"github.com/Trilives/sboxkit/internal/config"
	"github.com/Trilives/sboxkit/internal/execx"
	"github.com/Trilives/sboxkit/internal/i18n"
	"github.com/Trilives/sboxkit/internal/paths"
	"github.com/Trilives/sboxkit/internal/tui"
)

func selectorGroups(cfg map[string]any) []map[string]any {
	var selects []map[string]any
	for _, g := range groupsOf(cfg) {
		if t, _ := g["type"].(string); t == "selector" {
			selects = append(selects, g)
		}
	}
	return selects
}

func matchGroupByKeyword(groups []map[string]any, keywords []string) map[string]any {
	for _, keyword := range keywords {
		needle := strings.ToLower(strings.TrimSpace(keyword))
		if needle == "" {
			continue
		}
		for _, g := range groups {
			tag := fmt.Sprint(g["tag"])
			if strings.Contains(strings.ToLower(tag), needle) {
				return g
			}
		}
	}
	return nil
}

// pickGroup 定位目标分组：forced 指定时精确匹配 tag；否则先尝试 defaultTag，
// 再按 main_group_keywords 顺序做包含匹配。两者都找不到时返回 nil，交互层可
// 提示用户补充识别关键词，而不是静默退化到其他 selector 分组。
func pickGroup(cfg map[string]any, forced, defaultTag string, keywords []string) (map[string]any, error) {
	selects := selectorGroups(cfg)
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
	return matchGroupByKeyword(selects, keywords), nil
}

func selectorTags(groups []map[string]any) []string {
	tags := make([]string, 0, len(groups))
	for _, g := range groups {
		tag := strings.TrimSpace(fmt.Sprint(g["tag"]))
		if tag != "" {
			tags = append(tags, tag)
		}
	}
	return tags
}

func prependUnique(items []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return append([]string(nil), items...)
	}
	out := []string{value}
	for _, item := range items {
		if !strings.EqualFold(strings.TrimSpace(item), value) {
			out = append(out, item)
		}
	}
	return out
}

// addMainGroupKeyword 只接受能匹配当前 selector 的关键词，并返回一份新配置。
// 输入配置及其切片保持不变，调用方可在确认有效后再持久化。
func addMainGroupKeyword(settings config.Config, selectors []map[string]any, value string) (config.Config, bool) {
	value = strings.TrimSpace(value)
	if value == "" || matchGroupByKeyword(selectors, []string{value}) == nil {
		return settings, false
	}
	updated := settings
	updated.MainGroupKeywords = prependUnique(settings.MainGroupKeywords, value)
	return updated, true
}

func promptMainGroupKeywordWith(
	p paths.Paths,
	selectors []map[string]any,
	ask func(string, tui.AskOpts) (string, error),
) (bool, error) {
	tags := selectorTags(selectors)
	if len(tags) > 0 {
		execx.Warn(fmt.Sprintf(i18n.T("未识别到主选择组；当前 selector 分组：%s"), strings.Join(tags, ", ")))
	}
	value, err := ask(i18n.T("输入主选择组识别关键词（留空跳过）"), tui.AskOpts{AllowEmpty: true})
	if err != nil {
		return false, err
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return false, nil
	}
	settings, added := addMainGroupKeyword(config.Load(p), selectors, value)
	if !added {
		return false, nil
	}
	if err := config.Save(p, settings); err != nil {
		return false, err
	}
	execx.Ok(fmt.Sprintf(i18n.T("主选择组识别关键词已保存：%s"), value))
	return true, nil
}

func resolveTargetGroup(
	p paths.Paths,
	cfg map[string]any,
	group string,
	ask func(string, tui.AskOpts) (string, error),
) (map[string]any, error) {
	settings := config.Load(p)
	target, err := pickGroup(cfg, group, settings.DefaultOutbound, settings.MainGroupKeywords)
	if err != nil || target != nil {
		return target, err
	}
	selectors := selectorGroups(cfg)
	if added, err := promptMainGroupKeywordWith(p, selectors, ask); err != nil {
		return nil, err
	} else if !added {
		return nil, missingMainGroupError()
	}
	settings = config.Load(p)
	target, err = pickGroup(cfg, group, settings.DefaultOutbound, settings.MainGroupKeywords)
	if err != nil {
		return nil, err
	}
	if target == nil {
		return nil, missingMainGroupError()
	}
	return target, nil
}

func missingMainGroupError() error {
	return fmt.Errorf("%s", i18n.T("未识别到主选择组，无法切换节点"))
}
