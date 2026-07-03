package system

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Trilives/sboxkit/internal/paths"
)

const ServiceName = "sboxkit"

type Service struct {
	paths  paths.Paths
	runner Runner
}

func NewService(p paths.Paths, runner Runner) *Service {
	return &Service{paths: p, runner: runner}
}

func RenderServiceUnit(p paths.Paths) string {
	runtimeDir := p.EtcDir
	return fmt.Sprintf(`[Unit]
Description=sboxkit Service
Documentation=https://sing-box.sagernet.org/
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=root
WorkingDirectory=%s
ExecStart=%s run -c %s
Restart=on-failure
RestartSec=3
LimitNOFILE=1048576
AmbientCapabilities=CAP_NET_ADMIN CAP_NET_BIND_SERVICE CAP_NET_RAW
CapabilityBoundingSet=CAP_NET_ADMIN CAP_NET_BIND_SERVICE CAP_NET_RAW

[Install]
WantedBy=multi-user.target
`, runtimeDir, filepath.Join(runtimeDir, "sing-box"), filepath.Join(runtimeDir, "sboxkit.json"))
}

func (s *Service) Install(ctx context.Context, start bool) error {
	if err := s.preflight(); err != nil {
		return err
	}
	stagedConfig, err := s.stageRuntimeConfig()
	if err != nil {
		return err
	}
	defer os.Remove(stagedConfig)

	if err := s.runner.Run(ctx, "mkdir", "-p", s.paths.EtcDir); err != nil {
		return err
	}
	if err := s.runner.Run(ctx, "chmod", "0755", s.paths.EtcDir); err != nil {
		return err
	}
	core, err := s.singBoxSource()
	if err != nil {
		return err
	}
	if err := s.runner.Run(ctx, "install", "-m", "0755", core, filepath.Join(s.paths.EtcDir, "sing-box")); err != nil {
		return err
	}
	if err := s.syncRuntimeAssets(ctx); err != nil {
		return err
	}
	runtimeConfig := filepath.Join(s.paths.EtcDir, "sboxkit.json")
	if err := s.runner.Run(ctx, "install", "-m", "0644", stagedConfig, runtimeConfig); err != nil {
		return err
	}
	if err := s.runner.Run(ctx, filepath.Join(s.paths.EtcDir, "sing-box"), "check", "-c", runtimeConfig); err != nil {
		return err
	}

	unitTmp := filepath.Join(s.paths.StateDir, "sboxkit.service")
	if err := os.WriteFile(unitTmp, []byte(RenderServiceUnit(s.paths)), 0o644); err != nil {
		return err
	}
	defer os.Remove(unitTmp)
	if err := s.runner.Run(ctx, "install", "-m", "0644", unitTmp, "/etc/systemd/system/sboxkit.service"); err != nil {
		return err
	}
	if err := s.runner.Run(ctx, "systemctl", "daemon-reload"); err != nil {
		return err
	}
	if err := s.runner.Run(ctx, "systemctl", "enable", "sboxkit.service"); err != nil {
		return err
	}
	if start {
		return s.runner.Run(ctx, "systemctl", "restart", "sboxkit.service")
	}
	return nil
}

func (s *Service) SyncAndRestart(ctx context.Context) error {
	if err := s.preflight(); err != nil {
		return err
	}
	stagedConfig, err := s.stageRuntimeConfig()
	if err != nil {
		return err
	}
	defer os.Remove(stagedConfig)
	if err := s.syncRuntimeAssets(ctx); err != nil {
		return err
	}
	runtimeConfig := filepath.Join(s.paths.EtcDir, "sboxkit.json")
	if err := s.runner.Run(ctx, "install", "-m", "0644", stagedConfig, runtimeConfig); err != nil {
		return err
	}
	if err := s.runner.Run(ctx, filepath.Join(s.paths.EtcDir, "sing-box"), "check", "-c", runtimeConfig); err != nil {
		return err
	}
	return s.runner.Run(ctx, "systemctl", "restart", "sboxkit.service")
}

func (s *Service) Remove(ctx context.Context, purgeRuntime bool) error {
	_ = s.runner.Run(ctx, "systemctl", "stop", "sboxkit.service")
	_ = s.runner.Run(ctx, "systemctl", "disable", "sboxkit.service")
	if err := s.runner.Run(ctx, "rm", "-f", "/etc/systemd/system/sboxkit.service"); err != nil {
		return err
	}
	_ = s.runner.Run(ctx, "systemctl", "daemon-reload")
	_ = s.runner.Run(ctx, "systemctl", "reset-failed", "sboxkit.service")
	if purgeRuntime {
		if err := s.runner.Run(ctx, "rm", "-rf", s.paths.EtcDir); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) Status(ctx context.Context) error {
	return s.runner.Run(ctx, "systemctl", "status", "--no-pager", "sboxkit.service")
}

func (s *Service) Start(ctx context.Context) error {
	return s.runner.Run(ctx, "systemctl", "restart", "sboxkit.service")
}

func (s *Service) Stop(ctx context.Context) error {
	return s.runner.Run(ctx, "systemctl", "stop", "sboxkit.service")
}

func (s *Service) syncRuntimeAssets(ctx context.Context) error {
	if err := s.runner.Run(ctx, "mkdir", "-p", filepath.Join(s.paths.EtcDir, "ruleset")); err != nil {
		return err
	}
	for _, rule := range []string{s.paths.GeositeCN, s.paths.GeoIPCN} {
		if _, err := os.Stat(rule); err == nil {
			if err := s.runner.Run(ctx, "install", "-m", "0644", rule, filepath.Join(s.paths.EtcDir, "ruleset", filepath.Base(rule))); err != nil {
				return err
			}
		}
	}
	if _, err := os.Stat(s.paths.UIDir); err == nil {
		if err := s.runner.Run(ctx, "rm", "-rf", filepath.Join(s.paths.EtcDir, "ui")); err != nil {
			return err
		}
		if err := s.runner.Run(ctx, "cp", "-a", s.paths.UIDir, filepath.Join(s.paths.EtcDir, "ui")); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) preflight() error {
	if _, err := os.Stat(s.paths.ConfigFile); err != nil {
		return fmt.Errorf("required file missing %s: %w", s.paths.ConfigFile, err)
	}
	_, err := s.singBoxSource()
	return err
}

func (s *Service) singBoxSource() (string, error) {
	if _, err := os.Stat(s.paths.SingBoxBin); err == nil {
		return s.paths.SingBoxBin, nil
	}
	if _, err := os.Stat(s.paths.SystemSingBoxBin); err == nil {
		return s.paths.SystemSingBoxBin, nil
	}
	return "", fmt.Errorf("required sing-box core missing; expected %s or %s", s.paths.SingBoxBin, s.paths.SystemSingBoxBin)
}

func (s *Service) stageRuntimeConfig() (string, error) {
	data, err := os.ReadFile(s.paths.ConfigFile)
	if err != nil {
		return "", err
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return "", err
	}
	rewriteRuntimePaths(doc, s.paths)

	if err := os.MkdirAll(s.paths.StateDir, 0o755); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(s.paths.StateDir, ".runtime-config-*.json")
	if err != nil {
		return "", err
	}
	defer tmp.Close()
	encoded, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		os.Remove(tmp.Name())
		return "", err
	}
	if _, err := tmp.Write(append(encoded, '\n')); err != nil {
		os.Remove(tmp.Name())
		return "", err
	}
	return tmp.Name(), nil
}

func rewriteRuntimePaths(doc map[string]any, p paths.Paths) {
	exp, _ := doc["experimental"].(map[string]any)
	if exp == nil {
		exp = map[string]any{}
		doc["experimental"] = exp
	}
	cache, _ := exp["cache_file"].(map[string]any)
	if cache == nil {
		cache = map[string]any{}
		exp["cache_file"] = cache
	}
	cache["enabled"] = true
	cache["path"] = filepath.Join(p.EtcDir, "sboxkit.cache.db")
	if clash, ok := exp["clash_api"].(map[string]any); ok {
		clash["external_ui"] = filepath.Join(p.EtcDir, "ui")
	}
	if route, ok := doc["route"].(map[string]any); ok {
		if ruleSets, ok := route["rule_set"].([]any); ok {
			for _, item := range ruleSets {
				rule, ok := item.(map[string]any)
				if !ok || rule["type"] != "local" {
					continue
				}
				if pathText, ok := rule["path"].(string); ok && pathText != "" {
					rule["path"] = filepath.Join(p.EtcDir, "ruleset", filepath.Base(pathText))
				}
			}
		}
	}
}
