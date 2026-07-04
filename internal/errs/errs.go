// Package errs 定义跨层导航/中止哨兵错误（对应 Python 版 errors.py）。
package errs

import (
	"errors"
	"fmt"
)

// ErrCancelled 用户回退（^R / Ctrl-C / EOF）：上层据此回滚事务并返回上一层。
var ErrCancelled = errors.New("用户已取消")

// ErrSaveExit 用户保存返回（ESC）。包装 ErrCancelled，使
// errors.Is(ErrSaveExit, ErrCancelled) 成立（对应 Python 版 SaveExit ⊂ Cancelled）；
// 需要区分「保存返回」与「回退」时，先判 errors.Is(err, ErrSaveExit)。
var ErrSaveExit = fmt.Errorf("保存并返回: %w", ErrCancelled)
