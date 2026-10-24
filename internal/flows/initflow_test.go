package flows

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Trilives/sboxkit/internal/paths"
	"github.com/Trilives/sboxkit/internal/sysd"
	"github.com/Trilives/sboxkit/internal/txn"
)

func TestStartupResourcesReadyAcceptsBundledDebSeeds(t *testing.T) {
	p := testPaths(t)
	writeTestFile(t, p.SingBoxBin)
	writeTestFile(t, p.GeositeCN)
	writeTestFile(t, p.GeoIPCN)

	if !startupResourcesReady(p) {
		t.Fatal("deb bundled sing-box + geosite-cn.srs + geoip-cn.srs should be enough to start")
	}
}

func TestStartupResourcesReadyRequiresKernelAndRuleSets(t *testing.T) {
	p := testPaths(t)
	writeTestFile(t, p.SingBoxBin)
	writeTestFile(t, p.GeositeCN)

	if startupResourcesReady(p) {
		t.Fatal("missing geoip-cn.srs should require a download before start")
	}
}

func testPaths(t *testing.T) paths.Paths {
	t.Helper()
	return paths.FromRoot(t.TempDir())
}

func writeTestFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func stubServiceUnits(t *testing.T, installed bool, installErr error) *int {
	t.Helper()
	removed := 0
	origInstalled, origInstall, origRemove := serviceInstalled, installUnit, removeUnit
	serviceInstalled = func() bool { return installed }
	installUnit = func(paths.Paths) error { return installErr }
	removeUnit = func(paths.Paths) error { removed++; return nil }
	t.Cleanup(func() { serviceInstalled, installUnit, removeUnit = origInstalled, origInstall, origRemove })
	return &removed
}

func TestInstallServiceRollbackNeverRemovesExistingService(t *testing.T) {
	for _, installErr := range []error{
		&sysd.PrecheckError{Err: errors.New("rejected")},
		errors.New("restart failed"),
	} {
		removed := stubServiceUnits(t, true, installErr)
		session := txn.New("test")
		if err := installService(paths.Paths{}, session); err == nil {
			t.Fatal("install error swallowed")
		}
		session.Rollback()
		if *removed != 0 {
			t.Fatalf("existing service removed after %v", installErr)
		}
	}
}

func TestInstallServiceRollbackRemovesOnlyDeployedNewService(t *testing.T) {
	removed := stubServiceUnits(t, false, &sysd.PrecheckError{Err: errors.New("rejected")})
	session := txn.New("test")
	installService(paths.Paths{}, session)
	session.Rollback()
	if *removed != 0 {
		t.Fatal("rollback removed a service that the failed check never deployed")
	}

	removed = stubServiceUnits(t, false, errors.New("restart failed"))
	session = txn.New("test")
	installService(paths.Paths{}, session)
	session.Rollback()
	if *removed != 1 {
		t.Fatalf("half-installed new service not removed: %d", *removed)
	}
}
