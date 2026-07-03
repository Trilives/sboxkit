package webui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/Trilives/sboxkit/internal/paths"
)

func TestStatusEndpointReportsRuntimeLayout(t *testing.T) {
	p := paths.FromRoot(t.TempDir())
	server := NewServer(p)
	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["service"] != "sboxkit" {
		t.Fatalf("unexpected service %v", body["service"])
	}
	if body["runtime"] != "/etc/sboxkit" {
		t.Fatalf("unexpected runtime %v", body["runtime"])
	}
}

func TestIndexServedFromEmbeddedAssets(t *testing.T) {
	server := NewServer(paths.FromRoot(t.TempDir()))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if rec.Body.Len() == 0 {
		t.Fatal("expected embedded index body")
	}
}

func TestWriteAssetsWritesEmbeddedIndex(t *testing.T) {
	out := filepath.Join(t.TempDir(), "ui")
	if err := WriteAssets(out); err != nil {
		t.Fatalf("write assets: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(out, "index.html"))
	if err != nil {
		t.Fatalf("read index: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected non-empty index")
	}
}
