package config

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Trilives/sboxkit/internal/paths"
)

func testPaths(t *testing.T) paths.Paths {
	t.Helper()
	t.Setenv("SBOXKIT_HOME", t.TempDir())
	p := paths.Detect()
	if err := p.EnsureStateDirs(); err != nil {
		t.Fatalf("ensure state dirs: %v", err)
	}
	return p
}

func TestLoadMergesKnownFieldsWithDefaults(t *testing.T) {
	p := testPaths(t)
	data := []byte(`{"enable_tun":false,"unknown":"ignored","download_proxy":"http://127.0.0.1:7890","enable_file_log":true,"log_max_mb":12,"bootstrap_dns_type":"https","bootstrap_dns_path":"/custom-dns-query","bootstrap_dns_tls_server_name":"dns.example"}`)
	if err := os.WriteFile(p.CustomizeFile, data, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg := Load(p)
	if cfg.EnableTun {
		t.Fatal("expected enable_tun override to be false")
	}
	if cfg.DownloadProxy != "http://127.0.0.1:7890" {
		t.Fatalf("unexpected download proxy %q", cfg.DownloadProxy)
	}
	if len(cfg.AIDomainSuffixes) == 0 {
		t.Fatal("expected default AI domain suffixes")
	}
	if !cfg.EnableFileLog {
		t.Fatal("expected enable_file_log override to be true")
	}
	if cfg.LogMaxMB != 12 {
		t.Fatalf("unexpected log max MB %d", cfg.LogMaxMB)
	}
	if cfg.BootstrapDNSType != "https" {
		t.Fatalf("unexpected bootstrap DNS type %q", cfg.BootstrapDNSType)
	}
	if cfg.BootstrapDNSPath != "/custom-dns-query" {
		t.Fatalf("unexpected bootstrap DNS path %q", cfg.BootstrapDNSPath)
	}
	if cfg.BootstrapDNSTLSServerName != "dns.example" {
		t.Fatalf("unexpected bootstrap DNS TLS server name %q", cfg.BootstrapDNSTLSServerName)
	}
	if cfg.BootstrapDNSPort != 443 {
		t.Fatalf("https bootstrap without a stored port = %d, want safe default 443", cfg.BootstrapDNSPort)
	}
	if cfg.MixedPort != DefaultMixedPort {
		t.Fatalf("missing mixed_port should default to %d, got %d", DefaultMixedPort, cfg.MixedPort)
	}
}

func TestOldCustomizeDefaultsToSafeBootstrapAndEnabledResilience(t *testing.T) {
	p := testPaths(t)
	if err := os.WriteFile(p.CustomizeFile, []byte(`{"bootstrap_dns_server":"223.5.5.5","bootstrap_dns_port":53}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := Load(p)
	if cfg.BootstrapDNSType != "tcp" {
		t.Fatalf("old customize bootstrap type = %q, want safe tcp default", cfg.BootstrapDNSType)
	}
	if !cfg.EnableResilience {
		t.Fatal("old customize should opt into resilience repair by default")
	}
}

func TestResiliencePreferenceRoundTrips(t *testing.T) {
	p := testPaths(t)
	cfg := Defaults()
	cfg.EnableResilience = false
	if err := Save(p, cfg); err != nil {
		t.Fatal(err)
	}
	if Load(p).EnableResilience {
		t.Fatal("explicitly disabled resilience preference was not preserved")
	}
}

func TestMixedPortValidationAndInvalidFileFallback(t *testing.T) {
	cfg := Defaults()
	if err := SetField(&cfg, "mixed_port", "17890"); err != nil {
		t.Fatalf("set mixed port: %v", err)
	}
	if cfg.MixedPort != 17890 {
		t.Fatalf("mixed port = %d, want 17890", cfg.MixedPort)
	}
	for _, value := range []string{"0", "65536", "not-a-port"} {
		if err := SetField(&cfg, "mixed_port", value); err == nil {
			t.Fatalf("expected %q to be rejected", value)
		}
	}

	p := testPaths(t)
	if err := os.WriteFile(p.CustomizeFile, []byte(`{"mixed_port":-1}`), 0o600); err != nil {
		t.Fatalf("write invalid config: %v", err)
	}
	if got := Load(p).MixedPort; got != DefaultMixedPort {
		t.Fatalf("invalid stored port should default to %d, got %d", DefaultMixedPort, got)
	}
}

func TestSaveWritesJSON(t *testing.T) {
	p := testPaths(t)
	cfg := Defaults()
	cfg.GitHubToken = "secret"
	cfg.BootstrapDNSType = "https"
	cfg.BootstrapDNSPath = "/dns-query"
	cfg.BootstrapDNSTLSServerName = "cloudflare-dns.com"

	if err := Save(p, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	if _, err := os.Stat(p.CustomizeFile); err != nil {
		t.Fatalf("stat config: %v", err)
	}

	loaded := Load(p)
	if loaded.GitHubToken != "secret" {
		t.Fatal("expected saved GitHub token to round-trip")
	}
	if loaded.BootstrapDNSType != "https" || loaded.BootstrapDNSPath != "/dns-query" || loaded.BootstrapDNSTLSServerName != "cloudflare-dns.com" {
		t.Fatalf("bootstrap DNS fields did not round-trip: %#v", loaded)
	}
}

func TestFieldLabelsCoverInteractiveConfigEditor(t *testing.T) {
	cfg := Defaults()
	cfg.GitHubToken = "abcdef"

	for _, key := range FieldOrder {
		label := FieldLabel(cfg, key)
		if label == "" {
			t.Fatalf("empty label for %s", key)
		}
		if key == "github_token" && label == "GitHub Token：abcdef" {
			t.Fatal("secret field label exposed the raw token")
		}
	}
}

func TestSetFieldUpdatesLoggingFields(t *testing.T) {
	cfg := Defaults()
	if cfg.EnableFileLog {
		t.Fatal("file logging should default to disabled")
	}

	if err := SetField(&cfg, "enable_file_log", "true"); err != nil {
		t.Fatalf("set enable_file_log: %v", err)
	}
	if err := SetField(&cfg, "log_max_mb", "20"); err != nil {
		t.Fatalf("set log_max_mb: %v", err)
	}

	if !cfg.EnableFileLog {
		t.Fatal("enable_file_log was not set")
	}
	if cfg.LogMaxMB != 20 {
		t.Fatalf("log_max_mb = %d, want 20", cfg.LogMaxMB)
	}
}

func TestBootstrapDNSTypeValidation(t *testing.T) {
	cfg := Defaults()
	for _, value := range []string{"udp", "tcp", "https", "dhcp"} {
		if err := SetField(&cfg, "bootstrap_dns_type", value); err != nil {
			t.Fatalf("set bootstrap_dns_type=%q: %v", value, err)
		}
	}
	if err := SetField(&cfg, "bootstrap_dns_type", "bogus"); err == nil {
		t.Fatal("expected invalid bootstrap_dns_type to be rejected")
	}
}

func TestBootstrapDNSTypeAdjustsConventionalPort(t *testing.T) {
	cfg := Defaults()
	if err := SetField(&cfg, "bootstrap_dns_type", "https"); err != nil {
		t.Fatal(err)
	}
	if cfg.BootstrapDNSPort != 443 {
		t.Fatalf("https port = %d, want 443", cfg.BootstrapDNSPort)
	}
	if err := SetField(&cfg, "bootstrap_dns_type", "tcp"); err != nil {
		t.Fatal(err)
	}
	if cfg.BootstrapDNSPort != 53 {
		t.Fatalf("tcp port = %d, want 53", cfg.BootstrapDNSPort)
	}
}

func TestBootstrapDNSServerRequiresIPAddressAndInvalidStoredValueFallsBack(t *testing.T) {
	cfg := Defaults()
	if err := SetField(&cfg, "bootstrap_dns_server", "dns.example"); err == nil {
		t.Fatal("expected domain bootstrap server to be rejected")
	}

	p := testPaths(t)
	if err := os.WriteFile(p.CustomizeFile, []byte(`{"bootstrap_dns_type":"https","bootstrap_dns_server":"dns.example","bootstrap_dns_port":443}`), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded := Load(p)
	if loaded.BootstrapDNSServer != DefaultBootstrapDNSServer {
		t.Fatalf("invalid stored bootstrap server = %q, want %q", loaded.BootstrapDNSServer, DefaultBootstrapDNSServer)
	}
	if loaded.BootstrapDNSTLSServerName != DefaultBootstrapDNSTLSServerName {
		t.Fatalf("fallback TLS server name = %q, want %q", loaded.BootstrapDNSTLSServerName, DefaultBootstrapDNSTLSServerName)
	}
}

func TestNormalizeBootstrapDoHRepairsEmptyTLSName(t *testing.T) {
	cfg := Defaults()
	cfg.BootstrapDNSType = "https"
	cfg.BootstrapDNSTLSServerName = ""
	got := NormalizeBootstrapDNS(cfg)
	if got.BootstrapDNSTLSServerName != DefaultBootstrapDNSTLSServerName {
		t.Fatalf("empty DoH TLS name = %q, want %q", got.BootstrapDNSTLSServerName, DefaultBootstrapDNSTLSServerName)
	}
}

func TestNormalizeBootstrapDNSRepairsInvalidStoredValues(t *testing.T) {
	cfg := Defaults()
	cfg.BootstrapDNSType = "bogus"
	cfg.BootstrapDNSServer = "not-an-ip"
	cfg.BootstrapDNSPort = 0
	cfg.BootstrapDNSPath = "missing-slash"
	got := NormalizeBootstrapDNS(cfg)
	if got.BootstrapDNSType != "tcp" || got.BootstrapDNSServer != DefaultBootstrapDNSServer || got.BootstrapDNSPort != 53 || got.BootstrapDNSPath != DefaultBootstrapDNSPath {
		t.Fatalf("invalid bootstrap values were not normalized: %#v", got)
	}

	cfg = Defaults()
	cfg.BootstrapDNSServer = "dhcp"
	if got := NormalizeBootstrapDNS(cfg); got.BootstrapDNSType != "dhcp" {
		t.Fatalf("legacy dhcp server did not select DHCP type: %#v", got)
	}
}

func TestEnableResilienceIsRuntimeOnlyConfig(t *testing.T) {
	cfg := Defaults()
	if !Bool(cfg, "enable_resilience") {
		t.Fatal("resilience should default to enabled")
	}
	if err := SetField(&cfg, "enable_resilience", "false"); err != nil {
		t.Fatal(err)
	}
	if Bool(cfg, "enable_resilience") {
		t.Fatal("SetField did not disable resilience")
	}
	if slices.Contains(DeploymentFields, "enable_resilience") {
		t.Fatal("runtime resilience preference must not appear in DeploymentFields")
	}
}

func TestFakeIPFilterRoundTripAndEditorMetadata(t *testing.T) {
	p := testPaths(t)
	cfg := Defaults()
	SetStrList(&cfg, "fake_ip_filter", []string{"node.example", "*.lan"})
	if err := Save(p, cfg); err != nil {
		t.Fatalf("save: %v", err)
	}
	got := StrList(Load(p), "fake_ip_filter")
	if len(got) != 2 || got[0] != "node.example" || got[1] != "*.lan" {
		t.Fatalf("fake_ip_filter = %#v", got)
	}
	if label := FieldLabel(cfg, "fake_ip_filter"); label == "" {
		t.Fatal("fake_ip_filter editor label is empty")
	}
}

func TestMainGroupKeywordsDefaultRoundTripAndEditorMetadata(t *testing.T) {
	defaults := Defaults()
	if !slices.Equal(defaults.MainGroupKeywords, []string{"Proxy"}) {
		t.Fatalf("default main_group_keywords = %#v", defaults.MainGroupKeywords)
	}

	p := testPaths(t)
	updated := defaults
	updated.MainGroupKeywords = []string{"节点选择", "Proxy"}
	if err := Save(p, updated); err != nil {
		t.Fatalf("save: %v", err)
	}
	if got := StrList(Load(p), "main_group_keywords"); !slices.Equal(got, updated.MainGroupKeywords) {
		t.Fatalf("main_group_keywords = %#v", got)
	}
	if label := FieldLabel(updated, "main_group_keywords"); label == "" {
		t.Fatal("main_group_keywords editor label is empty")
	}
}

func TestEnsureExistsWritesDefaultsOnce(t *testing.T) {
	p := testPaths(t)
	if _, err := os.Stat(p.CustomizeFile); err == nil {
		t.Fatal("customize.json should not exist yet")
	}
	if _, err := EnsureExists(p); err != nil {
		t.Fatalf("ensure exists: %v", err)
	}
	if _, err := os.Stat(p.CustomizeFile); err != nil {
		t.Fatalf("expected customize.json to be created: %v", err)
	}
	_ = filepath.Base(p.CustomizeFile)
}
