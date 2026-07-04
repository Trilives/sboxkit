// 来源类型识别：校验拉取内容与用户所选类型是否相符（也支持启发式判断）。
//
// 与 mihomo 版最大的不同：mihomo 只需区分 clash（直接可用）/ base64（需转换），
// 因为 mihomo 自己也吃 Clash 配置；sing-box 有自己的原生订阅格式（JSON，字段
// 为 outbounds），所以这里是三态：clash / sing-box / base64。
package subscription

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Trilives/sboxkit/internal/i18n"
)

type SourceKind string

const (
	SourceClash   SourceKind = "clash"
	SourceSingBox SourceKind = "sing-box"
	SourceBase64  SourceKind = "base64"
	SourceUnknown SourceKind = "unknown"
)

// Detect 启发式判断订阅类型：返回 clash | sing-box | base64 | unknown。
func Detect(raw []byte) SourceKind {
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return SourceUnknown
	}

	// sing-box：JSON 且含 outbounds 列表
	if looksLikeSingBox(text) {
		return SourceSingBox
	}

	// clash：YAML 且含 proxies 列表
	if strings.Contains(text, "proxies:") || strings.Contains(text, "proxy-groups:") {
		var d any
		if err := yaml.Unmarshal([]byte(text), &d); err == nil {
			if m, ok := d.(map[string]any); ok {
				if _, ok := m["proxies"].([]any); ok {
					return SourceClash
				}
			}
		}
	}

	// base64：可解码且含节点分享链接
	if looksBase64(text) {
		compact := strings.Join(strings.Fields(text), "")
		if pad := len(compact) % 4; pad != 0 {
			compact += strings.Repeat("=", 4-pad)
		}
		if decoded, err := base64.StdEncoding.DecodeString(compact); err == nil {
			if strings.Contains(string(decoded), "://") {
				return SourceBase64
			}
		}
	}

	return SourceUnknown
}

func looksLikeSingBox(text string) bool {
	var data map[string]any
	if err := json.Unmarshal([]byte(text), &data); err != nil {
		return false
	}
	_, ok := data["outbounds"]
	return ok
}

func looksBase64(text string) bool {
	sample := strings.Join(strings.Fields(text), "")
	if len(sample) < 16 {
		return false
	}
	const allowed = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/=-_"
	for _, c := range sample {
		if !strings.ContainsRune(allowed, c) {
			return false
		}
	}
	return true
}

// WarnIfMismatch 若检测类型与声明不符，返回提示文本；相符或无法判断返回空串。
func WarnIfMismatch(declared SourceKind, raw []byte) string {
	found := Detect(raw)
	if found != SourceUnknown && found != declared {
		return fmt.Sprintf(i18n.T("内容看起来更像「%s」而非你选择的「%s」。"), found, declared)
	}
	return ""
}
