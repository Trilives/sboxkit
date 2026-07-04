package tui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/Trilives/sboxkit/internal/errs"
	"github.com/Trilives/sboxkit/internal/execx"
	"github.com/Trilives/sboxkit/internal/i18n"
)

// 非 TTY（管道/重定向/测试）回退：编号列表 + 文本输入。
// 回车 = esc 的等价（保存返回）；r = ^R 的等价（回退返回）。

func selectPlain(title string, options []string, opts SelectOpts) (int, error) {
	execx.Header(title)
	for i, opt := range options {
		fmt.Printf("  %d) %s\n", i+1, opt)
	}
	fmt.Printf(i18n.T("  回车) %s    r) %s\n"), opts.SaveLabel, opts.BackLabel)
	for {
		raw, err := readPlainLine(i18n.T("请选择: "))
		if err != nil {
			return 0, err
		}
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return 0, errs.ErrSaveExit
		}
		if strings.EqualFold(raw, "r") {
			return 0, errs.ErrCancelled
		}
		if n, err := strconv.Atoi(raw); err == nil && n >= 1 && n <= len(options) {
			return n - 1, nil
		}
		execx.Warn(i18n.T("无效选择，请重输。"))
	}
}

func multiSelectPlain(title string, options []string, defaultOn []int) ([]int, error) {
	chosen := map[int]bool{}
	for _, i := range defaultOn {
		chosen[i] = true
	}
	for {
		execx.Header(title)
		for i, opt := range options {
			mark := " "
			if chosen[i] {
				mark = "x"
			}
			fmt.Printf("  [%s] %d) %s\n", mark, i+1, opt)
		}
		fmt.Println(i18n.T("  输入编号(逗号分隔)切换勾选，回车确认，q 取消"))
		raw, err := readPlainLine(i18n.T("操作: "))
		if err != nil {
			return nil, err
		}
		raw = strings.ToLower(strings.TrimSpace(raw))
		if raw == "" {
			var picked []int
			for i, on := range chosen {
				if on {
					picked = append(picked, i)
				}
			}
			sort.Ints(picked)
			return picked, nil
		}
		if raw == "q" {
			return nil, errs.ErrCancelled
		}
		for _, tok := range strings.Split(strings.ReplaceAll(raw, " ", ""), ",") {
			if n, err := strconv.Atoi(tok); err == nil && n >= 1 && n <= len(options) {
				chosen[n-1] = !chosen[n-1]
			}
		}
	}
}
