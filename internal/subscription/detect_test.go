package subscription

import (
	"encoding/base64"
	"testing"
)

func TestDetectClash(t *testing.T) {
	yamlText := `
proxies:
  - {name: hk-01, type: ss}
proxy-groups:
  - {name: Proxies, type: select, proxies: [hk-01]}
`
	if got := Detect([]byte(yamlText)); got != "clash" {
		t.Fatalf("Detect = %s, 期望 clash", got)
	}
}

func TestDetectSingBox(t *testing.T) {
	jsonText := `{"outbounds":[{"type":"vmess","tag":"node-1"}]}`
	if got := Detect([]byte(jsonText)); got != "sing-box" {
		t.Fatalf("Detect = %s, 期望 sing-box", got)
	}
}

func TestDetectBase64(t *testing.T) {
	links := "vmess://eyJhZGQiOiIxLjIuMy40In0=\ntrojan://pw@example.com:443"
	encoded := base64.StdEncoding.EncodeToString([]byte(links))
	if got := Detect([]byte(encoded)); got != "base64" {
		t.Fatalf("Detect = %s, 期望 base64", got)
	}
}

func TestDetectUnknown(t *testing.T) {
	cases := [][]byte{
		nil,
		[]byte("   "),
		[]byte("<html>not a subscription</html>"),
	}
	for _, c := range cases {
		if got := Detect(c); got != "unknown" {
			t.Errorf("Detect(%q) = %s, 期望 unknown", c, got)
		}
	}
}

func TestWarnIfMismatch(t *testing.T) {
	clashText := []byte("proxies:\n  - {name: a, type: ss}\n")
	if msg := WarnIfMismatch("clash", clashText); msg != "" {
		t.Errorf("类型相符不应告警: %s", msg)
	}
	if msg := WarnIfMismatch("base64", clashText); msg == "" {
		t.Error("类型不符应返回提示")
	}
	if msg := WarnIfMismatch("clash", []byte("???")); msg != "" {
		t.Error("无法判断时不应告警")
	}
}
