package txn

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Trilives/sboxkit/internal/errs"
)

func TestBackupFileRestoresContent(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(f, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	tx := New("测试")
	if err := tx.BackupFile(f); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(f, []byte("new"), 0o644)
	tx.Rollback()

	got, err := os.ReadFile(f)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "old" {
		t.Fatalf("回退后内容 = %q, 期望 old", got)
	}
}

func TestBackupFileRemovesCreated(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "new.txt")

	tx := New("测试")
	if err := tx.BackupFile(f); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(f, []byte("x"), 0o644)
	tx.Rollback()

	if _, err := os.Stat(f); !os.IsNotExist(err) {
		t.Fatal("回退后新建文件应被删除")
	}
}

func TestSnapshotDirRestores(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "subscriptions")
	if err := os.MkdirAll(filepath.Join(sub, "a"), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(sub, "a", "meta.json"), []byte(`{"name":"a"}`), 0o644)

	tx := New("测试")
	if err := tx.Snapshot(sub); err != nil {
		t.Fatal(err)
	}
	os.RemoveAll(filepath.Join(sub, "a"))
	os.WriteFile(filepath.Join(sub, "junk.txt"), []byte("x"), 0o644)
	tx.Rollback()

	if _, err := os.Stat(filepath.Join(sub, "a", "meta.json")); err != nil {
		t.Fatal("目录快照未还原:", err)
	}
	if _, err := os.Stat(filepath.Join(sub, "junk.txt")); !os.IsNotExist(err) {
		t.Fatal("回退后会话内新增文件应消失")
	}
}

func TestSnapshotMissingPathDeletedOnRollback(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "later.json")

	tx := New("测试")
	if err := tx.Snapshot(f); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(f, []byte("x"), 0o644)
	tx.Rollback()

	if _, err := os.Stat(f); !os.IsNotExist(err) {
		t.Fatal("原本不存在的路径回退后应被删除")
	}
}

func TestCommitKeepsChanges(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "a.txt")
	os.WriteFile(f, []byte("old"), 0o644)

	tx := New("测试")
	if err := tx.BackupFile(f); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(f, []byte("new"), 0o644)
	tx.Commit()
	tx.Rollback() // commit 后回退应为空操作

	got, _ := os.ReadFile(f)
	if string(got) != "new" {
		t.Fatalf("commit 后内容 = %q, 期望 new", got)
	}
}

func TestTrackPathRemovesOnlyNew(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "keep")
	os.MkdirAll(existing, 0o755)
	fresh := filepath.Join(dir, "fresh")

	tx := New("测试")
	tx.TrackPath(existing)
	tx.TrackPath(fresh)
	os.MkdirAll(fresh, 0o755)
	tx.Rollback()

	if _, err := os.Stat(existing); err != nil {
		t.Fatal("已存在路径不应被回退删除")
	}
	if _, err := os.Stat(fresh); !os.IsNotExist(err) {
		t.Fatal("新建路径回退后应被删除")
	}
}

func TestRunSwallowsCancelledAndRollsBack(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "a.txt")
	os.WriteFile(f, []byte("old"), 0o644)

	err := Run("测试", func(tx *Transaction) error {
		if err := tx.BackupFile(f); err != nil {
			return err
		}
		os.WriteFile(f, []byte("new"), 0o644)
		return errs.ErrCancelled
	})
	if err != nil {
		t.Fatal("ErrCancelled 应被吞掉:", err)
	}
	got, _ := os.ReadFile(f)
	if string(got) != "old" {
		t.Fatalf("取消后应回退, 内容 = %q", got)
	}

	// ErrSaveExit 包装了 ErrCancelled，若逃逸到顶层同样按取消处理
	if err := Run("测试", func(tx *Transaction) error { return errs.ErrSaveExit }); err != nil {
		t.Fatal("ErrSaveExit 应被吞掉:", err)
	}
}

func TestRunPropagatesOtherErrorsAfterRollback(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "a.txt")
	os.WriteFile(f, []byte("old"), 0o644)
	sentinel := errors.New("boom")

	err := Run("测试", func(tx *Transaction) error {
		if err := tx.BackupFile(f); err != nil {
			return err
		}
		os.WriteFile(f, []byte("new"), 0o644)
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("其他错误应原样返回, got %v", err)
	}
	got, _ := os.ReadFile(f)
	if string(got) != "old" {
		t.Fatalf("出错后应回退, 内容 = %q", got)
	}
}
