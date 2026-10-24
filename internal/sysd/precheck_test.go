package sysd

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Trilives/sboxkit/internal/compat"
	"github.com/Trilives/sboxkit/internal/paths"
)

// rejectingKernel 写一个最小的 sing-box 替身：check 拒绝含 REJECT 的配置。
func rejectingKernel(t *testing.T, path string) {
	t.Helper()
	script := `#!/bin/sh
case "$1" in
version) echo "sing-box version 1.14.3" ;;
check) if grep -q REJECT "$3"; then echo "FATAL[0000] legacy rule rejected" >&2; exit 1; fi ;;
esac
`
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}

// installedUserService 构造已安装的用户模式服务：环境变量指向临时目录，
// IsInstalled / scopeFor 均落在该目录，不触及真实 systemd 单元。
func installedUserService(t *testing.T, stateConfig string) (paths.Paths, runtimePaths) {
	t.Helper()
	stubUserManager(t, nil)
	root := t.TempDir()
	t.Setenv("SBOXKIT_USER_MODE", "1")
	t.Setenv("SBOXKIT_HOME", filepath.Join(root, "state"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	p := paths.Detect()
	if err := p.EnsureStateDirs(); err != nil {
		t.Fatal(err)
	}
	rt := rtPathsFor(scopeFor(p), DefaultName)
	rejectingKernel(t, p.SingBoxBin)
	files := map[string]string{
		p.ConfigFile: stateConfig,
		p.GeositeCN:  "geosite-new",
		p.GeoIPCN:    "geoip-new",
		rt.Bin:       "core-running",
		rt.Config:    "config-running",
		rt.Geosite:   "geosite-running",
		rt.Unit:      "[Unit]\n",
	}
	for path, content := range files {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return p, rt
}

func TestSyncRejectedByDeployKernelLeavesRuntimeAndUnitUntouched(t *testing.T) {
	p, rt := installedUserService(t, `{"log":{"level":"REJECT"}}`)

	err := SyncAndRestart(p, DefaultName)

	var pre *PrecheckError
	if !errors.As(err, &pre) {
		t.Fatalf("expected PrecheckError, got %v", err)
	}
	var rejected *compat.Error
	if !errors.As(err, &rejected) || rejected.Kernel != "1.14.3" {
		t.Fatalf("rejection should carry the kernel verdict: %v", err)
	}
	for path, want := range map[string]string{
		rt.Bin:     "core-running",
		rt.Config:  "config-running",
		rt.Geosite: "geosite-running",
		rt.Unit:    "[Unit]\n",
	} {
		if got := readFile(t, path); got != want {
			t.Fatalf("%s changed to %q despite the failed check", path, got)
		}
	}
	if matches, _ := filepath.Glob(filepath.Join(p.State, ".runtime-config.*")); len(matches) != 0 {
		t.Fatalf("staged runtime config left behind: %v", matches)
	}
}

func TestPrepareRuntimeReturnsValidatedStagedConfig(t *testing.T) {
	p, _ := installedUserService(t, `{"log":{"level":"info"}}`)

	_, staged, err := prepareRuntime(p, DefaultName, "test", false)
	if err != nil {
		t.Fatalf("compatible config rejected: %v", err)
	}
	defer os.Remove(staged)
	if _, err := os.Stat(staged); err != nil {
		t.Fatalf("staged config missing: %v", err)
	}
}

func TestPreflightFailureIsReportedAsPrecheck(t *testing.T) {
	p, rt := installedUserService(t, `{}`)
	if err := os.Remove(p.ConfigFile); err != nil {
		t.Fatal(err)
	}

	var pre *PrecheckError
	if err := SyncAndRestart(p, DefaultName); !errors.As(err, &pre) {
		t.Fatalf("missing state config must fail before deployment, got %v", err)
	}
	if got := readFile(t, rt.Config); got != "config-running" {
		t.Fatalf("runtime config changed: %q", got)
	}
}

// 用户模式 + TUN 无法无人值守授予能力时服务继续使用运行时旧内核，校验也须用它。
func TestDeployKernelFollowsDeferredCoreSwitch(t *testing.T) {
	f := newDeployFixture(t, "core-v2", "core-v1", tunConfig)
	stubCanElevate(t, false)

	got, err := deployKernel(f.p, f.rt, f.staged)
	if err != nil || got != f.rt.Bin {
		t.Fatalf("deploy kernel = %q, %v; want the running core %s", got, err, f.rt.Bin)
	}

	f = newDeployFixture(t, "core-v2", "core-v1", mixedConfig)
	if got, err := deployKernel(f.p, f.rt, f.staged); err != nil || got != f.p.SingBoxBin {
		t.Fatalf("deploy kernel = %q, %v; want the state core %s", got, err, f.p.SingBoxBin)
	}
}
