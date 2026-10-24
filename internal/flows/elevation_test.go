package flows

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Trilives/sboxkit/internal/errs"
	"github.com/Trilives/sboxkit/internal/execx"
	"github.com/Trilives/sboxkit/internal/paths"
	"github.com/Trilives/sboxkit/internal/tui"
)

func TestResumeWalksEachMenuLevelThenStops(t *testing.T) {
	Resume(EntryServiceRestart)
	t.Cleanup(func() { resumePath = nil })

	for _, step := range []struct {
		keys []string
		want int
	}{
		{MainMenuKeys, 0},    // 运行时管理
		{runtimeMenuKeys, 2}, // 服务设置
		{serviceMenuKeys, 1}, // 重启服务
	} {
		got, ok := takeResume(step.keys)
		if !ok || got != step.want {
			t.Fatalf("takeResume(%v) = %d, %v, want %d", step.keys, got, ok, step.want)
		}
	}
	if _, ok := takeResume(MainMenuKeys); ok {
		t.Fatal("after the target the menus must be interactive again")
	}
}

func TestResumeGivesUpWhenAMenuDoesNotMatch(t *testing.T) {
	Resume(EntryUpdate)
	t.Cleanup(func() { resumePath = nil })

	if _, ok := takeResume(runtimeMenuKeys); ok {
		t.Fatal("tools/update must not select anything in the runtime menu")
	}
	if len(resumePath) != 0 {
		t.Fatalf("resume path = %v, want it dropped so later menus stay interactive", resumePath)
	}
}

func TestMenuSelectReturnsTheResumedOptionWithoutPrompting(t *testing.T) {
	Resume(EntryUninstall)
	t.Cleanup(func() { resumePath = nil })

	got, err := MenuSelect("main", make([]string, len(MainMenuKeys)), MainMenuKeys, tui.SelectOpts{})

	if err != nil || got != 5 {
		t.Fatalf("MenuSelect = %d, %v, want the uninstall entry", got, err)
	}
}

func TestEveryEntryResolvesThroughItsMenus(t *testing.T) {
	menus := map[string][]string{"": MainMenuKeys, "runtime": runtimeMenuKeys, "runtime/service": serviceMenuKeys, "tools": toolMenuKeys}
	for _, e := range knownEntries {
		segments := strings.Split(string(e), "/")
		for i, seg := range segments {
			keys := menus[strings.Join(segments[:i], "/")]
			if !contains(keys, seg) {
				t.Errorf("entry %q: %q is not an option of menu %q", e, seg, strings.Join(segments[:i], "/"))
			}
		}
		if parsed, ok := ParseEntry(string(e)); !ok || parsed != e {
			t.Errorf("ParseEntry(%q) = %q, %v", e, parsed, ok)
		}
	}
	if _, ok := ParseEntry("runtime"); ok {
		t.Error("a bare submenu is not a resumable action")
	}
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func stubElevate(t *testing.T, result error) *[]Entry {
	t.Helper()
	var got []Entry
	orig := elevateTo
	elevateTo = func(e Entry) error { got = append(got, e); return result }
	t.Cleanup(func() { elevateTo = orig })
	return &got
}

func TestRequireRootElevatesOnlyUnprivilegedSystemMode(t *testing.T) {
	if execx.IsRoot() {
		t.Skip("running as root")
	}
	calls := stubElevate(t, errs.ErrCancelled)

	if err := RequireRoot(userModePaths(t), EntryPinNode); err != nil {
		t.Fatalf("user mode RequireRoot = %v, want nil: user mode never needs root", err)
	}
	err := RequireRoot(paths.FromRoot(t.TempDir()), EntryPinNode)

	if !errors.Is(err, errs.ErrCancelled) {
		t.Fatalf("system mode RequireRoot = %v, want the elevation result", err)
	}
	if len(*calls) != 1 || (*calls)[0] != EntryPinNode {
		t.Fatalf("elevated to %v, want exactly the pin entry", *calls)
	}
}

func fakeProbe(uid uint32, readable, installed, reconcile, repair bool) launchProbe {
	return launchProbe{
		stateOwner: func(string) (uint32, bool, error) { return uid, true, nil },
		readable:   func(paths.Paths) bool { return readable },
		installed:  func() bool { return installed },
		reconcile:  func(paths.Paths) bool { return reconcile },
		repair:     func(paths.Paths) (bool, error) { return repair, nil },
	}
}

func TestLaunchNeedsRootOnlyWhenStartupMustWrite(t *testing.T) {
	p := paths.FromRoot("/var/lib/sboxkit")
	missing := fakeProbe(0, true, true, false, false)
	missing.stateOwner = func(string) (uint32, bool, error) { return 0, false, os.ErrNotExist }
	cases := []struct {
		name  string
		probe launchProbe
		want  bool
	}{
		{"ready to browse", fakeProbe(0, true, true, false, false), false},
		{"first run", missing, true},
		{"legacy user-owned state", fakeProbe(1000, true, true, false, false), true},
		{"not in admin group", fakeProbe(0, false, true, false, false), true},
		{"service not installed", fakeProbe(0, true, false, false, false), true},
		{"version changed", fakeProbe(0, true, true, true, false), true},
		{"watchdog needs repair", fakeProbe(0, true, true, false, true), true},
	}
	for _, tc := range cases {
		reason, got := tc.probe.check(p)
		if got != tc.want {
			t.Errorf("%s: needs root = %v (%q), want %v", tc.name, got, reason, tc.want)
		}
		if got && reason == "" {
			t.Errorf("%s: elevating without telling the user why", tc.name)
		}
	}
}

func TestStateReadableDetectsUnreadableSecrets(t *testing.T) {
	if execx.IsRoot() {
		t.Skip("root reads everything")
	}
	p := paths.FromRoot(t.TempDir())
	if !stateReadable(p) {
		t.Fatal("missing files are not a read failure")
	}
	if err := os.WriteFile(p.ConfigFile, []byte("{}"), 0o640); err != nil {
		t.Fatal(err)
	}
	if !stateReadable(p) {
		t.Fatal("readable config reported unreadable")
	}
	if err := os.Chmod(p.ConfigFile, 0); err != nil {
		t.Fatal(err)
	}
	if stateReadable(p) {
		t.Fatal("unreadable config must force elevation at launch")
	}
}

func TestStateOwnerReportsMissingState(t *testing.T) {
	if _, _, err := stateOwner(filepath.Join(t.TempDir(), "absent")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stateOwner(absent) err = %v, want ErrNotExist", err)
	}
}
