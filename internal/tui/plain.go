package tui

import (
	"fmt"
	"strconv"
	"strings"
)

func selectPlain(title string, options []string, opts SelectOpts) (int, error) {
	fmt.Println()
	fmt.Println(title)
	for i, opt := range options {
		fmt.Printf("  %d) %s\n", i+1, opt)
	}
	fmt.Printf("  Enter) %s    r) %s\n", opts.SaveLabel, opts.BackLabel)
	for {
		raw, err := readPlainLine("select: ")
		if err != nil {
			return 0, err
		}
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return 0, ErrSaveExit
		}
		if strings.EqualFold(raw, "r") || strings.EqualFold(raw, "q") {
			return 0, ErrCancelled
		}
		if n, err := strconv.Atoi(raw); err == nil && n >= 1 && n <= len(options) {
			return n - 1, nil
		}
		fmt.Println("Invalid selection.")
	}
}
