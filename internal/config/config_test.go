package config

import (
	"os"
	"path/filepath"
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
	data := []byte(`{"enable_tun":false,"unknown":"ignored","download_proxy":"http://127.0.0.1:7890","enable_file_log":true,"log_max_mb":12}`)
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
}

func TestSaveWritesJSON(t *testing.T) {
	p := testPaths(t)
	cfg := Defaults()
	cfg.GitHubToken = "secret"

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
