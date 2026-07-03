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
	} {
		if !isDir(dir) {
			t.Fatalf("expected directory %s", dir)
		}
	}
}

func TestDefaultRootUsesXDGStateHome(t *testing.T) {
	t.Setenv("SBOXKIT_ROOT", "")
	xdg := t.TempDir()
	t.Setenv("XDG_STATE_HOME", xdg)

	if got := DefaultRoot(); got != filepath.Join(xdg, "sboxkit") {
		t.Fatalf("unexpected default root %q", got)
	}
	if got := FromRoot("").Root; got != filepath.Join(xdg, "sboxkit") {
		t.Fatalf("unexpected FromRoot default %q", got)
	}
}

func TestDefaultRootCanBeOverridden(t *testing.T) {
	override := filepath.Join(t.TempDir(), "custom")
	t.Setenv("SBOXKIT_ROOT", override)

	if got := DefaultRoot(); got != override {
		t.Fatalf("unexpected default root override %q", got)
	}
}
