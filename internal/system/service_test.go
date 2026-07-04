package system

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Trilives/sboxkit/internal/paths"
	"github.com/Trilives/sboxkit/internal/testkit"
)

func TestRenderServiceUnit(t *testing.T) {
	got := RenderServiceUnit(paths.FromRoot("/opt/sboxkit"))
	want := testkit.ReadFixture(t, "testdata/system/sboxkit.service.golden")

	if strings.TrimSpace(got) != strings.TrimSpace(want) {
		t.Fatalf("unit mismatch\nwant:\n%s\n\ngot:\n%s", want, got)
	}
}

func TestServiceStatusUsesSystemctl(t *testing.T) {
	runner := &FakeRunner{}
	svc := NewService(paths.FromRoot(t.TempDir()), runner)

	if err := svc.Status(context.Background()); err != nil {
		t.Fatalf("status: %v", err)
	}
	if len(runner.Commands) != 1 {
		t.Fatalf("expected one command, got %#v", runner.Commands)
	}
	if got := strings.Join(runner.Commands[0], " "); got != "systemctl status --no-pager sboxkit.service" {
		t.Fatalf("unexpected command %q", got)
	}
}

func TestServiceStartStopUseSystemctl(t *testing.T) {
	runner := &FakeRunner{}
	svc := NewService(paths.FromRoot(t.TempDir()), runner)

	if err := svc.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := svc.Stop(context.Background()); err != nil {
		t.Fatalf("stop: %v", err)
	}

	got := runner.JoinedCommands()
	for _, want := range []string{
		"systemctl restart sboxkit.service",
		"systemctl stop sboxkit.service",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected command %q in:\n%s", want, got)
		}
	}
}

func TestSyncAndRestartCreatesConfigRevision(t *testing.T) {
	p := paths.FromRoot(t.TempDir())
	if err := p.EnsureStateDirs(); err != nil {
		t.Fatalf("create dirs: %v", err)
	}
	for path, data := range map[string]string{
		p.SingBoxBin: "core",
		p.ConfigFile: `{
		  "inbounds": [],
		  "outbounds": [],
		  "route": {
		    "rule_set": [
		      {"type":"local","tag":"geosite-cn","path":"` + p.GeositeCN + `"}
		    ]
		  }
		}`,
		p.GeositeCN: "geosite",
		p.GeoIPCN:   "geoip",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
		if err := os.WriteFile(path, []byte(data), 0o755); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	runner := &FakeRunner{}
	svc := NewService(p, runner)
	if err := svc.SyncAndRestart(context.Background()); err != nil {
		t.Fatalf("sync: %v", err)
	}

	joined := runner.JoinedCommands()
	for _, want := range []string{
		"mkdir -p " + filepath.Join(p.ActivationsDir),
		"ln -sfn",
		"systemctl restart sboxkit.service",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected command %q in:\n%s", want, joined)
		}
	}
	for _, notWant := range []string{
		"install -m 0755 " + p.SingBoxBin,
		"install -m 0644 " + p.GeositeCN,
		"install -m 0644 " + p.GeoIPCN,
		"/ruleset/",
		"/ui",
	} {
		if strings.Contains(joined, notWant) {
			t.Fatalf("revision should not copy runtime asset %q in:\n%s", notWant, joined)
		}
	}
}

func TestPruneActivationsKeepsCurrentAndPrevious(t *testing.T) {
	p := paths.FromRoot(t.TempDir())
	for _, name := range []string{"20260704T010000.000000000Z", "20260704T020000.000000000Z", "20260704T030000.000000000Z"} {
		if err := os.MkdirAll(filepath.Join(p.ActivationsDir, name), 0o755); err != nil {
			t.Fatalf("mkdir activation: %v", err)
		}
	}
	runner := &FakeRunner{}
	svc := NewService(p, runner)
	current := filepath.Join(p.ActivationsDir, "20260704T030000.000000000Z")
	previous := filepath.Join(p.ActivationsDir, "20260704T020000.000000000Z")

	if err := svc.pruneActivations(context.Background(), current, previous); err != nil {
		t.Fatalf("prune activations: %v", err)
	}

	joined := runner.JoinedCommands()
	if !strings.Contains(joined, "rm -rf "+filepath.Join(p.ActivationsDir, "20260704T010000.000000000Z")) {
		t.Fatalf("expected oldest activation prune in:\n%s", joined)
	}
	if strings.Contains(joined, "20260704T020000.000000000Z") || strings.Contains(joined, "20260704T030000.000000000Z") {
		t.Fatalf("current/previous activation should be kept:\n%s", joined)
	}
}

type FakeRunner struct {
	Commands [][]string
}

func (r *FakeRunner) Run(ctx context.Context, name string, args ...string) error {
	r.Commands = append(r.Commands, append([]string{name}, args...))
	return nil
}

func (r *FakeRunner) JoinedCommands() string {
	lines := make([]string, 0, len(r.Commands))
	for _, command := range r.Commands {
		lines = append(lines, strings.Join(command, " "))
	}
	return strings.Join(lines, "\n")
}
