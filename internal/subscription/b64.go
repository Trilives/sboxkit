// 通用 base64 订阅处理（对应 b64.py）。
//
// base64 来源先转成 Clash 风格的 proxy 字典（这一步与后端无关），再交给
// internal/converter.ClashToSingBox 生成 sing-box 配置：默认路径经云端
// subconverter（target=clash）转换；应急路径（base64_local_fallback，默认关闭）
// 本地直接解析节点分享链接为 Clash 风格 proxy 字典（best-effort，覆盖
// vmess/ss/trojan/vless/hysteria2/tuic，格式怪异时可能漏字段/转错）。
package subscription

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Trilives/sboxkit/internal/config"
	"github.com/Trilives/sboxkit/internal/execx"
	"github.com/Trilives/sboxkit/internal/i18n"
)

func b64decode(text string) ([]byte, error) {
	text = strings.TrimSpace(text)
	text = strings.ReplaceAll(text, "-", "+")
	text = strings.ReplaceAll(text, "_", "/")
	text = strings.Join(strings.Fields(text), "")
	if pad := len(text) % 4; pad != 0 {
		text += strings.Repeat("=", 4-pad)
	}
	return base64.StdEncoding.DecodeString(text)
}

// ToClashDict base64 内容 → Clash 配置 map（subconverter 优先，按需本地应急解析）。
func ToClashDict(rawText string, cfg config.Config) (map[string]any, error) {
	backend := cfg.SubconverterBackend
	proxy := cfg.DownloadProxy
	localFallback := cfg.Base64LocalFallback

	if backend != "" {
		clashYAML, err := toClashViaSubconverter(rawText, backend, proxy)
		if err == nil {
			var data any
			if yerr := yaml.Unmarshal([]byte(clashYAML), &data); yerr == nil {
				if m, ok := data.(map[string]any); ok {
					if _, ok := m["proxies"].([]any); ok {
						return m, nil
					}
				}
			}
			err = fmt.Errorf("%s", i18n.T("subconverter 返回内容无法解析为 Clash"))
		}
		if err != nil {
			if !localFallback {
				return nil, fmt.Errorf(i18n.T("subconverter 解析失败：%w。可更换后端，或开启应急本地解析 base64_local_fallback"), err)
			}
			execx.Warn(fmt.Sprintf(i18n.T("subconverter 失败，改用应急本地解析：%v"), err))
		}
	} else if !localFallback {
		return nil, fmt.Errorf("%s", i18n.T("未配置 subconverter 后端，且未开启应急本地解析（base64_local_fallback）"))
	}

	proxies := LocalParseToClash(rawText)
	if len(proxies) == 0 {
		return nil, fmt.Errorf("%s", i18n.T("本地解析未得到任何节点"))
	}
	return minimalClash(proxies), nil
}

// minimalClash 用解析出的 proxies 拼一份最小可用 Clash 配置（一个总 select 组 + 兜底规则）。
func minimalClash(proxies []map[string]any) map[string]any {
	names := make([]any, 0, len(proxies))
	list := make([]any, 0, len(proxies))
	for _, p := range proxies {
		list = append(list, p)
		if n := anyToStr(p["name"]); n != "" {
			names = append(names, n)
		}
	}
	return map[string]any{
		"proxies": list,
		"proxy-groups": []any{
			map[string]any{"name": "Proxy", "type": "select", "proxies": append(names, "DIRECT")},
		},
		"rules": []any{"GEOIP,CN,DIRECT", "MATCH,Proxy"},
	}
}

// toClashViaSubconverter base64 内容 → subconverter(target=clash) → Clash YAML 文本。
func toClashViaSubconverter(rawText, backend, proxy string) (string, error) {
	backend = strings.TrimRight(backend, "/")
	payload := url.QueryEscape(strings.TrimSpace(rawText))
	u := fmt.Sprintf("%s/sub?target=clash&list=false&url=%s", backend, payload)
	data, err := Fetch(u, "clash", proxy)
	if err != nil {
		return "", err
	}
	text := string(data)
	if !strings.Contains(text, "proxies:") {
		return "", fmt.Errorf("%s", i18n.T("subconverter 返回内容不含 proxies，可能后端不可用或订阅无效"))
	}
	return text, nil
}

// --------------------------------------------------------------------------
// 应急本地解析（best-effort）
// --------------------------------------------------------------------------

// LocalParseToClash 把 base64 订阅解析为 Clash 风格 proxy 字典列表。
func LocalParseToClash(rawText string) []map[string]any {
	decoded := rawText
	if b, err := b64decode(rawText); err == nil {
		decoded = string(b)
	}
	var proxies []map[string]any
	for _, line := range strings.Split(decoded, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.Contains(line, "://") {
			continue
		}
		if p := parseLink(line); p != nil {
			proxies = append(proxies, p)
		}
	}
	return proxies
}

func nameFromFragment(u *url.URL) string {
	if u.Fragment != "" {
		return u.Fragment
	}
	return ""
}

func parseLink(link string) map[string]any {
	scheme := strings.ToLower(strings.SplitN(link, "://", 2)[0])
	switch scheme {
	case "vmess":
		return parseVmess(link)
	case "ss":
		return parseSS(link)
	case "trojan":
		return parseUserinfo(link, "trojan")
	case "vless":
		return parseVless(link)
	case "hysteria2", "hy2":
		return parseUserinfo(link, "hysteria2")
	case "tuic":
		return parseTuic(link)
	}
	return nil
}

func parseVmess(link string) map[string]any {
	raw := strings.TrimPrefix(link, "vmess://")
	data, err := b64decode(raw)
	if err != nil {
		return nil
	}
	var info map[string]any
	if err := json.Unmarshal(data, &info); err != nil {
		return nil
	}
	name := anyToStr(info["ps"])
	if name == "" {
		name = anyToStr(info["add"])
	}
	if name == "" {
		name = "vmess"
	}
	p := map[string]any{
		"name": name, "type": "vmess",
		"server": info["add"], "port": info["port"],
		"uuid": info["id"], "alterId": orDefault(info["aid"], 0),
		"cipher": strOr(info["scy"], "auto"),
	}
	net := strings.ToLower(anyToStr(info["net"]))
	switch net {
	case "ws", "websocket":
		host := anyToStr(info["host"])
		if host == "" {
			host = anyToStr(info["add"])
		}
		p["network"] = "ws"
		p["ws-opts"] = map[string]any{"path": strOr(info["path"], "/"), "headers": map[string]any{"Host": host}}
	case "grpc":
		p["network"] = "grpc"
		p["grpc-opts"] = map[string]any{"grpc-service-name": anyToStr(info["path"])}
	}
	tls := strings.ToLower(anyToStr(info["tls"]))
	if tls == "tls" || tls == "true" || tls == "1" {
		p["tls"] = true
		sni := anyToStr(info["sni"])
		if sni == "" {
			sni = anyToStr(info["host"])
		}
		if sni != "" {
			p["servername"] = sni
		}
	}
	return p
}

func orDefault(v any, def any) any {
	if v == nil {
		return def
	}
	return v
}

func parseSS(link string) map[string]any {
	body := strings.TrimPrefix(link, "ss://")
	name := ""
	if u, err := url.Parse(link); err == nil {
		name = nameFromFragment(u)
	}
	body = strings.SplitN(body, "#", 2)[0]
	var creds, server string
	if i := strings.LastIndex(body, "@"); i >= 0 {
		creds, server = body[:i], body[i+1:]
		if b, err := b64decode(creds); err == nil {
			creds = string(b)
		}
	} else {
		b, err := b64decode(body)
		if err != nil {
			return nil
		}
		decoded := string(b)
		i := strings.LastIndex(decoded, "@")
		if i < 0 {
			return nil
		}
		creds, server = decoded[:i], decoded[i+1:]
	}
	cparts := strings.SplitN(creds, ":", 2)
	if len(cparts) != 2 {
		return nil
	}
	hi := strings.LastIndex(server, ":")
	if hi < 0 {
		return nil
	}
	host := server[:hi]
	portStr := server[hi+1:]
	portStr = strings.SplitN(portStr, "/", 2)[0]
	portStr = strings.SplitN(portStr, "?", 2)[0]
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil
	}
	if name == "" {
		name = host
	}
	return map[string]any{
		"name": name, "type": "ss", "server": host, "port": port,
		"cipher": cparts[0], "password": cparts[1],
	}
}

func parseUserinfo(link, ptype string) map[string]any {
	u, err := url.Parse(link)
	if err != nil {
		return nil
	}
	q := u.Query()
	name := nameFromFragment(u)
	if name == "" {
		name = u.Hostname()
	}
	password := ""
	if u.User != nil {
		password = u.User.Username()
	}
	p := map[string]any{
		"name": name, "type": ptype,
		"server": u.Hostname(), "port": portOf(u), "password": password,
	}
	sni := q.Get("sni")
	if sni == "" {
		sni = q.Get("peer")
	}
	if sni != "" {
		p["servername"] = sni
		p["tls"] = true
	}
	if v := q.Get("insecure"); v == "1" || v == "true" {
		p["skip-cert-verify"] = true
	}
	return p
}

func parseVless(link string) map[string]any {
	u, err := url.Parse(link)
	if err != nil {
		return nil
	}
	q := u.Query()
	name := nameFromFragment(u)
	if name == "" {
		name = u.Hostname()
	}
	uuid := ""
	if u.User != nil {
		uuid = u.User.Username()
	}
	p := map[string]any{
		"name": name, "type": "vless",
		"server": u.Hostname(), "port": portOf(u), "uuid": uuid,
	}
	if flow := q.Get("flow"); flow != "" {
		p["flow"] = flow
	}
	if sec := q.Get("security"); sec == "tls" || sec == "reality" {
		p["tls"] = true
		if sni := q.Get("sni"); sni != "" {
			p["servername"] = sni
		}
	}
	switch q.Get("type") {
	case "ws":
		host := q.Get("host")
		if host == "" {
			host = u.Hostname()
		}
		path := q.Get("path")
		if path == "" {
			path = "/"
		}
		p["network"] = "ws"
		p["ws-opts"] = map[string]any{"path": path, "headers": map[string]any{"Host": host}}
	case "grpc":
		p["network"] = "grpc"
		p["grpc-opts"] = map[string]any{"grpc-service-name": q.Get("serviceName")}
	}
	return p
}

func parseTuic(link string) map[string]any {
	u, err := url.Parse(link)
	if err != nil {
		return nil
	}
	q := u.Query()
	name := nameFromFragment(u)
	if name == "" {
		name = u.Hostname()
	}
	uuid, password := "", ""
	if u.User != nil {
		uuid = u.User.Username()
		password, _ = u.User.Password()
	}
	p := map[string]any{
		"name": name, "type": "tuic",
		"server": u.Hostname(), "port": portOf(u), "uuid": uuid, "password": password,
	}
	if cc := q.Get("congestion_control"); cc != "" {
		p["congestion-controller"] = cc
	}
	if sni := q.Get("sni"); sni != "" {
		p["servername"] = sni
		p["tls"] = true
	}
	return p
}

func portOf(u *url.URL) int {
	n, _ := strconv.Atoi(u.Port())
	return n
}
