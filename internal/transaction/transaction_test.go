package transaction

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRollbackRunsUndoInReverseOrder(t *testing.T) {
	var calls []string
	tx := New("test")
	tx.AddUndo("first", func() error {
		calls = append(calls, "first")
		return nil
	})
	tx.AddUndo("second", func() error {
		calls = append(calls, "second")
		return nil
	})

	if err := tx.Rollback(); err != nil {
		t.Fatalf("rollback: %v", err)
	}

	if len(calls) != 2 || calls[0] != "second" || calls[1] != "first" {
		t.Fatalf("unexpected rollback order: %#v", calls)
	}
}

func TestBackupFileRestoresPreviousContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte("before"), 0o644); err != nil {
		t.Fatalf("write before: %v", err)
	}

	tx := New("test")
	if err := tx.BackupFile(path); err != nil {
		t.Fatalf("backup: %v", err)
	}
	if err := os.WriteFile(path, []byte("after"), 0o600); err != nil {
		t.Fatalf("write after: %v", err)
	}

	if err := tx.Rollback(); err != nil {
		t.Fatalf("rollback: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read restored: %v", err)
	}
	if string(got) != "before" {
		t.Fatalf("expected restored content, got %q", got)
	}
}

func TestTrackPathRemovesNewDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "created")

	tx := New("test")
	tx.TrackPath(path)
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("create tracked path: %v", err)
	}

	if err := tx.Rollback(); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected tracked path removed, stat error: %v", err)
	}
}

func TestCommitPreventsRollback(t *testing.T) {
	called := false
	tx := New("test")
	tx.AddUndo("undo", func() error {
		called = true
		return nil
	})
	tx.Commit()

	if err := tx.Rollback(); err != nil {
		t.Fatalf("rollback after commit: %v", err)
	}
	if called {
		t.Fatal("did not expect undo after commit")
	}
}
