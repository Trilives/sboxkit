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
}
