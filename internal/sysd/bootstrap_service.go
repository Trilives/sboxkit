package sysd

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Trilives/sboxkit/internal/config"
	"github.com/Trilives/sboxkit/internal/execx"
	"github.com/Trilives/sboxkit/internal/i18n"
	"github.com/Trilives/sboxkit/internal/paths"
)

const (
	bootstrapDNSTunDevice = "singbox"
)

var bootstrapDNSListenAddress = net.JoinHostPort(
	config.DefaultBootstrapDNSRelayServer,
	strconv.Itoa(config.DefaultBootstrapDNSRelayPort),
)

type bootstrapDNSRuntimeConfig struct {
	Upstreams []config.BootstrapDNSUpstream `json:"upstreams"`
}

type bootstrapDNSArtifacts struct {
	Config string
	Unit   string
}

type rootFileWriter func(path, content, mode, reason string) error

func bootstrapServiceName(name string) string {
	if name == "" {
		name = DefaultName
	}
	return name + "-bootstrap-dns"
}

func bootstrapArtifacts(name string, rt runtimePaths) bootstrapDNSArtifacts {
	return bootstrapDNSArtifacts{
		Config: filepath.Join(rt.Dir, "bootstrap-dns.json"),
		Unit:   "/etc/systemd/system/" + bootstrapServiceName(name) + ".service",
	}
}

// stageBootstrapDNSConfig writes the normalized ordered failover list to a
// caller-owned temporary file. The runtime service receives only IP-literal
// upstreams, so starting the relay never depends on the proxy it bootstraps.
func stageBootstrapDNSConfig(p paths.Paths) (string, error) {
	cfg := config.NormalizeBootstrapDNS(config.Load(p))
	runtimeCfg := bootstrapDNSRuntimeConfig{
		Upstreams: append([]config.BootstrapDNSUpstream(nil), cfg.BootstrapDNSUpstreams...),
	}
	raw, err := json.MarshalIndent(runtimeCfg, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal bootstrap DNS runtime config: %w", err)
	}
	raw = append(raw, '\n')
	tmp, err := os.CreateTemp(p.State, ".bootstrap-dns.*.json")
	if err != nil {
		return "", fmt.Errorf("stage bootstrap DNS runtime config: %w", err)
	}
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return "", err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return "", err
	}
	return tmp.Name(), nil
}

func bootstrapServiceText(name string, rt runtimePaths, exe string) string {
	artifacts := bootstrapArtifacts(name, rt)
	quotedExe := strconv.Quote(strings.ReplaceAll(exe, "%", "%%"))
	return fmt.Sprintf(`[Unit]
Description=Bootstrap DNS relay for %s
Before=%s.service
PartOf=%s.service

[Service]
Type=simple
User=root
WorkingDirectory=%s
ExecStart=%s bootstrap-dns --config %s --listen %s --tun-dev %s
Restart=on-failure
RestartSec=3
NoNewPrivileges=true
CapabilityBoundingSet=CAP_NET_ADMIN CAP_NET_RAW
AmbientCapabilities=CAP_NET_ADMIN CAP_NET_RAW
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
ProtectKernelTunables=true
ProtectControlGroups=true
RestrictSUIDSGID=true
LockPersonality=true
MemoryDenyWriteExecute=true
RestrictAddressFamilies=AF_INET AF_INET6

[Install]
WantedBy=multi-user.target
`, name, name, name, rt.Dir, quotedExe, artifacts.Config, bootstrapDNSListenAddress, bootstrapDNSTunDevice)
}

// installBootstrapDNS writes both relay artifacts. The caller owns the single
// daemon-reload/restart transaction shared with the main unit.
func installBootstrapDNS(p paths.Paths, name string, rt runtimePaths) error {
	exe, err := StableExecutablePath()
	if err != nil {
		return fmt.Errorf(i18n.T("定位 sboxkit 可执行文件: %w"), err)
	}
	return installBootstrapDNSWith(p, name, rt, exe, execx.RunRoot, execx.WriteRoot)
}

func installBootstrapDNSWith(
	p paths.Paths,
	name string,
	rt runtimePaths,
	exe string,
	runRoot rootCommandRunner,
	writeRoot rootFileWriter,
) error {
	staged, err := stageBootstrapDNSConfig(p)
	if err != nil {
		return err
	}
	defer os.Remove(staged)

	artifacts := bootstrapArtifacts(name, rt)
	if _, err := runRoot([]string{"install", "-m", "0644", staged, artifacts.Config}, "", nil); err != nil {
		return err
	}
	return writeRoot(
		artifacts.Unit,
		bootstrapServiceText(name, rt, exe),
		"0644",
		i18n.T("写 bootstrap DNS 单元"),
	)
}

func removeBootstrapDNS(name string, rt runtimePaths, quiet bool) {
	removeBootstrapDNSWith(name, rt, quiet, execx.RunRoot)
}

func removeBootstrapDNSWith(name string, rt runtimePaths, quiet bool, runRoot rootCommandRunner) {
	unit := bootstrapServiceName(name) + ".service"
	opt := &execx.Opt{Capture: quiet}
	runRoot([]string{"systemctl", "stop", unit}, "", opt)
	runRoot([]string{"systemctl", "disable", unit}, "", opt)
	artifacts := bootstrapArtifacts(name, rt)
	runRoot([]string{"rm", "-f", artifacts.Unit, artifacts.Config}, "", nil)
}

func BootstrapDNSInstalled(name string) bool {
	_, err := os.Stat(bootstrapArtifacts(name, rtPaths(name)).Unit)
	return err == nil
}
