package converter

import "testing"

func convertOne(t *testing.T, proxy map[string]any) map[string]any {
	t.Helper()
	out, reason := convertProxy(proxy, map[string]bool{})
	if reason != "" {
		t.Fatalf("unexpected rejection: %s", reason)
	}
	return out
}

func rejectReason(t *testing.T, proxy map[string]any) string {
	t.Helper()
	out, reason := convertProxy(proxy, map[string]bool{})
	if reason == "" {
		t.Fatalf("expected rejection, got outbound %#v", out)
	}
	return reason
}

func TestVLessRealityAndClientFingerprint(t *testing.T) {
	out := convertOne(t, map[string]any{
		"name": "reality-01", "type": "vless", "server": "r.example.com", "port": 443,
		"uuid": "11111111-1111-1111-1111-111111111111", "flow": "xtls-rprx-vision",
		"tls": true, "client-fingerprint": "chrome",
		"reality-opts": map[string]any{"public-key": "PUB", "short-id": "ab12"},
	})
	tls, ok := out["tls"].(map[string]any)
	if !ok {
		t.Fatalf("missing tls block: %#v", out)
	}
	reality, ok := tls["reality"].(map[string]any)
	if !ok {
		t.Fatalf("missing reality block: %#v", tls)
	}
	if reality["public_key"] != "PUB" || reality["short_id"] != "ab12" {
		t.Fatalf("unexpected reality: %#v", reality)
	}
	utls, ok := tls["utls"].(map[string]any)
	if !ok || utls["fingerprint"] != "chrome" {
		t.Fatalf("expected utls fingerprint chrome, got %#v", tls["utls"])
	}
	if out["flow"] != "xtls-rprx-vision" {
		t.Fatalf("expected vision flow, got %#v", out["flow"])
	}
}

func TestCertificateFingerprintNotMappedToUTLS(t *testing.T) {
	// Clash `fingerprint` is a certificate pin, not a uTLS fingerprint — it must
	// not become tls.utls (converter.md §3).
	out := convertOne(t, map[string]any{
		"name": "trojan-01", "type": "trojan", "server": "t.example.com", "port": 443,
		"password": "pw", "tls": true, "sni": "t.example.com",
		"fingerprint": "aa:bb:cc:dd",
	})
	tls := out["tls"].(map[string]any)
	if _, ok := tls["utls"]; ok {
		t.Fatalf("certificate fingerprint leaked into utls: %#v", tls)
	}
}

func TestVLessRejectsUnsupportedEncryptionAndFlow(t *testing.T) {
	if r := rejectReason(t, map[string]any{
		"name": "enc", "type": "vless", "server": "s", "port": 443,
		"uuid": "u", "encryption": "xtls-rprx-direct",
	}); r == "" {
		t.Fatal("expected encryption rejection")
	}
	if r := rejectReason(t, map[string]any{
		"name": "flow", "type": "vless", "server": "s", "port": 443,
		"uuid": "u", "flow": "bogus-flow",
	}); r == "" {
		t.Fatal("expected flow rejection")
	}
}

func TestHysteria2ObfsBandwidthPorts(t *testing.T) {
	out := convertOne(t, map[string]any{
		"name": "hy2", "type": "hysteria2", "server": "h.example.com", "port": 443,
		"password": "pw", "up": "100 Mbps", "down": "1 Gbps",
		"ports": "443,8000-9000", "obfs": "salamander", "obfs-password": "opw",
	})
	if out["up_mbps"] != 100 || out["down_mbps"] != 1000 {
		t.Fatalf("bandwidth mis-parsed: up=%#v down=%#v", out["up_mbps"], out["down_mbps"])
	}
	obfs, ok := out["obfs"].(map[string]any)
	if !ok || obfs["type"] != "salamander" || obfs["password"] != "opw" {
		t.Fatalf("obfs not carried: %#v", out["obfs"])
	}
	ports, ok := out["server_ports"].([]string)
	if !ok || len(ports) != 2 || ports[0] != "443:443" || ports[1] != "8000:9000" {
		t.Fatalf("server_ports wrong: %#v", out["server_ports"])
	}
}

func TestTUICv5FieldsAndV4Rejected(t *testing.T) {
	out := convertOne(t, map[string]any{
		"name": "tuic5", "type": "tuic", "server": "q.example.com", "port": 443,
		"uuid": "u", "password": "pw", "congestion-controller": "bbr",
		"udp-relay-mode": "native", "reduce-rtt": true, "heartbeat-interval": 10000,
	})
	if out["congestion_control"] != "bbr" || out["udp_relay_mode"] != "native" {
		t.Fatalf("tuic v5 fields wrong: %#v", out)
	}
	if out["zero_rtt_handshake"] != true {
		t.Fatalf("expected zero_rtt_handshake true, got %#v", out["zero_rtt_handshake"])
	}
	if out["heartbeat"] != "10s" {
		t.Fatalf("expected heartbeat 10s, got %#v", out["heartbeat"])
	}

	if r := rejectReason(t, map[string]any{
		"name": "tuic4", "type": "tuic", "server": "q", "port": 443, "token": "tok",
	}); r == "" {
		t.Fatal("expected TUIC v4 rejection")
	}
}

func TestMultiplexOnlyWhenEnabled(t *testing.T) {
	base := map[string]any{
		"name": "mux", "type": "trojan", "server": "t", "port": 443,
		"password": "pw", "tls": true,
	}
	if out := convertOne(t, base); out["multiplex"] != nil {
		t.Fatalf("multiplex should be absent without smux: %#v", out["multiplex"])
	}
	base["smux"] = map[string]any{"enabled": true, "protocol": "smux", "max-streams": 8}
	out := convertOne(t, base)
	mux, ok := out["multiplex"].(map[string]any)
	if !ok || mux["enabled"] != true || mux["protocol"] != "smux" || mux["max_streams"] != 8 {
		t.Fatalf("multiplex not carried: %#v", out["multiplex"])
	}
}

func TestBandwidthMbps(t *testing.T) {
	cases := map[string]struct {
		in   any
		want int
	}{
		"plain":  {"50", 50},
		"mbps":   {"100 Mbps", 100},
		"gbps":   {"2 Gbps", 2000},
		"short":  {"30m", 30},
		"number": {200, 200},
		"empty":  {"", 0},
		"junk":   {"fast", 0},
	}
	for name, tc := range cases {
		if got := bandwidthMbps(tc.in); got != tc.want {
			t.Errorf("%s: bandwidthMbps(%v)=%d want %d", name, tc.in, got, tc.want)
		}
	}
}
