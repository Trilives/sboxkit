package sysd

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/Trilives/sboxkit/internal/paths"
)

// fakeLoginctl 记录 loginctl 调用；linger 为 show-user 的返回值。
type fakeLoginctl struct {
	linger  string
	failOn  string
	command []string
}

func stubLoginctl(t *testing.T, f *fakeLoginctl) {
	t.Helper()
	origHave, origRun := haveLoginctl, runLoginctl
	haveLoginctl = func() bool { return true }
	runLoginctl = func(args ...string) (string, error) {
		f.command = append(f.command, strings.Join(args, " "))
		if args[0] == f.failOn {
			return "", errors.New("polkit denied")
		}
		if args[0] == "show-user" {
			return f.linger + "\n", nil
		}
		return "", nil
	}
	t.Cleanup(func() { haveLoginctl, runLoginctl = origHave, origRun })
}

func lingerScope(t *testing.T) unitScope {
	t.Helper()
	s := userModeScope(t)
	if err := os.MkdirAll(s.State, 0o700); err != nil {
		t.Fatal(err)
	}
	return s
}

func (f *fakeLoginctl) ran(verb string) bool {
	for _, cmd := range f.command {
		if strings.HasPrefix(cmd, verb) {
			return true
		}
	}
	return false
}

func TestEnsureLingerRecordsWhenSboxkitEnablesIt(t *testing.T) {
	s := lingerScope(t)
	fake := &fakeLoginctl{linger: "no"}
	stubLoginctl(t, fake)

	s.ensureLinger()

	if !fake.ran("enable-linger " + currentUserName()) {
		t.Fatalf("linger was not enabled for the current user explicitly: %v", fake.command)
	}
	if !fileExists(s.lingerMarker()) {
		t.Fatal("sboxkit enabled linger but left no marker; uninstall could not revert it")
	}
}

func TestEnsureLingerLeavesPreexistingLingerUnmarked(t *testing.T) {
	s := lingerScope(t)
	fake := &fakeLoginctl{linger: "yes"}
	stubLoginctl(t, fake)

	s.ensureLinger()

	if fake.ran("enable-linger") || fileExists(s.lingerMarker()) {
		t.Fatalf("user-owned linger must not be claimed: commands=%v", fake.command)
	}
}

func TestEnsureLingerFailureLeavesNoMarker(t *testing.T) {
	s := lingerScope(t)
	stubLoginctl(t, &fakeLoginctl{linger: "no", failOn: "enable-linger"})

	s.ensureLinger()

	if fileExists(s.lingerMarker()) {
		t.Fatal("marker written although enable-linger failed")
	}
}

func TestRevertLingerDisablesOnlyWhatSboxkitEnabled(t *testing.T) {
	s := lingerScope(t)
	fake := &fakeLoginctl{linger: "no"}
	stubLoginctl(t, fake)

	if err := s.revertLinger(); err != nil || fake.ran("disable-linger") {
		t.Fatalf("revert without marker: err=%v commands=%v", err, fake.command)
	}

	s.ensureLinger()
	if err := s.revertLinger(); err != nil {
		t.Fatal(err)
	}
	if !fake.ran("disable-linger " + currentUserName()) {
		t.Fatalf("linger enabled by sboxkit was not reverted: %v", fake.command)
	}
	if fileExists(s.lingerMarker()) {
		t.Fatal("marker kept after revert")
	}
}

func TestRevertLingerKeepsMarkerWhenDisableFails(t *testing.T) {
	s := lingerScope(t)
	stubLoginctl(t, &fakeLoginctl{linger: "no", failOn: "disable-linger"})
	s.ensureLinger()

	if err := s.revertLinger(); err == nil {
		t.Fatal("disable-linger failure swallowed")
	}
	if !fileExists(s.lingerMarker()) {
		t.Fatal("marker dropped although linger is still on; a retry could not revert it")
	}
}

func TestRevertLingerIsNoopInSystemMode(t *testing.T) {
	fake := &fakeLoginctl{}
	stubLoginctl(t, fake)

	if err := RevertLinger(paths.FromRoot(t.TempDir())); err != nil || len(fake.command) != 0 {
		t.Fatalf("system mode touched linger: err=%v commands=%v", err, fake.command)
	}
}
