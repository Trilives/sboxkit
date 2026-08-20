package clashapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCurrentReturnsSelectedNode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/proxies/Proxy%20Group" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"now":"sg-02"}`))
	}))
	defer srv.Close()

	c := &Client{Base: srv.URL, http: srv.Client()}
	got, err := c.Current("Proxy Group")
	if err != nil {
		t.Fatal(err)
	}
	if got != "sg-02" {
		t.Fatalf("current = %q, want sg-02", got)
	}
}

func TestCurrentRejectsEmptyOrFailedResponse(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusBadGateway} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"now":""}`))
		}))
		c := &Client{Base: srv.URL, http: srv.Client()}
		if _, err := c.Current("Proxy"); err == nil {
			t.Fatalf("status %d: expected error", status)
		}
		srv.Close()
	}
}
