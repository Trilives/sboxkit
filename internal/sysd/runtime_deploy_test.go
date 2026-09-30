package sysd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Trilives/sboxkit/internal/paths"
)

const (
	mixedConfig = `{"inbounds":[{"type":"mixed"}]}`
	tunConfig   = `{"inbounds":[{"type":"tun"}]}`
)

type deployFixture struct {
	p      paths.Paths
	rt     runtimePaths
	staged string
}

// newDeployFixture 构造用户模式的 state 与运行时：state 内核为 stateCore，
// 运行时内核为 runtimeCore（空串表示运行时尚无内核），暂存配置为 config。
func newDeployFixture(t *testing.T, stateCore, runtimeCore, config string) deployFixture {
	t.Helper()
	s := userModeScope(t)
	p := paths.FromUserModeRoot(s.State, filepath.Dir(s.State))
	rt := rtPathsFor(s, DefaultName)
	files := map[string]string{
		p.SingBoxBin: stateCore,
		p.GeositeCN:  "geosite-new",
		p.GeoIPCN:    "geoip-new",
	}
	if runtimeCore != "" {
		files[rt.Bin] = runtimeCore
		files[rt.Geosite] = "geosite-old"
	}
	staged := filepath.Join(t.TempDir(), "staged.json")
	files[staged] = config
	for path, content := range files {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return deployFixture{p: p, rt: rt, staged: staged}
}

func stubCanElevate(t *testing.T, ok bool) {
	t.Helper()
	orig := canElevate
	canElevate = func() bool { return ok }
	t.Cleanup(func() { canElevate = orig })
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestPlanCoreDeployDecisions(t *testing.T) {
	cases := []struct {
		name        string
		runtimeCore string
		needCaps    bool
		elevate     bool
		want        coreAction
	}{
		{"unchanged core keeps file capabilities", "core-v2", true, false, coreKeep},
		{"changed core without TUN needs no sudo", "core-v1", false, false, coreReplace},
		{"changed core with TUN and sudo available", "core-v1", true, true, coreReplace},
		{"changed core with TUN, unattended", "core-v1", true, false, coreDefer},
		{"first deployment cannot keep a missing core", "", true, false, coreReplace},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newDeployFixture(t, "core-v2", tc.runtimeCore, mixedConfig)
			stubCanElevate(t, tc.elevate)

			if got := planCoreDeploy(f.p, f.rt, tc.needCaps); got != tc.want {
				t.Fatalf("planCoreDeploy = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPlanCoreDeployAlwaysReplacesInSystemScope(t *testing.T) {
	stubCanElevate(t, false)
	p := paths.FromRoot(t.TempDir())

	if got := planCoreDeploy(p, rtPathsFor(unitScope{}, DefaultName), true); got != coreReplace {
		t.Fatalf("system scope = %v, want coreReplace (capabilities come from the unit)", got)
	}
}

func TestDeployRuntimeAssetsInstallsUpdatedCoreAndRules(t *testing.T) {
	f := newDeployFixture(t, "core-v2", "core-v1", mixedConfig)
	stubCanElevate(t, false)

	if err := deployRuntimeAssets(f.p, f.rt, f.staged); err != nil {
		t.Fatal(err)
	}

	if got := readFile(t, f.rt.Bin); got != "core-v2" {
		t.Fatalf("runtime core = %q, want the updated core", got)
	}
	if got := readFile(t, f.rt.Geosite); got != "geosite-new" {
		t.Fatalf("runtime geosite = %q, want the updated rules", got)
	}
	if _, err := os.Stat(f.rt.Bin + ".new"); !os.IsNotExist(err) {
		t.Fatalf("staging binary left behind: %v", err)
	}
}

func TestDeployRuntimeAssetsKeepsOldCoreWhenTunUpdateIsUnattended(t *testing.T) {
	f := newDeployFixture(t, "core-v2", "core-v1", tunConfig)
	stubCanElevate(t, false)

	if err := deployRuntimeAssets(f.p, f.rt, f.staged); err != nil {
		t.Fatalf("unattended TUN update must not fail: %v", err)
	}

	if got := readFile(t, f.rt.Bin); got != "core-v1" {
		t.Fatalf("runtime core = %q, want the old core kept (it holds the capabilities)", got)
	}
	if got := readFile(t, f.rt.Geosite); got != "geosite-new" {
		t.Fatalf("rules must still be updated, got %q", got)
	}
}

func TestDeployRuntimeAssetsRejectsUnreadableStagedConfig(t *testing.T) {
	f := newDeployFixture(t, "core-v2", "core-v1", "{not json")

	if err := deployRuntimeAssets(f.p, f.rt, f.staged); err == nil {
		t.Fatal("broken staged config accepted")
	}
	if got := readFile(t, f.rt.Bin); got != "core-v1" {
		t.Fatalf("runtime core replaced despite the error: %q", got)
	}
}

func TestCorePendingOnlyForInstalledUserServiceWithStaleCore(t *testing.T) {
	f := newDeployFixture(t, "core-v2", "core-v1", tunConfig)
	if corePending(f.p, f.rt) {
		t.Fatal("no service installed yet, nothing to warn about")
	}
	if err := os.MkdirAll(filepath.Dir(f.rt.Unit), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.rt.Unit, []byte("[Unit]\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if !corePending(f.p, f.rt) {
		t.Fatal("stale runtime core not reported")
	}
	if err := os.WriteFile(f.rt.Bin, []byte("core-v2"), 0o755); err != nil {
		t.Fatal(err)
	}
	if corePending(f.p, f.rt) {
		t.Fatal("up-to-date runtime core reported as pending")
	}
}
