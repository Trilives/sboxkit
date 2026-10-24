package kernel

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Trilives/sboxkit/internal/paths"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readText(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func stagedCoreAndGeo(t *testing.T, p paths.Paths) *Staged {
	t.Helper()
	s, err := NewStaging(p)
	if err != nil {
		t.Fatal(err)
	}
	s.Kernel, s.Version = filepath.Join(s.Dir(), "sing-box"), "1.15.0"
	s.Geosite = filepath.Join(s.Dir(), "geosite-cn.srs")
	writeFile(t, s.Kernel, "core-new")
	writeFile(t, s.Geosite, "geosite-new")
	return s
}

func TestStagedBundleMixesStagedAndStateFiles(t *testing.T) {
	p := paths.FromRoot(t.TempDir())
	s := stagedCoreAndGeo(t, p)
	defer s.Discard()

	b := s.Bundle(p)

	if b.Kernel != s.Kernel || b.Geosite != s.Geosite || b.GeoIP != p.GeoIPCN {
		t.Fatalf("bundle = %+v", b)
	}
}

func TestStagedCommitSwapsFilesAndRestoreBringsBackPrevious(t *testing.T) {
	p := paths.FromRoot(t.TempDir())
	writeFile(t, p.SingBoxBin, "core-old")
	writeFile(t, p.SingBoxVersion, "1.14.3\n")
	writeFile(t, p.GeositeCN, "geosite-old")
	writeFile(t, p.GeoIPCN, "geoip-old")
	s := stagedCoreAndGeo(t, p)
	defer s.Discard()

	restore, err := s.Commit(p)
	if err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{
		p.SingBoxBin: "core-new", p.SingBoxVersion: "1.15.0\n", p.GeositeCN: "geosite-new", p.GeoIPCN: "geoip-old",
	} {
		if got := readText(t, path); got != want {
			t.Fatalf("after commit %s = %q, want %q", path, got, want)
		}
	}

	if err := restore(); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{
		p.SingBoxBin: "core-old", p.SingBoxVersion: "1.14.3\n", p.GeositeCN: "geosite-old",
	} {
		if got := readText(t, path); got != want {
			t.Fatalf("after restore %s = %q, want %q", path, got, want)
		}
	}
}

func TestStagedRestoreRemovesFilesThatDidNotExist(t *testing.T) {
	p := paths.FromRoot(t.TempDir())
	s := stagedCoreAndGeo(t, p)
	defer s.Discard()

	restore, err := s.Commit(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := restore(); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{p.SingBoxBin, p.SingBoxVersion, p.GeositeCN} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("%s should be gone after restore: %v", path, err)
		}
	}
}

func TestStagedCommitFailureLeavesStateUntouched(t *testing.T) {
	p := paths.FromRoot(t.TempDir())
	writeFile(t, p.SingBoxBin, "core-old")
	writeFile(t, p.GeositeCN, "geosite-old")
	s := stagedCoreAndGeo(t, p)
	defer s.Discard()
	if err := os.Remove(s.Geosite); err != nil { // 第二个换入失败
		t.Fatal(err)
	}

	if _, err := s.Commit(p); err == nil {
		t.Fatal("missing staged file accepted")
	}
	if got := readText(t, p.SingBoxBin); got != "core-old" {
		t.Fatalf("kernel = %q, want the old kernel back", got)
	}
	if got := readText(t, p.GeositeCN); got != "geosite-old" {
		t.Fatalf("geosite = %q", got)
	}
}

func TestStagedDiscardRemovesStagingDirectory(t *testing.T) {
	p := paths.FromRoot(t.TempDir())
	s := stagedCoreAndGeo(t, p)

	s.Discard()

	if _, err := os.Stat(s.Dir()); !os.IsNotExist(err) {
		t.Fatalf("staging directory left behind: %v", err)
	}
}
