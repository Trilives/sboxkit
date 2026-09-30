package sysd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Trilives/sboxkit/internal/execx"
	"github.com/Trilives/sboxkit/internal/paths"
)

// userModeScope 构造临时用户级 scope，并假定 systemd 用户实例可达（测试环境未必有）。
func userModeScope(t *testing.T) unitScope {
	t.Helper()
	stubUserManager(t, nil)
	root := t.TempDir()
	return unitScope{
		User:       true,
		UnitDir:    filepath.Join(root, "systemd-user"),
		RuntimeDir: filepath.Join(root, "kit dir", ".sboxkit", "runtime"),
		State:      filepath.Join(root, "kit dir", ".sboxkit"),
	}
}

func TestScopeForUserModePathsUsesUserManager(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/home/u/.config")
	p := paths.FromUserModeRoot("/home/u/kit/.sboxkit", "/home/u/kit")

	s := scopeFor(p)

	if !s.User || s.UnitDir != "/home/u/.config/systemd/user" || s.RuntimeDir != "/home/u/kit/.sboxkit/runtime" {
		t.Fatalf("userMode scope = %+v", s)
	}
	if got := strings.Join(s.systemctl("restart", "sing-box.service"), " "); got != "systemctl --user restart sing-box.service" {
		t.Fatalf("systemctl command = %q", got)
	}
}

func TestScopeForSystemPathsKeepsRootLayout(t *testing.T) {
	s := scopeFor(paths.FromRoot("/var/lib/sboxkit"))

	if s.User || s.UnitDir != systemUnitDir || s.RuntimeDir != paths.RuntimeDir {
		t.Fatalf("system scope = %+v", s)
	}
	if got := strings.Join(s.systemctl("restart", "x"), " "); got != "systemctl restart x" {
		t.Fatalf("systemctl command = %q", got)
	}
}

func TestUserMainUnitRunsWithoutRootOrCapabilities(t *testing.T) {
	s := userModeScope(t)
	rt := rtPathsFor(s, DefaultName)

	unit, err := renderUnit(DefaultName, rt)
	if err != nil {
		t.Fatal(err)
	}

	for _, forbidden := range []string{"User=root", "CAP_NET_ADMIN", "multi-user.target", "network-online.target"} {
		if strings.Contains(unit, forbidden) {
			t.Fatalf("user unit contains %q:\n%s", forbidden, unit)
		}
	}
	for _, want := range []string{
		"WantedBy=default.target",
		"Requires=sing-box-bootstrap-dns.service",
		fmt.Sprintf("ExecStart=%q run -c %q", rt.Bin, rt.Config),
		"WorkingDirectory=" + rt.Dir,
	} {
		if !strings.Contains(unit, want) {
			t.Fatalf("user unit missing %q:\n%s", want, unit)
		}
	}
	if rt.Unit != filepath.Join(s.UnitDir, "sing-box.service") {
		t.Fatalf("user unit path = %q", rt.Unit)
	}
}

func TestUserBootstrapUnitCarriesUserModeEnvironment(t *testing.T) {
	s := userModeScope(t)
	unit := bootstrapServiceText(DefaultName, rtPathsFor(s, DefaultName), "/home/u/kit/sboxkit")

	for _, want := range []string{
		"Environment=SBOXKIT_USER_MODE=1",
		fmt.Sprintf("Environment=%q", "SBOXKIT_HOME="+s.State),
		"PartOf=sing-box.service",
		"NoNewPrivileges=true",
		"WantedBy=default.target",
	} {
		if !strings.Contains(unit, want) {
			t.Fatalf("user relay unit missing %q:\n%s", want, unit)
		}
	}
	for _, forbidden := range []string{"User=root", "CapabilityBoundingSet", "ProtectSystem", "ProtectHome"} {
		if strings.Contains(unit, forbidden) {
			t.Fatalf("user relay unit contains %q:\n%s", forbidden, unit)
		}
	}
}

func TestUserWatchdogAndTimerPinUserModeState(t *testing.T) {
	s := userModeScope(t)

	for name, unit := range map[string]string{
		"watchdog": wdServiceText(s, DefaultName, "singbox", "/home/u/kit/sboxkit"),
		"timer":    timerServiceText(s, "/home/u/kit/sboxkit"),
	} {
		if !strings.Contains(unit, "Environment=SBOXKIT_USER_MODE=1") || !strings.Contains(unit, "SBOXKIT_HOME="+s.State) {
			t.Fatalf("%s unit lacks userMode environment:\n%s", name, unit)
		}
	}
	if unit := wdServiceText(unitScope{}, DefaultName, "singbox", "/usr/bin/sboxkit"); strings.Contains(unit, "SBOXKIT_USER_MODE") {
		t.Fatalf("system watchdog must not pin userMode mode:\n%s", unit)
	}
}

func TestUserScopeResilienceDoesNotRequireDispatcher(t *testing.T) {
	s := userModeScope(t)
	layout := layoutFor(s)
	if err := os.MkdirAll(layout.systemdDir, 0o755); err != nil {
		t.Fatal(err)
	}
	exe := "/home/u/kit/sboxkit"
	files := map[string]string{
		wdServiceIn(layout): wdServiceText(s, DefaultName, "singbox", exe),
		wdTimerIn(layout):   wdTimerText("2min"),
	}
	for path, content := range files {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	st := inspectResilienceStatus(layout, DefaultName, "2min", "singbox", exe, true, false, true)

	if st.DispatcherSupported || st.NeedsRepair || !st.Complete() {
		t.Fatalf("user scope watchdog should be complete without NM dispatcher: %#v", st)
	}
}

func TestStageRuntimeConfigKeepsTunInUserScope(t *testing.T) {
	state := t.TempDir()
	p := paths.Paths{State: state, ConfigFile: filepath.Join(state, "config.json")}
	raw := `{"inbounds":[{"type":"tun","tag":"tun-in"},{"type":"mixed","tag":"mixed-in"}],"outbounds":[]}`
	if err := os.WriteFile(p.ConfigFile, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}

	staged, err := stageRuntimeConfig(p, rtPathsFor(userModeScope(t), DefaultName))
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(staged)

	var doc map[string]any
	out, _ := os.ReadFile(staged)
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatal(err)
	}
	if !configNeedsNetAdmin(doc) {
		t.Fatalf("user mode must keep the TUN inbound (capabilities are granted instead):\n%s", out)
	}
}

func TestRemoveBootstrapDNSUsesUserManagerInUserModeScope(t *testing.T) {
	rt := rtPathsFor(userModeScope(t), DefaultName)
	var commands []string
	run := func(cmd []string, _ string, _ *execx.Opt) (execx.Result, error) {
		commands = append(commands, strings.Join(cmd, " "))
		return execx.Result{}, nil
	}

	removeBootstrapDNSWith(DefaultName, rt, true, run)

	if len(commands) < 2 || commands[0] != "systemctl --user stop sing-box-bootstrap-dns.service" {
		t.Fatalf("commands = %v", commands)
	}
}

func TestHealthRuntimeDirForUserModeAvoidsRootOwnedRun(t *testing.T) {
	userMode := paths.FromUserModeRoot("/home/u/kit/.sboxkit", "/home/u/kit")

	if got := healthRuntimeDirFor(userMode, "/run/user/1000"); got != "/run/user/1000/sboxkit" {
		t.Fatalf("with XDG_RUNTIME_DIR = %q", got)
	}
	if got := healthRuntimeDirFor(userMode, ""); got != "/home/u/kit/.sboxkit/run" {
		t.Fatalf("without XDG_RUNTIME_DIR = %q", got)
	}
	if got := healthRuntimeDirFor(paths.FromRoot("/var/lib/sboxkit"), "/run/user/0"); got != defaultHealthRuntimeDir {
		t.Fatalf("system = %q", got)
	}
}

func TestWriteFileAtomicAppliesModeAndCreatesParents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a", "b", "unit.service")

	if err := writeFileAtomic(path, "hello", "0640"); err != nil {
		t.Fatal(err)
	}

	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o640 {
		t.Fatalf("mode = %v", st.Mode().Perm())
	}
	if err := writeFileAtomic(path, "x", "not-octal"); err == nil {
		t.Fatal("invalid mode accepted")
	}
}

func TestCurrentScopeFollowsUserModeEnvironment(t *testing.T) {
	config := t.TempDir()
	t.Setenv("SBOXKIT_USER_MODE", "1")
	t.Setenv("SBOXKIT_HOME", filepath.Join(t.TempDir(), ".sboxkit"))
	t.Setenv("XDG_CONFIG_HOME", config)

	if !UserScope() {
		t.Fatal("SBOXKIT_USER_MODE=1 must select the user scope")
	}
	if got := strings.Join(JournalCommand(DefaultName, "-n", "5"), " "); got != "journalctl --user -n 5 -u sing-box.service" {
		t.Fatalf("journal command = %q", got)
	}
	if got := UnitPath(DefaultName); got != filepath.Join(config, "systemd", "user", "sing-box.service") {
		t.Fatalf("unit path = %q", got)
	}
}

func TestUserScopeRunsAndWritesWithoutSudo(t *testing.T) {
	s := userModeScope(t)
	target := filepath.Join(s.UnitDir, "probe.service")

	if err := s.ensurePrivilege("test"); err != nil {
		t.Fatalf("user scope asked for privileges: %v", err)
	}
	res, err := s.run([]string{"id", "-u"}, "", &execx.Opt{Capture: true})
	if err != nil || strings.TrimSpace(res.Stdout) != fmt.Sprint(os.Getuid()) {
		t.Fatalf("user scope run = %q, %v; want direct execution as uid %d", res.Stdout, err, os.Getuid())
	}
	if err := s.write(target, "[Unit]\n", "0644", ""); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(target); string(got) != "[Unit]\n" {
		t.Fatalf("written unit = %q", got)
	}
}

func TestUserExecStartEscapesDollarButEnvironmentDoesNot(t *testing.T) {
	s := userModeScope(t)
	s.State = "/home/u/$build/.sboxkit"
	exe := "/home/u/$build/sboxkit"

	unit := wdServiceText(s, DefaultName, "singbox", exe)

	if !strings.Contains(unit, `ExecStart="/home/u/$$build/sboxkit" healthcheck`) {
		t.Fatalf("ExecStart does not escape $:\n%s", unit)
	}
	if !strings.Contains(unit, `Environment="SBOXKIT_HOME=/home/u/$build/.sboxkit"`) {
		t.Fatalf("Environment must keep $ literal:\n%s", unit)
	}
}

func stubUserManager(t *testing.T, err error) {
	t.Helper()
	orig := userManagerProbe
	userManagerProbe = func(unitScope) error { return err }
	t.Cleanup(func() { userManagerProbe = orig })
}

func TestUserScopePrivilegeCheckReportsUnreachableUserManager(t *testing.T) {
	s := userModeScope(t)
	stubUserManager(t, errors.New("no user bus"))

	if err := s.ensurePrivilege("pause"); err == nil {
		t.Fatal("user scope must fail clearly when systemctl --user is unreachable")
	}
}

func TestScopeConflictBlocksOnlyRunningSystemService(t *testing.T) {
	if err := scopeConflict("/etc/systemd/system/sing-box.service", true); err == nil {
		t.Fatal("running system service would make the user service crash-loop on its ports")
	}
	if err := scopeConflict("/etc/systemd/system/sing-box.service", false); err != nil {
		t.Fatalf("installed but stopped system service should only warn: %v", err)
	}
}
