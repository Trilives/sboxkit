package tui

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/mattn/go-runewidth"
	"golang.org/x/term"
)

var (
	ErrCancelled = errors.New("cancelled")
	ErrSaveExit  = fmt.Errorf("save and exit: %w", ErrCancelled)
)

const (
	ansiReset = "\033[0m"
	ansiDim   = "\033[2m"
	ansiBold  = "\033[1m"
	ansiCyan  = "\033[36m"
)

var circled = []rune("①②③④⑤⑥⑦⑧⑨⑩⑪⑫⑬⑭⑮⑯⑰⑱⑲⑳")

func num(i int) string {
	if i < len(circled) {
		return string(circled[i])
	}
	return strconv.Itoa(i + 1)
}

var useColor = term.IsTerminal(int(os.Stdout.Fd())) && os.Getenv("NO_COLOR") == ""

func UseTUI() bool {
	return useColor && term.IsTerminal(int(os.Stdin.Fd()))
}

func dispWidth(s string) int {
	return runewidth.StringWidth(s)
}

func truncate(s string, maxW int) string {
	if maxW <= 0 {
		return ""
	}
	if dispWidth(s) <= maxW {
		return s
	}
	var out strings.Builder
	width, limit := 0, maxW-1
	for _, ch := range s {
		cw := runewidth.RuneWidth(ch)
		if width+cw > limit {
			break
		}
		out.WriteRune(ch)
		width += cw
	}
	return out.String() + "…"
}

var ansiRe = regexp.MustCompile(`\033\[[0-9;?]*[A-Za-z]`)

func stripAnsi(s string) string {
	return ansiRe.ReplaceAllString(s, "")
}

func rowPad(s string, w int) string {
	pad := w - dispWidth(stripAnsi(s))
	if pad < 0 {
		pad = 0
	}
	return s + strings.Repeat(" ", pad)
}

func dim(s string) string {
	if !useColor {
		return s
	}
	return ansiDim + s + ansiReset
}

func maxBoxWidth(termCols int) int {
	if termCols <= 0 {
		termCols = 80
	}
	return max(20, termCols-2)
}

func maxVisibleRows(termLines int) int {
	if termLines <= 0 {
		termLines = 24
	}
	return max(5, termLines-8)
}

func scrollTop(n int, idx int, visible int) int {
	if n <= visible {
		return 0
	}
	return max(0, min(idx-visible/2, n-visible))
}

func maxOf(ns []int) int {
	m := 0
	for _, n := range ns {
		if n > m {
			m = n
		}
	}
	return m
}

var stdinReader = bufio.NewReader(os.Stdin)

func readPlainLine(prompt string) (string, error) {
	fmt.Print(prompt)
	line, err := stdinReader.ReadString('\n')
	if err != nil && line == "" {
		fmt.Println()
		return "", ErrCancelled
	}
	return strings.TrimRight(line, "\r\n"), nil
}
