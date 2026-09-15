package sysd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

func TestDiagnosticCollectionUsesFixedCommandsConcurrentlyAndCopiesInputs(t *testing.T) {
	report := HealthReport{Failure: FailureBootstrapDNS, Results: []ProbeResult{{Layer: ProbeBootstrapDNS, Detail: "timeout"}}}
	targets := []string{"tcp://223.5.5.5:53"}
	var mutex sync.Mutex
	var commands []string
	runner := func(name string, args ...string) string {
		mutex.Lock()
		commands = append(commands, strings.Join(append([]string{name}, args...), " "))
		mutex.Unlock()
		time.Sleep(50 * time.Millisecond)
		return name + " output"
	}
	started := time.Now()
	snapshot := collectDiagnosticSnapshotWithRunner(report, "wlan0", "singbox", "sing-box", targets, runner)
	if elapsed := time.Since(started); elapsed >= 250*time.Millisecond {
		t.Fatalf("diagnostic collection took %v; fixed commands were not concurrent", elapsed)
	}
	if len(commands) != 8 {
		t.Fatalf("commands = %v, want 8 fixed collectors", commands)
	}
	for _, required := range []string{"ip_rule", "routes", "resolver", "service_lifecycle", "interface_lifecycle", "tun_lifecycle", "sockets", "conntrack"} {
		if _, ok := snapshot.Sections[required]; !ok {
			t.Errorf("missing section %q", required)
		}
	}
	targets[0] = "changed"
	report.Results[0].Detail = "changed"
	if snapshot.BootstrapTargets[0] != "tcp://223.5.5.5:53" || snapshot.ProbeResults[0].Detail != "timeout" {
		t.Fatal("snapshot retained caller-owned slices")
	}
}

func TestHealthEvidenceFilesAreReadableButNotWorldWritable(t *testing.T) {
	directory := t.TempDir()
	statePath := filepath.Join(directory, "state.json")
	if err := SaveWatchdogState(statePath, WatchdogState{InFailure: true}); err != nil {
		t.Fatal(err)
	}
	snapshotPath := filepath.Join(directory, "snapshot.json")
	if _, err := WriteDiagnosticSnapshotOnce(snapshotPath, DiagnosticSnapshot{Failure: FailureEndToEnd}, 4096); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{statePath, snapshotPath} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0o644 {
			t.Errorf("%s mode = %#o, want 0644", path, got)
		}
	}
}

func TestDiagnosticCommandIsBoundedAndReportsMissingPrograms(t *testing.T) {
	t.Parallel()
	if got := diagnosticCommand("printf", "ok"); got != "ok" {
		t.Fatalf("diagnosticCommand(printf) = %q", got)
	}
	if got := diagnosticCommand("sboxkit-command-that-does-not-exist"); !strings.HasPrefix(got, "unavailable:") {
		t.Fatalf("missing command result = %q", got)
	}
	if got := diagnosticCommand(""); got != "unavailable: empty command" {
		t.Fatalf("empty command result = %q", got)
	}
}

func TestResolverDiagnosticFallsBackToResolvConf(t *testing.T) {
	t.Parallel()
	got := resolverDiagnosticWithRunner(func(string, ...string) string { return "unavailable: injected" })
	if strings.TrimSpace(got) == "" {
		t.Fatal("resolver fallback is empty")
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
