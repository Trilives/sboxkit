package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMergesKnownFieldsWithDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "customize.json")
	if err := os.WriteFile(path, []byte(`{"enable_tun":false,"unknown":"ignored","download_proxy":"http://127.0.0.1:7890","enable_file_log":true,"log_max_mb":12}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

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

func TestSaveWritesJSONWithPrivateMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "customize.json")
	cfg := Defaults()
	cfg.GitHubToken = "secret"

	if err := Save(path, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat config: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("expected mode 0600, got %o", got)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("load saved config: %v", err)
	}
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
