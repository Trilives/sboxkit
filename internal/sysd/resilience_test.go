package sysd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInspectResilienceStatusDetectsMissingAndStaleUnits(t *testing.T) {
	root := t.TempDir()
	layout := resilienceLayout{
		dispatcherDir: filepath.Join(root, "dispatcher.d"),
		systemdDir:    filepath.Join(root, "systemd"),
	}
	if err := os.MkdirAll(layout.dispatcherDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(layout.systemdDir, 0o755); err != nil {
		t.Fatal(err)
	}

	st := inspectResilienceStatus(layout, DefaultName, "2min", "singbox", "/usr/bin/sboxkit", false, false)
	if !st.NeedsRepair {
		t.Fatalf("expected empty layout to need repair: %#v", st)
	}

	if err := os.WriteFile(dispatcherFileIn(layout, DefaultName), []byte(dispatcherText(DefaultName, "singbox", 20)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(wdServiceIn(layout), []byte(wdServiceText(DefaultName, "singbox", "/old/bin/sboxkit")), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(wdTimerIn(layout), []byte(wdTimerText("2min")), 0o644); err != nil {
		t.Fatal(err)
	}

	st = inspectResilienceStatus(layout, DefaultName, "2min", "singbox", "/usr/bin/sboxkit", true, false)
	if !st.WatchdogTimerEnabled {
		t.Fatalf("expected timer enabled: %#v", st)
	}
	if !st.NeedsRepair {
		t.Fatalf("expected stale executable path to need repair: %#v", st)
	}

	if err := os.WriteFile(wdServiceIn(layout), []byte(wdServiceText(DefaultName, "singbox", "/usr/bin/sboxkit")), 0o644); err != nil {
		t.Fatal(err)
	}
	st = inspectResilienceStatus(layout, DefaultName, "2min", "singbox", "/usr/bin/sboxkit", true, false)
	if st.NeedsRepair {
		t.Fatalf("complete matching install should be idempotent: %#v", st)
	}
}

func TestInspectResilienceStatusHonorsUserDisableMarker(t *testing.T) {
	root := t.TempDir()
	layout := resilienceLayout{
		dispatcherDir: filepath.Join(root, "dispatcher.d"),
		systemdDir:    filepath.Join(root, "systemd"),
	}
	st := inspectResilienceStatus(layout, DefaultName, "2min", "singbox", "/usr/bin/sboxkit", false, true)
	if !st.UserDisabled {
		t.Fatalf("expected user-disabled marker to be reported: %#v", st)
	}
	if st.NeedsRepair {
		t.Fatalf("explicit disable should suppress automatic repair: %#v", st)
	}
}

func TestInspectResilienceStatusRepairsInactiveTimer(t *testing.T) {
	root := t.TempDir()
	layout := resilienceLayout{
		dispatcherDir: filepath.Join(root, "dispatcher.d"),
		systemdDir:    filepath.Join(root, "systemd"),
	}
	if err := os.MkdirAll(layout.dispatcherDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(layout.systemdDir, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		dispatcherFileIn(layout, DefaultName): dispatcherText(DefaultName, "singbox", 20),
		wdServiceIn(layout):                   wdServiceText(DefaultName, "singbox", "/usr/bin/sboxkit"),
		wdTimerIn(layout):                     wdTimerText("2min"),
	}
	for path, content := range files {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	st := inspectResilienceStatus(layout, DefaultName, "2min", "singbox", "/usr/bin/sboxkit", true, false, false)
	if !st.NeedsRepair {
		t.Fatalf("inactive enabled timer should need repair: %#v", st)
	}
}

func TestStableExecutablePathPreservesInvocationSymlink(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "versions", "sboxkit-1.2.3")
	link := filepath.Join(root, "bin", "sboxkit")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	got, err := stableExecutablePath(link, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != link {
		t.Fatalf("executable path = %q, want stable invocation symlink %q", got, link)
	}
}

func TestStableExecutablePathUsesLookPathForBareCommand(t *testing.T) {
	want := filepath.Join(t.TempDir(), "sboxkit")
	got, err := stableExecutablePath("sboxkit", func(command string) (string, error) {
		if command != "sboxkit" {
			t.Fatalf("LookPath command = %q", command)
		}
		return want, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("executable path = %q, want %q", got, want)
	}
}
