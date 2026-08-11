package subscription

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Trilives/sboxkit/internal/config"
	"github.com/Trilives/sboxkit/internal/paths"
)

func TestSlug(t *testing.T) {
	cases := map[string]string{
		"  Hua  ": "Hua",
		"a/b\\c":  "a-b-c",
		"x .. y":  "x---y",
		"":        "sub",
		". ":      "sub",
		"多 词  订阅": "多-词-订阅",
	}
	for in, want := range cases {
		if got := Slug(in); got != want {
			t.Errorf("Slug(%q) = %q, 期望 %q", in, got, want)
		}
	}
}

func TestMetaRoundtripPythonCompatible(t *testing.T) {
	t.Setenv("SBOXKIT_HOME", t.TempDir())
	p := paths.Detect()
	if err := p.EnsureStateDirs(); err != nil {
		t.Fatal(err)
	}
	// Python 版写出的 meta.json（字段名快照）
	pyMeta := `{
  "name": "Hua",
  "url": "https://example.com/sub",
  "source_type": "clash",
  "apply_overlay": false,
  "created_at": "2026-07-01T10:00:00+00:00",
  "updated_at": "2026-07-02T10:00:00+00:00",
  "last_node_count": 42
}`
	dir := p.SubscriptionDir("Hua")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "meta.json"), []byte(pyMeta), 0o644)

	sub := Get(p, "Hua")
	if sub == nil {
		t.Fatal("应能直读 Python 版 meta.json")
	}
	if sub.Name != "Hua" || sub.SourceType != "clash" || sub.LastNodeCount != 42 {
		t.Errorf("meta 字段解析不符: %+v", sub)
	}

	os.WriteFile(p.ActiveFile, []byte("Hua\n"), 0o644)
	if active := GetActive(p); active == nil || active.Name != "Hua" {
		t.Error("GetActive 应解析 active 指针")
	}

	subs := ListAll(p)
	if len(subs) != 1 || subs[0].Name != "Hua" {
		t.Errorf("ListAll = %+v", subs)
	}
}

func TestRebuildKeepsFakeIPFromStoredOriginal(t *testing.T) {
	t.Setenv("SBOXKIT_HOME", t.TempDir())
	p := paths.Detect()
	if err := p.EnsureStateDirs(); err != nil {
		t.Fatal(err)
	}
	sub := &Subscription{
		Name: "fake", URL: "/unused", SourceType: "local", ApplyOverlay: true,
		CreatedAt: now(), UpdatedAt: now(),
	}
	dir := p.SubscriptionDir(sub.Name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	meta, _ := json.Marshal(sub)
	if err := os.WriteFile(metaFile(p, sub.Name), meta, 0o644); err != nil {
		t.Fatal(err)
	}
	raw := `
dns:
  enhanced-mode: fake-ip
  fake-ip-filter: ["*.lan"]
proxies:
  - {name: ss, type: ss, server: 1.2.3.4, port: 443, cipher: aes-128-gcm, password: pw}
`
	if err := os.WriteFile(rawFile(p, sub), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Load(p)
	cfg.FakeIPFilter = []string{"*.lan", "+.user.example", "node.provider.example"}
	if err := config.Save(p, cfg); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err := Rebuild(p, sub.Name); err != nil {
			t.Fatalf("rebuild %d: %v", i+1, err)
		}
		data, err := os.ReadFile(configFile(p, sub.Name))
		if err != nil {
			t.Fatal(err)
		}
		text := string(data)
		if !strings.Contains(text, `"type": "fakeip"`) || !strings.Contains(text, `"lan"`) ||
			!strings.Contains(text, `"user.example"`) || !strings.Contains(text, `"node.provider.example"`) {
			t.Fatalf("rebuild %d lost fake-ip semantics:\n%s", i+1, data)
		}
		if strings.Count(text, `"lan"`) != 1 {
			t.Fatalf("rebuild %d duplicated merged fake-ip exclusions:\n%s", i+1, data)
		}
	}
}
