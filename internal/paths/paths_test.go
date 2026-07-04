package paths

import (
	"path/filepath"
	"testing"
)

func TestFromRootBuildsExpectedLayout(t *testing.T) {
	root := t.TempDir()
	p := FromRoot(root)

	if p.Root != root {
		t.Fatalf("expected root %q, got %q", root, p.Root)
	}
	if p.StateDir != filepath.Join(root, "state") {
		t.Fatalf("unexpected state dir %q", p.StateDir)
	}
	if p.CustomizeFile != filepath.Join(root, "state", "customize.json") {
		t.Fatalf("unexpected customize path %q", p.CustomizeFile)
	}
	if p.LogDir != filepath.Join(root, "state", "logs") {
		t.Fatalf("unexpected log dir %q", p.LogDir)
	}
	if p.DownloadsDir != filepath.Join(root, "cache", "downloads") {
		t.Fatalf("unexpected downloads dir %q", p.DownloadsDir)
	}
	if p.ActivationsDir != filepath.Join(root, "revisions") {
		t.Fatalf("unexpected revisions dir %q", p.ActivationsDir)
	}
	if p.RuntimeLink != filepath.Join(root, "current") {
		t.Fatalf("unexpected current link %q", p.RuntimeLink)
	}
	if p.SingBoxCacheDB != filepath.Join(root, "sing-box", "cache.db") {
		t.Fatalf("unexpected sing-box cache %q", p.SingBoxCacheDB)
	}
	if p.AdminConfigFile != "/etc/sboxkit/config.json" {
		t.Fatalf("unexpected admin config path %q", p.AdminConfigFile)
	}
	if p.EtcDir != "/etc/sboxkit" {
		t.Fatalf("unexpected etc dir %q", p.EtcDir)
	}
	if p.SystemSingBoxBin != "/usr/lib/sboxkit/sing-box" {
		t.Fatalf("unexpected packaged sing-box path %q", p.SystemSingBoxBin)
	}
}

func TestEnsureStateDirsCreatesRuntimeDirectories(t *testing.T) {
	p := FromRoot(t.TempDir())

	if err := p.EnsureStateDirs(); err != nil {
		t.Fatalf("ensure state dirs: %v", err)
	}

	for _, dir := range []string{
		p.StateDir,
		p.BinDir,
		p.UIDir,
		p.RulesetDir,
		p.DownloadsDir,
		p.SubscriptionsDir,
		p.LogDir,
		p.ActivationsDir,
		p.SingBoxDir,
	} {
		if !isDir(dir) {
			t.Fatalf("expected directory %s", dir)
		}
	}
}

func TestDefaultRootUsesVarLibByDefault(t *testing.T) {
	t.Setenv("SBOXKIT_ROOT", "")
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	if got := DefaultRoot(); got != "/var/lib/sboxkit" {
		t.Fatalf("unexpected default root %q", got)
	}
	if got := FromRoot("").Root; got != "/var/lib/sboxkit" {
		t.Fatalf("unexpected FromRoot default %q", got)
	}
	p := FromRoot("")
	if p.DownloadsDir != "/var/cache/sboxkit/downloads" {
		t.Fatalf("unexpected default downloads dir %q", p.DownloadsDir)
	}
}

func TestDefaultRootCanBeOverridden(t *testing.T) {
	override := filepath.Join(t.TempDir(), "custom")
	t.Setenv("SBOXKIT_ROOT", override)

	if got := DefaultRoot(); got != override {
		t.Fatalf("unexpected default root override %q", got)
	}
}
