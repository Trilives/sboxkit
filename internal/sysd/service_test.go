package sysd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Trilives/sboxkit/internal/execx"
	"github.com/Trilives/sboxkit/internal/paths"
)

func TestStageRuntimeConfigRewritesExternalUIAndCachePath(t *testing.T) {
	state := t.TempDir()
	p := paths.Paths{
		State:      state,
		ConfigFile: filepath.Join(state, "config.json"),
		UI:         state, // hasUIAssets 检查 p.UI/index.html
	}
	if err := os.WriteFile(filepath.Join(state, "index.html"), []byte("<html></html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	raw := `{"inbounds":[],"outbounds":[],"experimental":{"clash_api":{"external_controller":"127.0.0.1:9090","external_ui":"ui"}}}`
	if err := os.WriteFile(p.ConfigFile, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}

	staged, err := stageRuntimeConfig(p, runtimePaths{
		UI:      "/var/lib/sboxkit-runtime/ui",
		CacheDB: "/var/lib/sboxkit-runtime/cache.db",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(staged)

	out, err := os.ReadFile(staged)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"/var/lib/sboxkit-runtime/ui"`) {
		t.Fatalf("external_ui was not rewritten in staged config:\n%s", out)
	}
	if !strings.Contains(string(out), `"/var/lib/sboxkit-runtime/cache.db"`) {
		t.Fatalf("cache_file.path was not rewritten in staged config:\n%s", out)
	}
}

func TestStageRuntimeConfigUsesEmbeddedUIWhenStateUIIsMissing(t *testing.T) {
	state := t.TempDir()
	p := paths.Paths{
		State:      state,
		ConfigFile: filepath.Join(state, "config.json"),
		UI:         filepath.Join(state, "ui"),
	}
	raw := `{"inbounds":[],"outbounds":[],"experimental":{"clash_api":{"external_controller":"127.0.0.1:9090","external_ui":"ui"}}}`
	if err := os.WriteFile(p.ConfigFile, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}

	staged, err := stageRuntimeConfig(p, runtimePaths{
		UI:      "/var/lib/sboxkit-runtime/ui",
		CacheDB: "/var/lib/sboxkit-runtime/cache.db",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(staged)

	out, err := os.ReadFile(staged)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"/var/lib/sboxkit-runtime/ui"`) {
		t.Fatalf("embedded UI path was not staged when state UI was missing:\n%s", out)
	}
}

func TestSyncUIRuntimeStagesEmbeddedAssetsWithoutTouchingStateUI(t *testing.T) {
	rt := runtimePaths{UI: filepath.Join(t.TempDir(), "runtime-ui")}
	var commands [][]string
	stagedHasIndex := false
	runRoot := func(cmd []string, _ string, _ *execx.Opt) (execx.Result, error) {
		commands = append(commands, append([]string(nil), cmd...))
		if len(cmd) == 4 && cmd[0] == "cp" {
			_, err := os.Stat(filepath.Join(cmd[2], "index.html"))
			stagedHasIndex = err == nil
		}
		return execx.Result{}, nil
	}

	if err := syncUIRuntimeWith(rt, runRoot); err != nil {
		t.Fatal(err)
	}
	if !stagedHasIndex {
		t.Fatal("embedded UI was not materialized before the privileged copy")
	}
	if len(commands) != 2 {
		t.Fatalf("root commands = %v, want remove and copy of runtime UI only", commands)
	}
	if got := strings.Join(commands[0], " "); got != "rm -rf "+rt.UI {
		t.Fatalf("first root command = %q", got)
	}
	if commands[1][0] != "cp" || commands[1][1] != "-a" || commands[1][3] != rt.UI {
		t.Fatalf("copy command = %v", commands[1])
	}
	for _, cmd := range commands {
		if strings.Contains(strings.Join(cmd, " "), "/var/lib/sboxkit/ui") {
			t.Fatalf("runtime refresh must not mutate shared state UI: %v", cmd)
		}
	}
}

func TestSyncUIRuntimeReturnsPrivilegedReplacementErrors(t *testing.T) {
	wantErr := errors.New("root operation failed")
	for _, failAt := range []int{1, 2} {
		t.Run(fmt.Sprintf("command_%d", failAt), func(t *testing.T) {
			calls := 0
			runRoot := func(_ []string, _ string, _ *execx.Opt) (execx.Result, error) {
				calls++
				if calls == failAt {
					return execx.Result{}, wantErr
				}
				return execx.Result{}, nil
			}
			err := syncUIRuntimeWith(runtimePaths{UI: filepath.Join(t.TempDir(), "runtime-ui")}, runRoot)
			if !errors.Is(err, wantErr) {
				t.Fatalf("error = %v, want %v", err, wantErr)
			}
			if calls != failAt {
				t.Fatalf("root calls = %d, want stop at %d", calls, failAt)
			}
		})
	}
}

func TestSyncUIRuntimeReturnsTemporaryDirectoryError(t *testing.T) {
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "missing"))
	err := syncUIRuntimeWith(runtimePaths{UI: "/runtime/ui"}, nil)
	if err == nil || !strings.Contains(err.Error(), "stage embedded UI") {
		t.Fatalf("error = %v, want temporary staging failure", err)
	}
}

func TestResumeCompanionUnitsExcludesResidualWatchdog(t *testing.T) {
	got := resumeCompanionUnits([]string{WatchdogName + ".timer", TimerName + ".timer"})
	if len(got) != 1 || got[0] != TimerName+".timer" {
		t.Fatalf("resume companions = %v, want only weekly update timer", got)
	}
}
