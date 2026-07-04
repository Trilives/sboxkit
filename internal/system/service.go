package system

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

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
	currentDir := p.RuntimeLink
	core := serviceCorePath(p)
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
`, currentDir, core, filepath.Join(currentDir, "config.json"))
}

func (s *Service) Install(ctx context.Context, start bool) error {
	if err := s.preflight(); err != nil {
		return err
	}
	activation, err := s.stageActivation(ctx)
	if err != nil {
		return err
	}

	if err := s.runner.Run(ctx, "mkdir", "-p", s.paths.EtcDir); err != nil {
		return err
	}
	if err := s.runner.Run(ctx, "chmod", "0755", s.paths.EtcDir); err != nil {
		return err
	}
	if err := s.activate(ctx, activation); err != nil {
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
	previous, _ := os.Readlink(s.paths.RuntimeLink)
	activation, err := s.stageActivation(ctx)
	if err != nil {
		return err
	}
	if err := s.activate(ctx, activation); err != nil {
		return err
	}
	if err := s.pruneActivations(ctx, activation, previous); err != nil {
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
		if err := s.runner.Run(ctx, "rm", "-rf", s.paths.Root, s.paths.EtcDir, filepath.Dir(s.paths.DownloadsDir)); err != nil {
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

func (s *Service) stageActivation(ctx context.Context) (string, error) {
	revision := time.Now().UTC().Format("20060102T150405.000000000Z")
	activation := filepath.Join(s.paths.ActivationsDir, revision)
	if err := s.runner.Run(ctx, "mkdir", "-p", activation); err != nil {
		return "", err
	}
	core, err := s.singBoxSource()
	if err != nil {
		return "", err
	}
	stagedConfig, err := s.stageRuntimeConfig(revision)
	if err != nil {
		return "", err
	}
	defer os.Remove(stagedConfig)
	runtimeConfig := filepath.Join(activation, "config.json")
	if err := s.runner.Run(ctx, "install", "-m", "0644", stagedConfig, runtimeConfig); err != nil {
		return "", err
	}
	manifest, err := s.writeActivationManifest(revision, core)
	if err != nil {
		return "", err
	}
	defer os.Remove(manifest)
	if err := s.runner.Run(ctx, "install", "-m", "0644", manifest, filepath.Join(activation, "manifest.json")); err != nil {
		return "", err
	}
	if _, err := os.Stat(filepath.Join(s.paths.StateDir, "healthcheck.sh")); err == nil {
		if err := s.runner.Run(ctx, "install", "-m", "0755", filepath.Join(s.paths.StateDir, "healthcheck.sh"), filepath.Join(activation, "healthcheck.sh")); err != nil {
			return "", err
		}
	}
	if err := s.runner.Run(ctx, core, "check", "-c", runtimeConfig); err != nil {
		return "", err
	}
	return activation, nil
}

func (s *Service) activate(ctx context.Context, activation string) error {
	tmpLink := s.paths.RuntimeLink + ".next"
	if err := s.runner.Run(ctx, "ln", "-sfn", activation, tmpLink); err != nil {
		return err
	}
	if err := s.runner.Run(ctx, "mv", "-Tf", tmpLink, s.paths.RuntimeLink); err != nil {
		return err
	}
	return nil
}

func (s *Service) pruneActivations(ctx context.Context, current string, previous string) error {
	entries, err := os.ReadDir(s.paths.ActivationsDir)
	if err != nil {
		return nil
	}
	keep := map[string]bool{
		filepath.Clean(current):  true,
		filepath.Clean(previous): previous != "",
	}
	var dirs []string
	for _, entry := range entries {
		if entry.IsDir() {
			dirs = append(dirs, filepath.Join(s.paths.ActivationsDir, entry.Name()))
		}
	}
	sort.Strings(dirs)
	for _, dir := range dirs {
		if keep[filepath.Clean(dir)] {
			continue
		}
		if err := s.runner.Run(ctx, "rm", "-rf", dir); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) uiSource() string {
	if _, err := os.Stat(filepath.Join(s.paths.UIDir, "index.html")); err == nil {
		return s.paths.UIDir
	}
	if _, err := os.Stat(filepath.Join(s.paths.SystemUIDir, "index.html")); err == nil {
		return s.paths.SystemUIDir
	}
	return ""
}

func (s *Service) preflight() error {
	if _, err := os.Stat(s.paths.ConfigFile); err != nil {
		return fmt.Errorf("required file missing %s: %w", s.paths.ConfigFile, err)
	}
	_, err := s.singBoxSource()
	return err
}

func (s *Service) singBoxSource() (string, error) {
	if _, err := os.Stat(s.paths.SystemSingBoxBin); err == nil {
		return s.paths.SystemSingBoxBin, nil
	}
	if _, err := os.Stat(s.paths.SingBoxBin); err == nil {
		return s.paths.SingBoxBin, nil
	}
	return "", fmt.Errorf("required sing-box core missing; expected %s or %s", s.paths.SingBoxBin, s.paths.SystemSingBoxBin)
}

func serviceCorePath(p paths.Paths) string {
	if _, err := os.Stat(p.SystemSingBoxBin); err == nil {
		return p.SystemSingBoxBin
	}
	if _, err := os.Stat(p.SingBoxBin); err == nil {
		return p.SingBoxBin
	}
	return p.SystemSingBoxBin
}

func (s *Service) stageRuntimeConfig(revision string) (string, error) {
	data, err := os.ReadFile(s.paths.ConfigFile)
	if err != nil {
		return "", err
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return "", err
	}
	rewriteRuntimePaths(doc, s.paths, s.uiSource())

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

func (s *Service) writeActivationManifest(revision string, core string) (string, error) {
	manifest := map[string]any{
		"revision":     revision,
		"created_at":   time.Now().UTC().Format(time.RFC3339),
		"sing_box":     core,
		"config":       s.paths.ConfigFile,
		"state_root":   s.paths.Root,
		"rulesets":     s.paths.RulesetDir,
		"web_ui":       s.uiSource(),
		"cache_db":     s.paths.SingBoxCacheDB,
		"admin_config": s.paths.AdminConfigFile,
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(s.paths.StateDir, 0o755); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(s.paths.StateDir, ".activation-manifest-*.json")
	if err != nil {
		return "", err
	}
	defer tmp.Close()
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		os.Remove(tmp.Name())
		return "", err
	}
	return tmp.Name(), nil
}

func rewriteRuntimePaths(doc map[string]any, p paths.Paths, uiPath string) {
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
	cache["path"] = p.SingBoxCacheDB
	if clash, ok := exp["clash_api"].(map[string]any); ok {
		if uiPath != "" {
			clash["external_ui"] = uiPath
		}
	}
}
