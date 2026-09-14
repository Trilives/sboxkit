package sysd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestWriteDiagnosticSnapshotOnceIsIdempotentAndRedacted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "health-diagnostic.json")
	snapshot := DiagnosticSnapshot{
		GeneratedAt:      time.Date(2026, time.September, 11, 16, 35, 0, 0, time.UTC),
		Failure:          FailureBootstrapDNS,
		DefaultInterface: "wlan0",
		BootstrapTargets: []string{"223.5.5.5:53", "1.1.1.1:853"},
		ProbeResults: []ProbeResult{
			{Layer: ProbePhysicalUplink, OK: true},
			{Layer: ProbeBootstrapDNS, OK: false, Detail: "dial tcp: connection refused"},
		},
		Sections: map[string]string{
			"ip_rule":             "10000: from all fwmark 0x2023 lookup 2022",
			"routes":              "default via 192.0.2.1 dev wlan0",
			"resolver":            "Current DNS Server: 223.5.5.5\nAuthorization: Bearer resolver-secret",
			"service_lifecycle":   "ActiveEnterTimestamp=Thu 2026-09-11 16:30:00",
			"interface_lifecycle": "GENERAL.CONNECTION=my-home-network",
			"sockets":             "https://alice:hunter2@example.test/dns-query?token=query-secret",
			"conntrack":           "password=conntrack-secret api_key=key-secret",
		},
	}

	created, err := WriteDiagnosticSnapshotOnce(path, snapshot, 32*1024)
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("first snapshot write reported that no file was created")
	}
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"wlan0",
		"223.5.5.5:53",
		"ip_rule",
		"routes",
		"resolver",
		"service_lifecycle",
		"interface_lifecycle",
		"sockets",
		"conntrack",
		string(ProbeBootstrapDNS),
	} {
		if !bytes.Contains(first, []byte(required)) {
			t.Errorf("snapshot does not contain required diagnostic field %q: %s", required, first)
		}
	}
	for _, secret := range []string{
		"resolver-secret",
		"hunter2",
		"query-secret",
		"conntrack-secret",
		"key-secret",
	} {
		if bytes.Contains(first, []byte(secret)) {
			t.Errorf("snapshot leaked secret %q: %s", secret, first)
		}
	}
	if !bytes.Contains(first, []byte("[REDACTED]")) {
		t.Fatalf("snapshot did not visibly redact sensitive values: %s", first)
	}

	replacement := snapshot
	replacement.Failure = FailureEndToEnd
	replacement.Sections = map[string]string{"replacement": "must not overwrite first evidence"}
	created, err = WriteDiagnosticSnapshotOnce(path, replacement, 32*1024)
	if err != nil {
		t.Fatal(err)
	}
	if created {
		t.Fatal("second snapshot write overwrote an existing first-failure snapshot")
	}
	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(second, first) {
		t.Fatal("idempotent snapshot write changed the original evidence")
	}
}

func TestDiagnosticSnapshotHonorsByteLimitAndRemainsValidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bounded-diagnostic.json")
	const maxBytes = 1024
	snapshot := DiagnosticSnapshot{
		GeneratedAt:      time.Date(2026, time.September, 11, 16, 35, 0, 0, time.UTC),
		Failure:          FailureEndToEnd,
		DefaultInterface: "wlan0",
		BootstrapTargets: []string{"223.5.5.5:53"},
		ProbeResults: []ProbeResult{
			{Layer: ProbePhysicalUplink, OK: true},
			{Layer: ProbeBootstrapDNS, OK: true},
			{Layer: ProbeNodeResolution, OK: true},
			{Layer: ProbeMixedPort, OK: true},
			{Layer: ProbeEndToEnd, OK: false, Detail: strings.Repeat("代理失败日志🙂", 500)},
		},
		Sections: map[string]string{
			"routes":    strings.Repeat("default via 192.0.2.1 dev wlan0\n", 500),
			"sockets":   strings.Repeat("tcp ESTAB 127.0.0.1:2080 192.0.2.10:443\n", 500),
			"conntrack": strings.Repeat("tcp 6 431999 ESTABLISHED\n", 500),
		},
	}

	created, err := WriteDiagnosticSnapshotOnce(path, snapshot, maxBytes)
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("bounded snapshot was not created")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) > maxBytes {
		t.Fatalf("snapshot size = %d bytes, want <= %d", len(raw), maxBytes)
	}
	if !utf8.Valid(raw) {
		t.Fatal("byte limiting split a UTF-8 sequence")
	}
	if !json.Valid(raw) {
		t.Fatalf("bounded snapshot is not valid JSON: %q", raw)
	}
	if !bytes.Contains(raw, []byte(`"truncated":true`)) {
		t.Fatalf("bounded snapshot does not disclose truncation: %s", raw)
	}
}
