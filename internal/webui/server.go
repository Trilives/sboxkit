package webui

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"

	"github.com/Trilives/sboxkit/internal/config"
	"github.com/Trilives/sboxkit/internal/paths"
	"github.com/Trilives/sboxkit/internal/subscription"
	"github.com/Trilives/sboxkit/internal/uiassets"
)

type Server struct {
	paths paths.Paths
}

func NewServer(p paths.Paths) *Server {
	return &Server{paths: p}
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
		"runtime": s.paths.EtcDir,
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

func WriteAssets(outDir string) error {
	return uiassets.Write(outDir)
}
