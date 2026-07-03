package transaction

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type UndoFunc func() error

type undoStep struct {
	desc string
	fn   UndoFunc
}

type Transaction struct {
	name      string
	undos     []undoStep
	committed bool
}

func New(name string) *Transaction {
	return &Transaction{name: name}
}

func (tx *Transaction) AddUndo(desc string, fn UndoFunc) {
	if tx.committed {
		return
	}
	tx.undos = append(tx.undos, undoStep{desc: desc, fn: fn})
}

func (tx *Transaction) BackupFile(path string) error {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		tx.AddUndo("remove new file "+path, func() error {
			if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return os.Remove(path)
		})
		return nil
	}
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("backup file %s: path is a directory", path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	mode := info.Mode().Perm()
	tx.AddUndo("restore file "+path, func() error {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, data, mode); err != nil {
			return err
		}
		return os.Chmod(path, mode)
	})
	return nil
}

func (tx *Transaction) TrackPath(path string) {
	if _, err := os.Stat(path); err == nil {
		return
	}
	tx.AddUndo("remove new path "+path, func() error {
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return os.RemoveAll(path)
	})
}

func (tx *Transaction) Commit() {
	tx.committed = true
	tx.undos = nil
}

func (tx *Transaction) Rollback() error {
	if tx.committed {
		return nil
	}

	var rollbackErr error
	for i := len(tx.undos) - 1; i >= 0; i-- {
		step := tx.undos[i]
		if err := step.fn(); err != nil {
			rollbackErr = errors.Join(rollbackErr, fmt.Errorf("%s: %w", step.desc, err))
		}
	}
	tx.undos = nil
	return rollbackErr
}
