package webui

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/Trilives/sboxkit/internal/config"
	"github.com/Trilives/sboxkit/internal/node"
	"github.com/Trilives/sboxkit/internal/paths"
	"github.com/Trilives/sboxkit/internal/subscription"
	"github.com/Trilives/sboxkit/internal/uiassets"
)

type Server struct {
	paths       paths.Paths
	clashAPIURL string
	clashHTTP   *http.Client
}

func NewServer(p paths.Paths) *Server {
	return &Server{paths: p, clashAPIURL: "http://127.0.0.1:9090"}
}

func (s *Server) ListenAndServe(ctx context.Context, addr string) error {
	if addr == "" {
		addr = "127.0.0.1:8790"
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", addr, err)
	}
	server := &http.Server{Handler: s.Handler()}
	go func() {
		<-ctx.Done()
		_ = server.Shutdown(context.Background())
	}()
	if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/status", s.handleStatus)
	mux.HandleFunc("/api/proxies", s.handleProxies)
	mux.HandleFunc("/api/proxies/", s.handleProxySwitch)
	mux.Handle("/", http.FileServer(http.FS(uiassets.FS())))
	return mux
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	manager := subscription.NewManager(s.paths, config.Defaults())
	active, _ := manager.Active()
	response := map[string]any{
		"service": "sboxkit",
		"state":   s.paths.StateDir,
		"runtime": s.paths.RuntimeLink,
		"active":  nil,
	}
	if active != nil {
		response["active"] = map[string]any{
			"name":  active.Name,
			"nodes": active.LastNodeCount,
			"type":  active.SourceType,
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

func (s *Server) handleProxies(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	groups, err := s.clashClient().Groups(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"groups": groups})
}

func (s *Server) handleProxySwitch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	group, err := url.PathUnescape(strings.TrimPrefix(r.URL.Path, "/api/proxies/"))
	if err != nil || group == "" {
		http.Error(w, "invalid group", http.StatusBadRequest)
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if body.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	if err := s.clashClient().Switch(r.Context(), group, body.Name); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) clashClient() *node.Client {
	client := node.NewClient(s.clashAPIURL, "")
	if s.clashHTTP != nil {
		client.HTTP = s.clashHTTP
	}
	return client
}

func WriteAssets(outDir string) error {
	return uiassets.Write(outDir)
}
