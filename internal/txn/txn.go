// Package txn 事务 / 回退引擎（对应 Python 版 tx.py）：
// 让整个配置流程可受控中止并回退已应用的改动。
//
// 用法：
//
//	err := txn.Run("初始化", func(t *txn.Transaction) error {
//	    if err := t.BackupFile(cfgPath); err != nil { return err } // 改文件前先登记快照
//	    ...
//	    t.AddUndo("卸载服务", func() error { return service.Remove(name) })
//	    return nil // 正常走完 → commit
//	})
//	// 返回 ErrCancelled → 自动按 LIFO 回退并吞掉；其他错误 → 回退后原样返回
package txn

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Trilives/sboxkit/internal/errs"
	"github.com/Trilives/sboxkit/internal/execx"
	"github.com/Trilives/sboxkit/internal/i18n"
)

type undo struct {
	desc string
	fn   func() error
}

type Transaction struct {
	name     string
	undos    []undo
	cleanups []func()
}

func New(name string) *Transaction { return &Transaction{name: name} }

// Run 执行 fn 并按结果提交/回退（对应 Python 的 with Transaction(...)）。
func Run(name string, fn func(*Transaction) error) error {
	t := New(name)
	defer func() {
		if r := recover(); r != nil {
			t.Rollback()
			panic(r)
		}
	}()
	err := fn(t)
	if err == nil {
		t.Commit()
		return nil
	}
	if errors.Is(err, errs.ErrCancelled) {
		execx.Warn(fmt.Sprintf(i18n.T("已取消「%s」。"), name))
		t.Rollback()
		return nil
	}
	execx.Error(fmt.Sprintf(i18n.T("「%s」出错：%v"), name, err))
	t.Rollback()
	return err
}

// AddUndo 登记任意自定义回退动作（如卸载服务、还原 active 指针）。
func (t *Transaction) AddUndo(desc string, fn func() error) {
	t.undos = append(t.undos, undo{desc, fn})
}

// BackupFile 在修改/创建 path 前调用，登记回退到当前状态
// （记录改动前内容，或「原本不存在」→ 回退时删除）。
func (t *Transaction) BackupFile(path string) error {
	st, err := os.Stat(path)
	if err == nil {
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		mode := st.Mode()
		t.AddUndo(i18n.T("还原文件 ")+path, func() error {
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return err
			}
			return os.WriteFile(path, data, mode)
		})
		return nil
	}
	if os.IsNotExist(err) {
		t.AddUndo(i18n.T("删除新建文件 ")+path, func() error {
			rmErr := os.Remove(path)
			if rmErr != nil && os.IsNotExist(rmErr) {
				return nil
			}
			return rmErr
		})
		return nil
	}
	return err
}

// Snapshot 快照文件或目录（含「原本不存在」），回退时整体还原。
// 适合一次性保护一批配置路径（config.yaml / active / customize.json /
// subscriptions/）；仅快照配置类小文件，勿用于内核/UI 等大产物。
func (t *Transaction) Snapshot(path string) error {
	tmpRoot, err := os.MkdirTemp("", "sbtx-")
	if err != nil {
		return err
	}
	t.cleanups = append(t.cleanups, func() { os.RemoveAll(tmpRoot) })

	st, serr := os.Lstat(path)
	existed := serr == nil
	isDir := existed && st.IsDir()
	dest := filepath.Join(tmpRoot, filepath.Base(path))
	if existed {
		if isDir {
			err = copyTree(path, dest)
		} else {
			err = copyFile(path, dest)
		}
		if err != nil {
			return err
		}
	}

	t.AddUndo(i18n.T("还原 ")+path, func() error {
		if cur, err := os.Lstat(path); err == nil {
			if cur.IsDir() {
				os.RemoveAll(path)
			} else {
				os.Remove(path)
			}
		}
		if !existed {
			return nil
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if isDir {
			return copyTree(dest, path)
		}
		return copyFile(dest, path)
	})
	return nil
}

// TrackPath 登记一个将被创建的文件/目录；回退时若原本不存在则删除。
func (t *Transaction) TrackPath(path string) {
	if _, err := os.Lstat(path); err == nil {
		return // 已存在则不归我们删除，避免误删
	}
	t.AddUndo(i18n.T("删除新建路径 ")+path, func() error {
		st, err := os.Lstat(path)
		if err != nil {
			return nil
		}
		if st.IsDir() {
			return os.RemoveAll(path)
		}
		return os.Remove(path)
	})
}

// Commit 提交：清空回退栈并运行清理（删除快照临时目录）。
func (t *Transaction) Commit() {
	t.undos = nil
	t.runCleanups()
}

// Rollback 按登记逆序回退；单个 undo 失败不阻断其余，最后汇总报告。
func (t *Transaction) Rollback() {
	if len(t.undos) == 0 {
		t.runCleanups()
		return
	}
	execx.Warn(fmt.Sprintf(i18n.T("正在回退「%s」已应用的改动…"), t.name))
	failed := 0
	for i := len(t.undos) - 1; i >= 0; i-- {
		u := t.undos[i]
		if err := u.fn(); err != nil {
			failed++
			execx.Error(fmt.Sprintf(i18n.T("  回退失败: %s (%v)"), u.desc, err))
		} else {
			execx.Info(i18n.T("  已回退: ") + u.desc)
		}
	}
	t.undos = nil
	t.runCleanups()
	if failed > 0 {
		execx.Error(fmt.Sprintf(i18n.T("回退完成，但有 %d 项失败，请手动检查。"), failed))
	} else {
		execx.Ok(i18n.T("已回退到操作前状态。"))
	}
}

func (t *Transaction) runCleanups() {
	for _, fn := range t.cleanups {
		fn()
	}
	t.cleanups = nil
}

func copyFile(src, dst string) error {
	st, err := os.Stat(src)
	if err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, st.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		case d.IsDir():
			return os.MkdirAll(target, info.Mode().Perm())
		default:
			return copyFile(path, target)
		}
	})
}
