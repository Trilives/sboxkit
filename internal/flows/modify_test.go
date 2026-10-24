package flows

import (
	"os"
	"testing"

	"github.com/Trilives/sboxkit/internal/paths"
	"github.com/Trilives/sboxkit/internal/txn"
)

// ^R 回退时服务须按还原后的配置重新同步，而不是按回退前的新配置。
func TestSessionRollbackResyncsServiceAfterRestoringState(t *testing.T) {
	p := paths.FromRoot(t.TempDir())
	if err := p.EnsureStateDirs(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.ConfigFile, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	var seen string
	orig := resyncAfterRollback
	resyncAfterRollback = func(paths.Paths) {
		raw, _ := os.ReadFile(p.ConfigFile)
		seen = string(raw)
	}
	t.Cleanup(func() { resyncAfterRollback = orig })
	session := txn.New("test")

	if err := guardSession(session, p); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.ConfigFile, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	session.Rollback()

	if seen != "old" {
		t.Fatalf("service resynced with %q, want the restored config", seen)
	}
}
