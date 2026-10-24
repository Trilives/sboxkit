package sysd

import (
	"errors"
	"os"
	"testing"

	"github.com/Trilives/sboxkit/internal/paths"
)

// stubRuntime 替换服务探测与同步；同步成功时像 SyncAndRestart 一样记下部署版本。
func stubRuntime(t *testing.T, installed, active bool, syncErr error) *int {
	t.Helper()
	calls := 0
	origInstalled, origActive, origSync := runtimeInstalled, runtimeActive, runtimeSync
	runtimeInstalled = func(string) bool { return installed }
	runtimeActive = func(string) bool { return active }
	runtimeSync = func(p paths.Paths, _ string) error {
		calls++
		if syncErr != nil {
			return syncErr
		}
		return recordRuntimeVersion(p)
	}
	t.Cleanup(func() { runtimeInstalled, runtimeActive, runtimeSync = origInstalled, origActive, origSync })
	return &calls
}

func runtimeState(t *testing.T, version, deployed string) paths.Paths {
	t.Helper()
	orig := Version
	Version = version
	t.Cleanup(func() { Version = orig })
	p := paths.FromRoot(t.TempDir())
	if err := p.EnsureStateDirs(); err != nil {
		t.Fatal(err)
	}
	if deployed != "" {
		if err := os.WriteFile(p.RuntimeVersion, []byte(deployed+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return p
}

// 升级后首次运行整体同步一次（单元、面板、内核与规则集、运行时配置），之后不再打扰。
func TestReconcileRuntimeSyncsOnceAfterVersionChange(t *testing.T) {
	p := runtimeState(t, "0.3.3", "0.3.2")
	calls := stubRuntime(t, true, true, nil)

	ReconcileRuntime(p, DefaultName)
	ReconcileRuntime(p, DefaultName)

	if *calls != 1 {
		t.Fatalf("runtime synced %d times, want once per version change", *calls)
	}
	if got := deployedRuntimeVersion(p); got != "0.3.3" {
		t.Fatalf("deployed runtime version = %q, want 0.3.3", got)
	}
}

// 旧版本部署的服务没有版本记录：视为版本已变化。
func TestReconcileRuntimeSyncsServiceDeployedBeforeVersionStamp(t *testing.T) {
	p := runtimeState(t, "0.3.3", "")
	calls := stubRuntime(t, true, true, nil)

	ReconcileRuntime(p, DefaultName)

	if *calls != 1 {
		t.Fatalf("runtime synced %d times, want once", *calls)
	}
}

func TestReconcileRuntimeLeavesSameVersionAlone(t *testing.T) {
	p := runtimeState(t, "0.3.3", "0.3.3")
	calls := stubRuntime(t, true, true, nil)

	ReconcileRuntime(p, DefaultName)

	if *calls != 0 {
		t.Fatalf("unchanged version triggered %d syncs", *calls)
	}
}

// 未安装不注册；已暂停的服务不能被同步顺带启动。
func TestReconcileRuntimeSkipsMissingOrPausedService(t *testing.T) {
	for _, c := range []struct {
		name              string
		installed, active bool
	}{{"not installed", false, false}, {"paused", true, false}} {
		p := runtimeState(t, "0.3.3", "0.3.2")
		calls := stubRuntime(t, c.installed, c.active, nil)

		ReconcileRuntime(p, DefaultName)

		if *calls != 0 {
			t.Errorf("%s: runtime synced %d times", c.name, *calls)
		}
		if got := deployedRuntimeVersion(p); got != "0.3.2" {
			t.Errorf("%s: version stamp changed to %q", c.name, got)
		}
	}
}

// 同步失败（如校验不通过、未授权）不记版本：下次启动再试。
func TestReconcileRuntimeRetriesAfterFailedSync(t *testing.T) {
	p := runtimeState(t, "0.3.3", "0.3.2")
	calls := stubRuntime(t, true, true, errors.New("sudo declined"))

	ReconcileRuntime(p, DefaultName)
	ReconcileRuntime(p, DefaultName)

	if *calls != 2 {
		t.Fatalf("runtime synced %d times, want a retry on every start until it succeeds", *calls)
	}
	if got := deployedRuntimeVersion(p); got != "0.3.2" {
		t.Fatalf("failed sync recorded version %q", got)
	}
}

func TestRecordRuntimeVersionReplacesStamp(t *testing.T) {
	p := runtimeState(t, "0.3.3", "0.3.2")

	if err := recordRuntimeVersion(p); err != nil {
		t.Fatal(err)
	}

	if got := deployedRuntimeVersion(p); got != "0.3.3" {
		t.Fatalf("deployed runtime version = %q", got)
	}
	info, err := os.Stat(p.RuntimeVersion)
	if err != nil || info.Mode().Perm() != 0o644 {
		t.Fatalf("stamp must stay readable by the invoking user: %v %v", info.Mode(), err)
	}
}
