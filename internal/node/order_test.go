package node

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReorderSelectorConfigMovesSelectedNodeFirst(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{
	  "outbounds": [
	    {"type":"selector","tag":"Proxy","outbounds":["A","B","C"],"default":"A"},
	    {"type":"selector","tag":"Other","outbounds":["X","Y"],"default":"X"}
	  ]
	}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if err := ReorderSelectorConfig(path, "Proxy", "C"); err != nil {
		t.Fatalf("reorder: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, `"outbounds": [
        "C",
        "A",
        "B"
      ]`) {
		t.Fatalf("selected node was not moved first:\n%s", text)
	}
	if !strings.Contains(text, `"default": "C"`) {
		t.Fatalf("selector default should follow selected node:\n%s", text)
	}
}

func TestReorderSelectorConfigRejectsMissingNode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"outbounds":[{"type":"selector","tag":"Proxy","outbounds":["A"]}]}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if err := ReorderSelectorConfig(path, "Proxy", "missing"); err == nil {
		t.Fatal("expected missing node error")
	}
}
