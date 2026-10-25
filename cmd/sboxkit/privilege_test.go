package main

import (
	"testing"

	"github.com/Trilives/sboxkit/internal/flows"
	"github.com/Trilives/sboxkit/internal/paths"
)

func stubLaunchCheck(t *testing.T, needsRoot bool) {
	t.Helper()
	orig := launchCheck
	launchCheck = func(paths.Paths) (string, bool) { return "stub", needsRoot }
	t.Cleanup(func() { launchCheck = orig })
}

func TestElevationOnlyForSystemModeWriteCommands(t *testing.T) {
	stubLaunchCheck(t, false)
	system := paths.FromRoot("/var/lib/sboxkit")
	user := paths.FromUserModeRoot("/k/.sboxkit", "/k")
	cases := []struct {
		name   string
		p      paths.Paths
		cmd    string
		isRoot bool
		want   bool
	}{
		{"menu browses without password", system, "", false, false},
		{"update", system, "update", false, true},
		{"pause", system, "pause", false, true},
		{"nettest reads group-readable state", system, "nettest", false, false},
		{"already root", system, "update", true, false},
		{"version stays unprivileged", system, "version", false, false},
		{"diagnose reads /run only", system, "diagnose", false, false},
		{"healthcheck runs from root unit", system, "healthcheck", false, false},
		{"bootstrap relay", system, "bootstrap-dns", false, false},
		{"TUN DNS hook runs from root unit", system, "tun-dns", false, false},
		{"user mode never elevates", user, "", false, false},
		{"user mode update", user, "update", false, false},
	}
	for _, tc := range cases {
		if _, got := elevationReason(tc.p, tc.cmd, tc.isRoot); got != tc.want {
			t.Errorf("%s: elevationReason(%q, root=%v) = %v, want %v", tc.name, tc.cmd, tc.isRoot, got, tc.want)
		}
	}
}

func TestMenuElevatesAtLaunchOnlyWhenTheLaunchCheckSaysSo(t *testing.T) {
	stubLaunchCheck(t, true)
	system := paths.FromRoot("/var/lib/sboxkit")

	if reason, ok := elevationReason(system, "", false); !ok || reason != "stub" {
		t.Fatalf("elevationReason = (%q, %v), want the launch check's reason", reason, ok)
	}
	if _, ok := elevationReason(system, "", true); ok {
		t.Fatal("root never re-elevates")
	}
}

func TestWriteCommandsCoverEveryMutatingSubcommand(t *testing.T) {
	for _, cmd := range []string{"init", "modify", "uninstall", "update", "assets-update", "portable-update", "pause", "resume"} {
		if !writeCommands[cmd] {
			t.Errorf("%q writes state and must run as root in system mode", cmd)
		}
	}
}

func TestParseResumeAcceptsOnlyKnownEntries(t *testing.T) {
	args, err := parseResume([]string{flows.ResumeFlag + string(flows.EntryPinNode)})
	if err != nil || len(args) != 0 {
		t.Fatalf("parseResume(known) = %q, %v, want the interactive menu", args, err)
	}
	for _, bad := range [][]string{
		{flows.ResumeFlag + "runtime/../../etc"},
		{flows.ResumeFlag + string(flows.EntryConfig), "update"},
	} {
		if _, err := parseResume(bad); err == nil {
			t.Errorf("parseResume(%q) accepted", bad)
		}
	}
	if args, err := parseResume([]string{"update"}); err != nil || len(args) != 1 {
		t.Fatalf("parseResume(update) = %q, %v, want the command untouched", args, err)
	}
}
