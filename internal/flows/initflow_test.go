package flows

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Trilives/sboxkit/internal/paths"
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
