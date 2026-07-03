package proxyenv

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".bashrc")
	if err := os.WriteFile(path, []byte("export KEEP=1\n"), 0o644); err != nil {
		t.Fatalf("seed bashrc: %v", err)
	}

	if err := Write(path); err != nil {
		t.Fatalf("write first: %v", err)
	}
	if err := Write(path); err != nil {
		t.Fatalf("write second: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read bashrc: %v", err)
	}
	text := string(data)
	if strings.Count(text, beginMarker) != 1 {
		t.Fatalf("expected one proxy block, got:\n%s", text)
	}
	if !strings.Contains(text, `export http_proxy="http://127.0.0.1:7890"`) {
		t.Fatalf("missing http proxy export:\n%s", text)
	}
	if !strings.Contains(text, "export KEEP=1") {
		t.Fatalf("existing content was not preserved:\n%s", text)
	}
}

func TestRemoveDeletesOnlyManagedBlock(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".bashrc")
	if err := Write(path); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := Remove(path); err != nil {
		t.Fatalf("remove: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read bashrc: %v", err)
	}
	if strings.Contains(string(data), beginMarker) {
		t.Fatalf("managed block was not removed:\n%s", string(data))
	}
}
