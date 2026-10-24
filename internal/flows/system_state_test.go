package flows

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Trilives/sboxkit/internal/execx"
	"github.com/Trilives/sboxkit/internal/paths"
)

func TestEnsureSystemStateRefusesNonRoot(t *testing.T) {
	p := paths.FromRoot(filepath.Join(t.TempDir(), "state"))

	err := ensureSystemState(p, 1000)

	if err == nil || !strings.Contains(err.Error(), "sudo") {
		t.Fatalf("ensureSystemState as non-root = %v, want an error asking for sudo", err)
	}
	if _, statErr := os.Stat(p.State); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("state dir must not be created for a non-root caller, stat err = %v", statErr)
	}
}

func stubAdminGroup(t *testing.T, gid int, name string) {
	t.Helper()
	orig := adminGroup
	adminGroup = func() (int, string) { return gid, name }
	t.Cleanup(func() { adminGroup = orig })
}

func TestEnsureSystemStateSecuresWholeTreeForAdminGroup(t *testing.T) {
	p := paths.FromRoot(filepath.Join(t.TempDir(), "state"))
	stubAdminGroup(t, 27, "sudo")
	var secured []string
	var groups []int
	orig := secureTree
	secureTree = func(root string, gid int) error {
		secured, groups = append(secured, root), append(groups, gid)
		return nil
	}
	t.Cleanup(func() { secureTree = orig })

	if err := ensureSystemState(p, 0); err != nil {
		t.Fatal(err)
	}

	if len(secured) != 1 || secured[0] != p.State || groups[0] != 27 {
		t.Fatalf("secured %v for groups %v, want the whole state tree %q for the admin group", secured, groups, p.State)
	}
}

func TestSecureSystemStateFailureIsReported(t *testing.T) {
	p := paths.FromRoot(t.TempDir())
	orig := secureTree
	secureTree = func(string, int) error { return os.ErrPermission }
	t.Cleanup(func() { secureTree = orig })

	if err := secureSystemState(p); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("secureSystemState = %v, want the tree error so root skips trusting the state", err)
	}
}

func TestSecureSystemStateIsNoOpForNonRootAndUserMode(t *testing.T) {
	if execx.IsRoot() {
		t.Skip("running as root")
	}
	orig := secureTree
	secureTree = func(string, int) error { t.Fatal("must not touch the tree"); return nil }
	t.Cleanup(func() { secureTree = orig })

	if err := SecureSystemState(paths.FromRoot(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	if err := SecureSystemState(userModePaths(t)); err != nil {
		t.Fatal(err)
	}
}

func TestTightenCredentialsLocksSecretsAndSkipsLinks(t *testing.T) {
	p := paths.FromRoot(t.TempDir())
	if err := os.MkdirAll(p.Subscriptions, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{p.ConfigFile, execx.LogPath(p.State)} {
		if err := os.WriteFile(f, []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	elsewhere := filepath.Join(t.TempDir(), "elsewhere")
	if err := os.WriteFile(elsewhere, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(elsewhere, p.CustomizeFile); err != nil {
		t.Fatal(err)
	}

	if err := tightenCredentials(p); err != nil {
		t.Fatal(err)
	}

	for path, want := range map[string]os.FileMode{
		p.Subscriptions:        0o750,
		p.ConfigFile:           0o640,
		execx.LogPath(p.State): 0o640,
		elsewhere:              0o644,
	} {
		st, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if st.Mode().Perm() != want {
			t.Errorf("%s mode = %v, want %v", filepath.Base(path), st.Mode().Perm(), want)
		}
	}
}

func TestTightenCredentialsKeepsDirectorySetgid(t *testing.T) {
	p := paths.FromRoot(t.TempDir())
	if err := os.MkdirAll(p.Subscriptions, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(p.Subscriptions, 0o755|os.ModeSetgid); err != nil {
		t.Fatal(err)
	}

	if err := tightenCredentials(p); err != nil {
		t.Fatal(err)
	}

	st, err := os.Stat(p.Subscriptions)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode()&os.ModeSetgid == 0 || st.Mode().Perm() != 0o750 {
		t.Fatalf("subscriptions mode = %v, want 0750 with setgid so new files keep the admin group", st.Mode())
	}
}

func TestTightenCredentialsUsesOwnerOnlyModesInUserMode(t *testing.T) {
	p := userModePaths(t)
	if err := os.MkdirAll(filepath.Dir(p.ConfigFile), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.ConfigFile, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := tightenCredentials(p); err != nil {
		t.Fatal(err)
	}

	if st, _ := os.Stat(p.ConfigFile); st.Mode().Perm() != 0o600 {
		t.Fatalf("user-mode config mode = %v, want 0600", st.Mode().Perm())
	}
}

func TestSystemModeRefusesToTakeOverAUserOwnedCustomHome(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("needs a directory owned by an unprivileged user")
	}
	custom := paths.FromRoot(t.TempDir()) // 例如 SBOXKIT_HOME=$HOME

	if err := rejectForeignSystemRoot(custom, 0); err == nil {
		t.Fatal("root would chown a user-owned SBOXKIT_HOME and rewrite everything in it")
	}
	if err := rejectForeignSystemRoot(custom, os.Geteuid()); err != nil {
		t.Fatalf("unprivileged runs never secure the tree: %v", err)
	}
	if err := rejectForeignSystemRoot(paths.FromRoot(filepath.Join(t.TempDir(), "missing")), 0); err != nil {
		t.Fatalf("a directory root creates itself is fine: %v", err)
	}
	if err := rejectForeignSystemRoot(paths.FromRoot(paths.DefaultStateRoot), 0); err != nil {
		t.Fatalf("the default state directory must stay migratable: %v", err)
	}
}
