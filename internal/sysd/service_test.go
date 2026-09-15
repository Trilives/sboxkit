package sysd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Trilives/sboxkit/internal/config"
	"github.com/Trilives/sboxkit/internal/execx"
	"github.com/Trilives/sboxkit/internal/paths"
)

func TestStageRuntimeConfigRewritesExternalUIAndCachePath(t *testing.T) {
	state := t.TempDir()
	p := paths.Paths{
		State:      state,
		ConfigFile: filepath.Join(state, "config.json"),
		UI:         state, // hasUIAssets 检查 p.UI/index.html
	}
	if err := os.WriteFile(filepath.Join(state, "index.html"), []byte("<html></html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	raw := `{"inbounds":[],"outbounds":[],"experimental":{"clash_api":{"external_controller":"127.0.0.1:9090","external_ui":"ui"}}}`
	if err := os.WriteFile(p.ConfigFile, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}

	staged, err := stageRuntimeConfig(p, runtimePaths{
		UI:      "/var/lib/sboxkit-runtime/ui",
		CacheDB: "/var/lib/sboxkit-runtime/cache.db",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(staged)

	out, err := os.ReadFile(staged)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"/var/lib/sboxkit-runtime/ui"`) {
		t.Fatalf("external_ui was not rewritten in staged config:\n%s", out)
	}
	if !strings.Contains(string(out), `"/var/lib/sboxkit-runtime/cache.db"`) {
		t.Fatalf("cache_file.path was not rewritten in staged config:\n%s", out)
	}
}

func TestRuntimeSecretsUsePrivateDeploymentModes(t *testing.T) {
	if runtimeDirectoryMode != "0700" || runtimeConfigMode != "0600" {
		t.Fatalf("runtime modes expose node credentials: directory=%s config=%s", runtimeDirectoryMode, runtimeConfigMode)
	}
}

func TestStageRuntimeConfigUsesEmbeddedUIWhenStateUIIsMissing(t *testing.T) {
	state := t.TempDir()
	p := paths.Paths{
		State:      state,
		ConfigFile: filepath.Join(state, "config.json"),
		UI:         filepath.Join(state, "ui"),
	}
	raw := `{"inbounds":[],"outbounds":[],"experimental":{"clash_api":{"external_controller":"127.0.0.1:9090","external_ui":"ui"}}}`
	if err := os.WriteFile(p.ConfigFile, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}

	staged, err := stageRuntimeConfig(p, runtimePaths{
		UI:      "/var/lib/sboxkit-runtime/ui",
		CacheDB: "/var/lib/sboxkit-runtime/cache.db",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(staged)

	out, err := os.ReadFile(staged)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"/var/lib/sboxkit-runtime/ui"`) {
		t.Fatalf("embedded UI path was not staged when state UI was missing:\n%s", out)
	}
}

func TestSyncUIRuntimeStagesEmbeddedAssetsWithoutTouchingStateUI(t *testing.T) {
	rt := runtimePaths{UI: filepath.Join(t.TempDir(), "runtime-ui")}
	var commands [][]string
	stagedHasIndex := false
	runRoot := func(cmd []string, _ string, _ *execx.Opt) (execx.Result, error) {
		commands = append(commands, append([]string(nil), cmd...))
		if len(cmd) == 4 && cmd[0] == "cp" {
			_, err := os.Stat(filepath.Join(cmd[2], "index.html"))
			stagedHasIndex = err == nil
		}
		return execx.Result{}, nil
	}

	if err := syncUIRuntimeWith(rt, runRoot); err != nil {
		t.Fatal(err)
	}
	if !stagedHasIndex {
		t.Fatal("embedded UI was not materialized before the privileged copy")
	}
	if len(commands) != 2 {
		t.Fatalf("root commands = %v, want remove and copy of runtime UI only", commands)
	}
	if got := strings.Join(commands[0], " "); got != "rm -rf "+rt.UI {
		t.Fatalf("first root command = %q", got)
	}
	if commands[1][0] != "cp" || commands[1][1] != "-a" || commands[1][3] != rt.UI {
		t.Fatalf("copy command = %v", commands[1])
	}
	for _, cmd := range commands {
		if strings.Contains(strings.Join(cmd, " "), "/var/lib/sboxkit/ui") {
			t.Fatalf("runtime refresh must not mutate shared state UI: %v", cmd)
		}
	}
}

func TestSyncUIRuntimeReturnsPrivilegedReplacementErrors(t *testing.T) {
	wantErr := errors.New("root operation failed")
	for _, failAt := range []int{1, 2} {
		t.Run(fmt.Sprintf("command_%d", failAt), func(t *testing.T) {
			calls := 0
			runRoot := func(_ []string, _ string, _ *execx.Opt) (execx.Result, error) {
				calls++
				if calls == failAt {
					return execx.Result{}, wantErr
				}
				return execx.Result{}, nil
			}
			err := syncUIRuntimeWith(runtimePaths{UI: filepath.Join(t.TempDir(), "runtime-ui")}, runRoot)
			if !errors.Is(err, wantErr) {
				t.Fatalf("error = %v, want %v", err, wantErr)
			}
			if calls != failAt {
				t.Fatalf("root calls = %d, want stop at %d", calls, failAt)
			}
		})
	}
}

func TestSyncUIRuntimeReturnsTemporaryDirectoryError(t *testing.T) {
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "missing"))
	err := syncUIRuntimeWith(runtimePaths{UI: "/runtime/ui"}, nil)
	if err == nil || !strings.Contains(err.Error(), "stage embedded UI") {
		t.Fatalf("error = %v, want temporary staging failure", err)
	}
}

func TestResumeCompanionUnitsExcludesResidualWatchdog(t *testing.T) {
	bootstrapUnit := bootstrapServiceName(DefaultName) + ".service"
	got := resumeCompanionUnits([]string{bootstrapUnit, WatchdogName + ".timer", TimerName + ".timer"})
	if len(got) != 2 || got[0] != bootstrapUnit || got[1] != TimerName+".timer" {
		t.Fatalf("resume companions = %v, want bootstrap relay and weekly update timer", got)
	}
}

func TestStageBootstrapDNSConfigPreservesNormalizedUpstreamOrder(t *testing.T) {
	p := paths.Paths{
		State:         t.TempDir(),
		CustomizeFile: filepath.Join(t.TempDir(), "customize.json"),
	}
	want := []config.BootstrapDNSUpstream{
		{Provider: "Cloudflare", Type: "https", Server: "1.1.1.1", Port: 443, Path: "/dns-query", TLSServerName: "cloudflare-dns.com"},
		{Provider: "AliDNS", Type: "tcp", Server: "223.5.5.5", Port: 53},
	}
	raw, err := json.Marshal(config.Config{BootstrapDNSUpstreams: want})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.CustomizeFile, raw, 0o600); err != nil {
		t.Fatal(err)
	}

	staged, err := stageBootstrapDNSConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(staged)

	gotRaw, err := os.ReadFile(staged)
	if err != nil {
		t.Fatal(err)
	}
	var got bootstrapDNSRuntimeConfig
	if err := json.Unmarshal(gotRaw, &got); err != nil {
		t.Fatalf("unmarshal staged bootstrap config: %v\n%s", err, gotRaw)
	}
	if len(got.Upstreams) != len(want) {
		t.Fatalf("staged upstreams = %#v, want %#v", got.Upstreams, want)
	}
	for i := range want {
		if got.Upstreams[i] != want[i] {
			t.Fatalf("staged upstream[%d] = %#v, want %#v", i, got.Upstreams[i], want[i])
		}
	}
}

func TestBootstrapDNSServiceRunsStableExecutableAndFollowsMainService(t *testing.T) {
	rt := runtimePaths{Dir: "/var/lib/sboxkit-runtime"}
	unit := bootstrapServiceText(DefaultName, rt, `/opt/sbox kit/%preview/sboxkit`)

	for _, want := range []string{
		"Before=sing-box.service",
		"PartOf=sing-box.service",
		`ExecStart="/opt/sbox kit/%%preview/sboxkit" bootstrap-dns --config /var/lib/sboxkit-runtime/bootstrap-dns.json --listen 127.0.0.1:6450 --tun-dev singbox`,
		"CapabilityBoundingSet=CAP_NET_ADMIN CAP_NET_RAW",
		"ProtectSystem=strict",
		"ProtectHome=true",
		"RestrictAddressFamilies=AF_INET AF_INET6",
	} {
		if !strings.Contains(unit, want) {
			t.Fatalf("bootstrap unit missing %q:\n%s", want, unit)
		}
	}
}

func TestMainServiceRequiresBootstrapDNSRelay(t *testing.T) {
	unit, err := renderUnit(DefaultName, rtPaths(DefaultName))
	if err != nil {
		t.Fatal(err)
	}
	bootstrapUnit := bootstrapServiceName(DefaultName) + ".service"
	for _, want := range []string{
		"Requires=" + bootstrapUnit,
		"After=network-online.target " + bootstrapUnit,
	} {
		if !strings.Contains(unit, want) {
			t.Fatalf("main unit missing %q:\n%s", want, unit)
		}
	}
}

func TestBootstrapDNSArtifactsUseRuntimeAndSystemdPaths(t *testing.T) {
	rt := rtPaths("custom")
	artifacts := bootstrapArtifacts("custom", rt)
	if artifacts.Config != filepath.Join(rt.Dir, "bootstrap-dns.json") {
		t.Fatalf("bootstrap config path = %q", artifacts.Config)
	}
	if artifacts.Unit != "/etc/systemd/system/custom-bootstrap-dns.service" {
		t.Fatalf("bootstrap unit path = %q", artifacts.Unit)
	}
}

func TestInstallBootstrapDNSWritesRuntimeConfigAndCompanionUnit(t *testing.T) {
	p := paths.Paths{State: t.TempDir()}
	rt := rtPaths(DefaultName)
	var installedConfig bootstrapDNSRuntimeConfig
	var installedDestination string
	runRoot := func(cmd []string, _ string, _ *execx.Opt) (execx.Result, error) {
		if len(cmd) != 5 || cmd[0] != "install" {
			t.Fatalf("unexpected root command: %v", cmd)
		}
		raw, err := os.ReadFile(cmd[3])
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, &installedConfig); err != nil {
			t.Fatalf("unmarshal installed relay config: %v", err)
		}
		installedDestination = cmd[4]
		return execx.Result{}, nil
	}
	var unitPath, unitText string
	writeRoot := func(path, content, mode, _ string) error {
		if mode != "0644" {
			t.Fatalf("unit mode = %q", mode)
		}
		unitPath, unitText = path, content
		return nil
	}

	if err := installBootstrapDNSWith(p, DefaultName, rt, "/usr/bin/sboxkit", runRoot, writeRoot); err != nil {
		t.Fatal(err)
	}
	artifacts := bootstrapArtifacts(DefaultName, rt)
	if installedDestination != artifacts.Config || len(installedConfig.Upstreams) < 2 {
		t.Fatalf("installed relay config destination=%q config=%#v", installedDestination, installedConfig)
	}
	if unitPath != artifacts.Unit || !strings.Contains(unitText, "PartOf=sing-box.service") {
		t.Fatalf("installed relay unit path=%q content=\n%s", unitPath, unitText)
	}
}

func TestRemoveBootstrapDNSStopsDisablesAndDeletesAllArtifacts(t *testing.T) {
	rt := rtPaths(DefaultName)
	var commands [][]string
	runRoot := func(cmd []string, _ string, _ *execx.Opt) (execx.Result, error) {
		commands = append(commands, append([]string(nil), cmd...))
		return execx.Result{}, nil
	}

	removeBootstrapDNSWith(DefaultName, rt, true, runRoot)

	unit := bootstrapServiceName(DefaultName) + ".service"
	artifacts := bootstrapArtifacts(DefaultName, rt)
	want := [][]string{
		{"systemctl", "stop", unit},
		{"systemctl", "disable", unit},
		{"rm", "-f", artifacts.Unit, artifacts.Config},
	}
	if fmt.Sprint(commands) != fmt.Sprint(want) {
		t.Fatalf("remove commands = %v, want %v", commands, want)
	}
}
