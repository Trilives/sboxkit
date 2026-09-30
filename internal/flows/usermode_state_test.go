package flows

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Trilives/sboxkit/internal/paths"
)

func userModePaths(t *testing.T) paths.Paths {
	t.Helper()
	bundle := t.TempDir()
	return paths.FromUserModeRoot(filepath.Join(bundle, ".sboxkit"), bundle)
}

func TestEnsureStateRootLocksUserModeStateToOwner(t *testing.T) {
	p := userModePaths(t)
	if err := os.MkdirAll(p.State, 0o755); err != nil { // 旧版本留下的 0755
		t.Fatal(err)
	}

	if err := EnsureStateRoot(p); err != nil {
		t.Fatal(err)
	}

	st, err := os.Stat(p.State)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != userModeStateMode {
		t.Fatalf("state mode = %v, want %v: subscriptions hold node credentials", st.Mode().Perm(), os.FileMode(userModeStateMode))
	}
	if leftovers, _ := filepath.Glob(filepath.Join(p.State, ".probe*")); len(leftovers) != 0 {
		t.Fatalf("writability probe left behind: %v", leftovers)
	}
}

func TestRejectForeignRootRefusesSudoIntoUsersBundle(t *testing.T) {
	p := userModePaths(t)
	owner, ok := ownerUID(p.BundleRoot)
	if !ok || owner == 0 {
		t.Skip("bundle is root-owned in this environment")
	}

	err := rejectForeignRoot(p, 0)

	if err == nil || !strings.Contains(err.Error(), "sudo") {
		t.Fatalf("root in a user's bundle must be refused, got %v", err)
	}
}

func TestRejectForeignRootAllowsOwnerAndSystemMode(t *testing.T) {
	p := userModePaths(t)
	owner, _ := ownerUID(p.BundleRoot)

	if err := rejectForeignRoot(p, int(owner)); err != nil {
		t.Fatalf("bundle owner refused: %v", err)
	}
	if err := rejectForeignRoot(paths.FromRoot(p.State), 0); err != nil {
		t.Fatalf("system mode run as root refused: %v", err)
	}
}
