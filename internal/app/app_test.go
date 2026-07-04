package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Trilives/sboxkit/internal/config"
	"github.com/Trilives/sboxkit/internal/paths"
	"github.com/Trilives/sboxkit/internal/testkit"
)

func TestRunPrintsHelpWhenRequested(t *testing.T) {
	var stdout bytes.Buffer
	code := Run([]string{"--help"}, &stdout, &bytes.Buffer{})

	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}

	out := stdout.String()
	for _, want := range []string{
		"sboxkit",
		"init",
		"modify",
		"nettest",
		"uninstall",
		"update",
		"version",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("help output missing %q:\n%s", want, out)
		}
	}
}

func TestRunRejectsUnknownCommand(t *testing.T) {
	var stderr bytes.Buffer
	code := Run([]string{"missing"}, &bytes.Buffer{}, &stderr)

	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
	if !strings.Contains(stderr.String(), "unknown command") {
		t.Fatalf("expected unknown command error, got %q", stderr.String())
	}
}

func TestRunPrintsVersion(t *testing.T) {
	var stdout bytes.Buffer
	code := Run([]string{"version"}, &stdout, &bytes.Buffer{})

	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if !strings.Contains(stdout.String(), "sboxkit dev") {
		t.Fatalf("expected dev version output, got %q", stdout.String())
	}
}

func TestRunWritesFileLogWhenEnabled(t *testing.T) {
	root := t.TempDir()
	t.Setenv("SBOXKIT_ROOT", root)
	cfg := config.Defaults()
	cfg.EnableFileLog = true
	cfg.LogMaxMB = 1
	if err := config.Save(paths.FromRoot(root).CustomizeFile, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	var stderr bytes.Buffer
	code := Run([]string{"missing"}, &bytes.Buffer{}, &stderr)
	if code != 2 {
		t.Fatalf("Run missing = %d, want 2", code)
	}
	logs, err := os.ReadDir(paths.FromRoot(root).LogDir)
	if err != nil {
		t.Fatalf("read logs: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("logs = %d, want 1", len(logs))
	}
	data, err := os.ReadFile(filepath.Join(paths.FromRoot(root).LogDir, logs[0].Name()))
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	text := string(data)
	for _, want := range []string{"sboxkit [missing]", "unknown command: missing", "exit code: 2"} {
		if !strings.Contains(text, want) {
			t.Fatalf("log missing %q:\n%s", want, text)
		}
	}
}

func TestRunRecognizesPlannedCommands(t *testing.T) {
	var stdout bytes.Buffer
	code := Run([]string{"sub", "--help"}, &stdout, &bytes.Buffer{})
	if code != 0 {
		t.Fatalf("expected sub help exit code 0, got %d", code)
	}
	if !strings.Contains(stdout.String(), "add") {
		t.Fatalf("expected subcommand help, got %q", stdout.String())
	}
}

func TestInitNoTunCanWriteProxyEnvironment(t *testing.T) {
	root := t.TempDir()
	proxyEnvFile := filepath.Join(t.TempDir(), ".bashrc")
	var stdout bytes.Buffer
	code := Run([]string{
		"init",
		"--root", root,
		"--no-tun",
		"--write-proxy-env",
		"--proxy-env-file", proxyEnvFile,
	}, &stdout, &bytes.Buffer{})

	if code != 0 {
		t.Fatalf("expected init exit code 0, got %d", code)
	}
	configData, err := os.ReadFile(filepath.Join(root, "state", "customize.json"))
	if err != nil {
		t.Fatalf("read customize: %v", err)
	}
	if !strings.Contains(string(configData), `"enable_tun": false`) {
		t.Fatalf("expected enable_tun false in customize.json:\n%s", string(configData))
	}
	envData, err := os.ReadFile(proxyEnvFile)
	if err != nil {
		t.Fatalf("read proxy env file: %v", err)
	}
	if !strings.Contains(string(envData), `http://127.0.0.1:7890`) {
		t.Fatalf("expected proxy exports, got:\n%s", string(envData))
	}
}

func TestInitDefaultsToStableStateRoot(t *testing.T) {
	t.Setenv("SBOXKIT_ROOT", "")
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	root, _ := parseRoot(nil)
	if root != "/var/lib/sboxkit" {
		t.Fatalf("expected default root /var/lib/sboxkit, got %q", root)
	}
}

func TestSubAddLocalSingBoxFileAlwaysUsesConverter(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(t.TempDir(), "config.json")
	raw := `{
  "outbounds": [
    {"type": "shadowsocks", "tag": "local-node", "server": "127.0.0.1", "server_port": 8388, "method": "2022-blake3-aes-128-gcm", "password": "secret"}
  ]
}`
	if err := os.WriteFile(source, []byte(raw), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := Run([]string{
		"sub", "add",
		"--root", root,
		"--name", "file-json",
		"--file", source,
		"--source", "sing-box",
		"--passthrough",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected sub add exit code 0, got %d, stderr=%s", code, stderr.String())
	}

	data, err := os.ReadFile(filepath.Join(root, "state", "subscriptions", "file-json", "config.json"))
	if err != nil {
		t.Fatalf("read generated config: %v", err)
	}
	text := string(data)
	if strings.Contains(text, `"tag": "local-node"`) && !strings.Contains(text, `"tag": "Proxy"`) {
		t.Fatalf("local sing-box file was passed through instead of converted:\n%s", text)
	}
	if !strings.Contains(text, `"tag": "Proxy"`) {
		t.Fatalf("converted config should contain generated Proxy selector:\n%s", text)
	}
}

func TestSubOverwriteLocalCanReplaceExistingLocalSlot(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(t.TempDir(), "first.yaml")
	second := filepath.Join(t.TempDir(), "second.yaml")
	if err := os.WriteFile(first, []byte(`proxies:
  - {name: "first", type: ss, server: "127.0.0.1", port: 8388, cipher: "aes-128-gcm", password: "secret"}
`), 0o600); err != nil {
		t.Fatalf("write first: %v", err)
	}
	if err := os.WriteFile(second, []byte(`proxies:
  - {name: "second", type: ss, server: "127.0.0.2", port: 8388, cipher: "aes-128-gcm", password: "secret"}
`), 0o600); err != nil {
		t.Fatalf("write second: %v", err)
	}

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"sub", "overwrite-local", "--root", root, "--file", first}, &stdout, &stderr); code != 0 {
		t.Fatalf("first overwrite = %d, stderr=%s", code, stderr.String())
	}
	if code := Run([]string{"sub", "overwrite-local", "--root", root, "--file", second}, &stdout, &stderr); code != 0 {
		t.Fatalf("second overwrite = %d, stderr=%s", code, stderr.String())
	}

	data, err := os.ReadFile(filepath.Join(root, "state", "subscriptions", "local-overwrite", "raw.yaml"))
	if err != nil {
		t.Fatalf("read overwritten raw: %v", err)
	}
	if !strings.Contains(string(data), "second") || strings.Contains(string(data), "first") {
		t.Fatalf("local overwrite slot was not replaced:\n%s", data)
	}
	active, err := os.ReadFile(filepath.Join(root, "state", "active"))
	if err != nil {
		t.Fatalf("read active: %v", err)
	}
	if string(active) != "local-overwrite\n" {
		t.Fatalf("active = %q, want local-overwrite", active)
	}
}

func TestSubAddAcceptsLocalConfigFile(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(source, []byte(`proxies:
  - {name: "local", type: ss, server: "127.0.0.1", port: 8388, cipher: "aes-128-gcm", password: "secret"}
`), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}

	var stdout bytes.Buffer
	code := Run([]string{
		"sub", "add",
		"--root", root,
		"--name", "file",
		"--file", source,
	}, &stdout, &bytes.Buffer{})
	if code != 0 {
		t.Fatalf("expected sub add exit code 0, got %d", code)
	}
	if !strings.Contains(stdout.String(), "config file file ready") {
		t.Fatalf("unexpected output %q", stdout.String())
	}
	if _, err := os.Stat(filepath.Join(root, "state", "subscriptions", "file", "source.yaml")); err != nil {
		t.Fatalf("expected copied source: %v", err)
	}
}

func TestRunSubSwitchRebuildsStaleSubscriptionConfig(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(t.TempDir(), "source.yaml")
	if err := os.WriteFile(source, []byte(testkit.ReadFixture(t, "testdata/converter/clash-basic.yaml")), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}

	var stdout, stderr bytes.Buffer
	if code := runSub([]string{"add", "--root", root, "--name", "first", "--file", source}, &stdout, &stderr); code != 0 {
		t.Fatalf("add first = %d, stderr=%s", code, stderr.String())
	}
	if code := runSub([]string{"add", "--root", root, "--name", "second", "--file", source, "--no-active"}, &stdout, &stderr); code != 0 {
		t.Fatalf("add second = %d, stderr=%s", code, stderr.String())
	}

	p := paths.FromRoot(root)
	staleConfig := filepath.Join(p.SubscriptionsDir, "second", "config.json")
	if err := os.WriteFile(staleConfig, []byte(`{"stale":true}`), 0o644); err != nil {
		t.Fatalf("write stale config: %v", err)
	}

	if code := runSub([]string{"switch", "--root", root, "--name", "second"}, &stdout, &stderr); code != 0 {
		t.Fatalf("switch = %d, stderr=%s", code, stderr.String())
	}
	data, err := os.ReadFile(p.ConfigFile)
	if err != nil {
		t.Fatalf("read active config: %v", err)
	}
	if strings.Contains(string(data), `"stale":true`) {
		t.Fatalf("switch copied stale config instead of rebuilding: %s", data)
	}
	if !strings.Contains(stdout.String(), "rebuilt") {
		t.Fatalf("switch output should mention rebuild, got %q", stdout.String())
	}
}

func TestUninstallPrintsAptRemovalHint(t *testing.T) {
	var stdout bytes.Buffer
	code := Run([]string{"uninstall", "--root", t.TempDir(), "--keep-system"}, &stdout, &bytes.Buffer{})
	if code != 0 {
		t.Fatalf("expected uninstall exit code 0, got %d", code)
	}
	out := stdout.String()
	for _, want := range []string{"sudo apt remove sboxkit", "sudo apt purge sboxkit"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected uninstall output to contain %q, got:\n%s", want, out)
		}
	}
}

func TestServiceTrafficWarningPolicy(t *testing.T) {
	tests := []struct {
		name string
		cmd  string
		rest []string
		want bool
	}{
		{name: "install starts by default", cmd: "install", want: true},
		{name: "install no start", cmd: "install", rest: []string{"--no-start"}, want: false},
		{name: "sync restarts", cmd: "sync", want: true},
		{name: "start restarts service", cmd: "start", want: true},
		{name: "stop does not start", cmd: "stop", want: false},
		{name: "remove does not start", cmd: "remove", want: false},
		{name: "status does not start", cmd: "status", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := serviceCommandStartsOrRestarts(tt.cmd, tt.rest); got != tt.want {
				t.Fatalf("serviceCommandStartsOrRestarts() = %v, want %v", got, tt.want)
			}
		})
	}
	if !strings.Contains(serviceTrafficWarning(), "SSH") {
		t.Fatalf("warning should mention SSH risk: %q", serviceTrafficWarning())
	}
}
