package subscription

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Trilives/sboxkit/internal/config"
)

func TestLocalParseVmessWS(t *testing.T) {
	info := map[string]any{
		"ps": "HK-vmess", "add": "1.2.3.4", "port": 443, "id": "uuid-1", "aid": 0,
		"scy": "auto", "net": "ws", "path": "/ws", "host": "cdn.example.com", "tls": "tls",
	}
	raw, _ := json.Marshal(info)
	link := "vmess://" + base64.StdEncoding.EncodeToString(raw)
	sub := base64.StdEncoding.EncodeToString([]byte(link + "\n"))

	proxies := LocalParseToClash(sub)
	if len(proxies) != 1 {
		t.Fatalf("解析出 %d 个节点, 期望 1", len(proxies))
	}
	p := proxies[0]
	if p["name"] != "HK-vmess" || p["type"] != "vmess" || p["server"] != "1.2.3.4" {
		t.Errorf("vmess 基本字段不符: %v", p)
	}
	if p["network"] != "ws" {
		t.Error("应识别 ws 传输")
	}
	ws := p["ws-opts"].(map[string]any)
	if ws["path"] != "/ws" {
		t.Error("ws path 不符")
	}
	if p["tls"] != true || p["servername"] != "cdn.example.com" {
		t.Error("tls/servername 不符")
	}
}

func TestLocalParseSSWithUserinfo(t *testing.T) {
	creds := base64.StdEncoding.EncodeToString([]byte("aes-256-gcm:passw0rd"))
	link := "ss://" + creds + "@5.6.7.8:8388#SG-ss"
	proxies := LocalParseToClash(link) // 非 base64 整体，逐行含 :// 即解析
	if len(proxies) != 1 {
		t.Fatalf("解析出 %d 个节点, 期望 1", len(proxies))
	}
	p := proxies[0]
	if p["name"] != "SG-ss" || p["cipher"] != "aes-256-gcm" || p["password"] != "passw0rd" || p["port"] != 8388 {
		t.Errorf("ss 字段不符: %v", p)
	}
}

func TestLocalParseTrojanAndHy2(t *testing.T) {
	links := strings.Join([]string{
		"trojan://pw@t.example.com:443?sni=t.example.com#TJ",
		"hysteria2://pw2@h.example.com:8443?sni=h.example.com&insecure=1#HY",
	}, "\n")
	proxies := LocalParseToClash(links)
	if len(proxies) != 2 {
		t.Fatalf("解析出 %d 个节点, 期望 2", len(proxies))
	}
	tj, hy := proxies[0], proxies[1]
	if tj["type"] != "trojan" || tj["password"] != "pw" || tj["tls"] != true {
		t.Errorf("trojan 字段不符: %v", tj)
	}
	if hy["type"] != "hysteria2" || hy["skip-cert-verify"] != true {
		t.Errorf("hysteria2 字段不符: %v", hy)
	}
}

func TestLocalParseVless(t *testing.T) {
	link := "vless://uuid-2@v.example.com:443?security=reality&sni=v.example.com&type=grpc&serviceName=svc&flow=xtls-rprx-vision#VL"
	proxies := LocalParseToClash(link)
	if len(proxies) != 1 {
		t.Fatal("vless 应解析成功")
	}
	p := proxies[0]
	if p["uuid"] != "uuid-2" || p["flow"] != "xtls-rprx-vision" || p["tls"] != true {
		t.Errorf("vless 字段不符: %v", p)
	}
	grpc := p["grpc-opts"].(map[string]any)
	if grpc["grpc-service-name"] != "svc" {
		t.Error("grpc serviceName 不符")
	}
}

func TestMinimalClashShape(t *testing.T) {
	m := minimalClash([]map[string]any{{"name": "n1", "type": "ss"}})
	groups := m["proxy-groups"].([]any)
	g := groups[0].(map[string]any)
	members := g["proxies"].([]any)
	if members[len(members)-1] != "DIRECT" {
		t.Error("总选择组应以 DIRECT 兜底")
	}
	rules := m["rules"].([]any)
	if rules[len(rules)-1] != "MATCH,Proxy" {
		t.Error("兜底规则应为 MATCH,Proxy")
	}
}

func TestToClashDictRequiresBackendOrFallback(t *testing.T) {
	_, err := ToClashDict("dm1lc3M6Ly9ub3BlCg==", config.Config{
		SubconverterBackend: "", Base64LocalFallback: false,
	})
	if err == nil {
		t.Fatal("无后端且未开应急解析应报错")
	}
}
