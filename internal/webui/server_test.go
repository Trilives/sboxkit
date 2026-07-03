package webui

import (
	"bytes"
	"encoding/json"
	"io"
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

func TestProxyEndpointsForwardToSingBoxClashAPI(t *testing.T) {
	var switched bytes.Buffer
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/proxies":
			return jsonResponse(http.StatusOK, `{"proxies":{"Proxy":{"name":"Proxy","type":"selector","now":"A","all":["A","B"]}}}`), nil
		case r.Method == http.MethodPut && r.URL.Path == "/proxies/Proxy":
			_, _ = switched.ReadFrom(r.Body)
			return jsonResponse(http.StatusNoContent, ""), nil
		default:
			return jsonResponse(http.StatusNotFound, "not found"), nil
		}
	})

	server := NewServer(paths.FromRoot(t.TempDir()))
	server.clashAPIURL = "http://127.0.0.1:9090"
	server.clashHTTP = &http.Client{Transport: transport}

	getReq := httptest.NewRequest(http.MethodGet, "/api/proxies", nil)
	getRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GET /api/proxies status = %d", getRec.Code)
	}
	if !bytes.Contains(getRec.Body.Bytes(), []byte(`"Proxy"`)) {
		t.Fatalf("GET /api/proxies body missing group: %s", getRec.Body.String())
	}

	putReq := httptest.NewRequest(http.MethodPut, "/api/proxies/Proxy", bytes.NewBufferString(`{"name":"B"}`))
	putRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(putRec, putReq)
	if putRec.Code != http.StatusNoContent {
		t.Fatalf("PUT /api/proxies/Proxy status = %d", putRec.Code)
	}
	if switched.String() != `{"name":"B"}` {
		t.Fatalf("forwarded switch body = %q", switched.String())
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewBufferString(body)),
	}
}
