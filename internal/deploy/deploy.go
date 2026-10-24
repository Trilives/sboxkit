// Package deploy 把一次状态目录改动与服务同步合成一个事务：先快照受影响的状态路径
// （订阅原文 / 生成配置 / active 指针等），写入新状态，服务已安装时再同步运行时并
// 重启。任一步失败都还原全部状态路径；同步已改动运行时（前置校验之后的失败）时，
// 再按还原后的状态重新同步，使运行时文件与服务回到改动前的组合。
package deploy

import (
	"errors"
	"fmt"

	"github.com/Trilives/sboxkit/internal/i18n"
	"github.com/Trilives/sboxkit/internal/paths"
	"github.com/Trilives/sboxkit/internal/sysd"
	"github.com/Trilives/sboxkit/internal/txn"
)

// Change 一次状态目录改动。Commit 可在 t 上登记额外的回退动作（如还原内核文件），
// 这些动作先于运行时重新同步执行。
type Change struct {
	Name     string
	Snapshot []string
	Commit   func(t *txn.Transaction) error
	Sync     bool
}

// 可在测试中替换。
var (
	serviceInstalled = func() bool { return sysd.IsInstalled(sysd.DefaultName) }
	syncService      = func(p paths.Paths) error { return sysd.SyncAndRestart(p, sysd.DefaultName) }
)

// Apply 执行 c；失败时回退并返回原错误。回退后重新同步也失败时一并返回，
// 调用方据此知道服务可能没有回到原组合。
func Apply(p paths.Paths, c Change) error {
	t := txn.New(c.Name)
	runtimeChanged := false
	var resyncErr error
	// 最先登记 → 最后执行：状态路径全部还原后，再把运行时对齐回退后的状态。
	t.AddUndo(i18n.T("同步服务到回退后的配置"), func() error {
		if !runtimeChanged {
			return nil
		}
		resyncErr = syncService(p)
		return resyncErr
	})
	if err := apply(p, t, c, &runtimeChanged); err != nil {
		t.Rollback()
		if resyncErr != nil {
			return errors.Join(err, fmt.Errorf(i18n.T("回退后重新同步服务失败，服务可能未恢复到改动前的组合：%w"), resyncErr))
		}
		return err
	}
	t.Commit()
	return nil
}

func apply(p paths.Paths, t *txn.Transaction, c Change, runtimeChanged *bool) error {
	for _, path := range c.Snapshot {
		if err := t.Snapshot(path); err != nil {
			return err
		}
	}
	if c.Commit != nil {
		if err := c.Commit(t); err != nil {
			return err
		}
	}
	if !c.Sync || !serviceInstalled() {
		return nil
	}
	err := syncService(p)
	var pre *sysd.PrecheckError
	*runtimeChanged = err != nil && !errors.As(err, &pre)
	return err
}
