package flows

import (
	"fmt"
	"strings"

	"github.com/Trilives/sboxkit/internal/i18n"
)

// selectorCurrent 返回 selector 中第一个真实节点，作为运行时 API 不可用时的兜底。
// 传入完整配置时也会排除 selector/urltest 子组；省略时仍会排除内置端点和订阅信息。
func selectorCurrent(group map[string]any, configs ...map[string]any) string {
	groupTags := map[string]bool{}
	if len(configs) > 0 {
		for _, outbound := range groupsOf(configs[0]) {
			tag, _ := outbound["tag"].(string)
			typ, _ := outbound["type"].(string)
			if groupTypes[typ] {
				groupTags[tag] = true
			}
		}
	}
	for _, name := range outboundsOf(group["outbounds"]) {
		if name != "" && !builtinNodes[name] && !isInfo(name) && !groupTags[name] {
			return name
		}
	}
	return ""
}

// currentNode 优先读取运行时选择；API 不可用或读取失败时退回配置中的首个真实节点。
func currentNode(group, cfg map[string]any, runtimeCurrent func(string) (string, error)) string {
	fallback := selectorCurrent(group, cfg)
	if runtimeCurrent == nil {
		return fallback
	}
	name, err := runtimeCurrent(fmt.Sprint(group["tag"]))
	if err != nil || strings.TrimSpace(name) == "" {
		return fallback
	}
	return name
}

// nodeMenuLabels 为 TTY 与非 TTY 菜单生成同一套节点标签。
func nodeMenuLabels(names []string, delays map[string]int, current string, apiOK bool) []string {
	labels := make([]string, len(names))
	for i, name := range names {
		parts := []string{name}
		if name == current {
			parts = append(parts, i18n.T("当前"))
		} else if apiOK {
			parts = append(parts, fmtDelay(delays, name))
		}
		labels[i] = strings.Join(parts, "   ")
	}
	return labels
}
