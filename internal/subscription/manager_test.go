package subscription

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Trilives/sboxkit/internal/config"
	"github.com/Trilives/sboxkit/internal/paths"
	"github.com/Trilives/sboxkit/internal/testkit"
)

func TestManagerAddAndSwitchSubscription(t *testing.T) {
	p := paths.FromRoot(t.TempDir())
	manager := NewManager(p, config.Defaults())
	manager.fetch = func(rawURL string, source SourceKind, proxy string) ([]byte, error) {
		return []byte(testkit.ReadFixture(t, "testdata/converter/clash-basic.yaml")), nil
	}

	sub, err := manager.Add("My Sub", "https://example.com/sub", SourceClash, true, true)
	if err != nil {
		t.Fatalf("add subscription: %v", err)
	}
	if sub.Name != "My-Sub" {
		t.Fatalf("unexpected slug %q", sub.Name)
	}
	if sub.LastNodeCount != 2 {
		t.Fatalf("expected 2 converted nodes, got %d", sub.LastNodeCount)
	}

	active, err := os.ReadFile(p.ActiveFile)
	if err != nil {
		t.Fatalf("read active: %v", err)
	}
	if string(active) != "My-Sub\n" {
		t.Fatalf("unexpected active content %q", active)
	}
	if _, err := os.Stat(filepath.Join(p.SubscriptionsDir, "My-Sub", "config.json")); err != nil {
		t.Fatalf("expected generated config: %v", err)
	}
}

func TestManagerAddFileCopiesSourceAndSwitches(t *testing.T) {
	p := paths.FromRoot(t.TempDir())
	manager := NewManager(p, config.Defaults())
	source := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(source, []byte(testkit.ReadFixture(t, "testdata/converter/clash-basic.yaml")), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}

	sub, err := manager.AddFile("local", source, "", true, true)
	if err != nil {
		t.Fatalf("add file: %v", err)
	}

	if sub.SourceType != SourceClash {
		t.Fatalf("expected detected clash source, got %s", sub.SourceType)
	}
	if sub.LastNodeCount != 2 {
		t.Fatalf("expected 2 converted nodes, got %d", sub.LastNodeCount)
	}
	for _, want := range []string{
		filepath.Join(p.SubscriptionsDir, "local", "source.yaml"),
		filepath.Join(p.SubscriptionsDir, "local", "raw.yaml"),
		p.ConfigFile,
	} {
		if _, err := os.Stat(want); err != nil {
			t.Fatalf("expected copied/generated file %s: %v", want, err)
		}
	}
}

func TestManagerWritesEmbeddedUIOnlyWhenLanPanelEnabled(t *testing.T) {
	p := paths.FromRoot(t.TempDir())
	cfg := config.Defaults()
	cfg.LanPanel = true
	manager := NewManager(p, cfg)
	manager.fetch = func(rawURL string, source SourceKind, proxy string) ([]byte, error) {
		return []byte(testkit.ReadFixture(t, "testdata/converter/clash-basic.yaml")), nil
	}

	if _, err := manager.Add("panel", "https://example.com/sub", SourceClash, true, true); err != nil {
		t.Fatalf("add subscription: %v", err)
	}
	if _, err := os.Stat(filepath.Join(p.UIDir, "index.html")); err != nil {
		t.Fatalf("expected embedded UI index: %v", err)
	}
}
