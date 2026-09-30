package kernel

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Trilives/sboxkit/internal/paths"
)

func writeSeed(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSeedFromUserModeBundleTakesOverCoreAndRules(t *testing.T) {
	root := t.TempDir()
	writeSeed(t, filepath.Join(root, "core", "sing-box"), "core")
	writeSeed(t, filepath.Join(root, "core", "sing-box.version"), "v1.13.19\n")
	writeSeed(t, filepath.Join(root, "ruleset", "geosite-cn.srs"), "site")
	writeSeed(t, filepath.Join(root, "ruleset", "geoip-cn.srs"), "ip")
	p := paths.FromUserModeRoot(filepath.Join(root, ".sboxkit"), root)

	seeded, err := SeedFromSystem(p)

	if err != nil {
		t.Fatal(err)
	}
	if len(seeded) != 3 {
		t.Fatalf("seeded = %v, want core + two rule-sets", seeded)
	}
	version, _ := os.ReadFile(p.SingBoxVersion)
	if strings.TrimSpace(string(version)) != "v1.13.19" {
		t.Fatalf("recorded version = %q", version)
	}
	st, err := os.Stat(p.SingBoxBin)
	if err != nil || st.Mode().Perm()&0o100 == 0 {
		t.Fatalf("seeded core is not executable: %v %v", st, err)
	}
}

func TestSeedFromUserModeBundleKeepsExistingState(t *testing.T) {
	root := t.TempDir()
	writeSeed(t, filepath.Join(root, "core", "sing-box"), "bundled")
	p := paths.FromUserModeRoot(filepath.Join(root, ".sboxkit"), root)
	writeSeed(t, p.SingBoxBin, "updated")

	if _, err := SeedFromSystem(p); err != nil {
		t.Fatal(err)
	}

	got, _ := os.ReadFile(p.SingBoxBin)
	if string(got) != "updated" {
		t.Fatalf("existing core was overwritten with %q", got)
	}
}

func TestSeedDirsFollowInstallMode(t *testing.T) {
	bin, rules := seedDirs(paths.FromRoot("/var/lib/sboxkit"))
	if bin != SeedBinDir || rules != SeedRulesetDir {
		t.Fatalf("system seed dirs = %q %q", bin, rules)
	}
	bin, rules = seedDirs(paths.FromUserModeRoot("/k/.sboxkit", "/k"))
	if bin != "/k/core" || rules != "/k/ruleset" {
		t.Fatalf("userMode seed dirs = %q %q", bin, rules)
	}
}
