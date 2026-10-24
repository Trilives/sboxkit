package deploy

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Trilives/sboxkit/internal/i18n"
	"github.com/Trilives/sboxkit/internal/paths"
	"github.com/Trilives/sboxkit/internal/sysd"
	"github.com/Trilives/sboxkit/internal/txn"
)

func TestMain(m *testing.M) {
	i18n.SetLang(i18n.ZH)
	os.Exit(m.Run())
}

// fakeService 记录每次同步时看到的生效配置。
type fakeService struct {
	installed bool
	errs      []error
	seen      []string
}

func stubService(t *testing.T, p paths.Paths, svc *fakeService) {
	t.Helper()
	origInstalled, origSync := serviceInstalled, syncService
	serviceInstalled = func() bool { return svc.installed }
	syncService = func(got paths.Paths) error {
		if got.ConfigFile != p.ConfigFile {
			t.Fatalf("synced unexpected state %s", got.State)
		}
		raw, _ := os.ReadFile(p.ConfigFile)
		svc.seen = append(svc.seen, string(raw))
		if len(svc.errs) == 0 {
			return nil
		}
		err := svc.errs[0]
		svc.errs = svc.errs[1:]
		return err
	}
	t.Cleanup(func() { serviceInstalled, syncService = origInstalled, origSync })
}

func stateWith(t *testing.T, config string) paths.Paths {
	t.Helper()
	p := paths.FromRoot(t.TempDir())
	if err := p.EnsureStateDirs(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.ConfigFile, []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func writeConfig(p paths.Paths, config string) func(*txn.Transaction) error {
	return func(*txn.Transaction) error { return os.WriteFile(p.ConfigFile, []byte(config), 0o644) }
}

func configText(t *testing.T, p paths.Paths) string {
	t.Helper()
	raw, err := os.ReadFile(p.ConfigFile)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestApplyCommitsAndSyncsInstalledService(t *testing.T) {
	p := stateWith(t, "old")
	svc := &fakeService{installed: true}
	stubService(t, p, svc)

	err := Apply(p, Change{Name: "test", Snapshot: []string{p.ConfigFile}, Commit: writeConfig(p, "new"), Sync: true})

	if err != nil || configText(t, p) != "new" {
		t.Fatalf("apply = %v, config %q", err, configText(t, p))
	}
	if len(svc.seen) != 1 || svc.seen[0] != "new" {
		t.Fatalf("service synced %v, want once with the new config", svc.seen)
	}
}

func TestApplySkipsSyncWhenServiceIsNotInstalled(t *testing.T) {
	p := stateWith(t, "old")
	svc := &fakeService{}
	stubService(t, p, svc)

	if err := Apply(p, Change{Name: "test", Snapshot: []string{p.ConfigFile}, Commit: writeConfig(p, "new"), Sync: true}); err != nil {
		t.Fatal(err)
	}
	if len(svc.seen) != 0 {
		t.Fatalf("uninstalled service synced: %v", svc.seen)
	}
}

func TestApplyRestoresStateWhenCommitFails(t *testing.T) {
	p := stateWith(t, "old")
	svc := &fakeService{installed: true}
	stubService(t, p, svc)
	boom := errors.New("disk full")

	err := Apply(p, Change{Name: "test", Snapshot: []string{p.ConfigFile, p.ActiveFile}, Sync: true,
		Commit: func(t *txn.Transaction) error {
			os.WriteFile(p.ConfigFile, []byte("half"), 0o644)
			os.WriteFile(p.ActiveFile, []byte("other\n"), 0o644)
			return boom
		}})

	if !errors.Is(err, boom) || configText(t, p) != "old" {
		t.Fatalf("apply = %v, config %q; want the original config", err, configText(t, p))
	}
	if _, err := os.Stat(p.ActiveFile); !os.IsNotExist(err) {
		t.Fatalf("new active pointer survived the rollback: %v", err)
	}
	if len(svc.seen) != 0 {
		t.Fatalf("runtime touched after a failed commit: %v", svc.seen)
	}
}

// 前置校验失败时运行时未被改动：只还原状态目录，不再重启服务。
func TestApplyRestoresStateWithoutResyncWhenPrecheckFails(t *testing.T) {
	p := stateWith(t, "old")
	svc := &fakeService{installed: true, errs: []error{&sysd.PrecheckError{Err: errors.New("rejected")}}}
	stubService(t, p, svc)

	err := Apply(p, Change{Name: "test", Snapshot: []string{p.ConfigFile}, Commit: writeConfig(p, "new"), Sync: true})

	var pre *sysd.PrecheckError
	if !errors.As(err, &pre) || configText(t, p) != "old" {
		t.Fatalf("apply = %v, config %q", err, configText(t, p))
	}
	if len(svc.seen) != 1 {
		t.Fatalf("unchanged runtime was resynced: %v", svc.seen)
	}
}

// 运行时已被改动（例如重启失败）时，还原状态目录后再按旧状态重新同步。
func TestApplyResyncsRestoredStateWhenDeploymentFails(t *testing.T) {
	p := stateWith(t, "old")
	svc := &fakeService{installed: true, errs: []error{errors.New("restart failed")}}
	stubService(t, p, svc)
	var order []string

	err := Apply(p, Change{Name: "test", Snapshot: []string{p.ConfigFile}, Sync: true,
		Commit: func(t *txn.Transaction) error {
			t.AddUndo("restore assets", func() error { order = append(order, "assets"); return nil })
			return os.WriteFile(p.ConfigFile, []byte("new"), 0o644)
		}})

	if err == nil || configText(t, p) != "old" {
		t.Fatalf("apply = %v, config %q", err, configText(t, p))
	}
	if len(svc.seen) != 2 || svc.seen[1] != "old" {
		t.Fatalf("service synced %v, want a resync with the restored config", svc.seen)
	}
	if len(order) != 1 {
		t.Fatalf("asset restore not run before resync: %v", order)
	}
}

func TestApplyRestoresCreatedSubscriptionDirectory(t *testing.T) {
	p := stateWith(t, "old")
	stubService(t, p, &fakeService{})
	dir := filepath.Join(p.Subscriptions, "new-sub")

	err := Apply(p, Change{Name: "test", Snapshot: []string{dir}, Commit: func(*txn.Transaction) error {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		return errors.New("convert failed")
	}})

	if err == nil {
		t.Fatal("commit error swallowed")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("new subscription directory survived the rollback: %v", err)
	}
}

func TestApplyReportsFailedResyncAfterRollback(t *testing.T) {
	p := stateWith(t, "old")
	restartErr, resyncErr := errors.New("restart failed"), errors.New("old config rejected")
	stubService(t, p, &fakeService{installed: true, errs: []error{restartErr, resyncErr}})

	err := Apply(p, Change{Name: "test", Snapshot: []string{p.ConfigFile}, Commit: writeConfig(p, "new"), Sync: true})

	if !errors.Is(err, restartErr) || !errors.Is(err, resyncErr) {
		t.Fatalf("caller must learn that the resync failed too: %v", err)
	}
	if configText(t, p) != "old" {
		t.Fatalf("state not restored: %q", configText(t, p))
	}
}
