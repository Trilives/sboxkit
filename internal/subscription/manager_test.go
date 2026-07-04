package subscription

import (
	"os"
	"path/filepath"
	"testing"

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
