package app

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Trilives/sboxkit/internal/paths"
)

func parseRoot(args []string) (string, []string) {
	root := ""
	rest := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		if args[i] == "--root" && i+1 < len(args) {
			root = args[i+1]
			i++
			continue
		}
		if strings.HasPrefix(args[i], "--root=") {
			root = strings.TrimPrefix(args[i], "--root=")
			continue
		}
		rest = append(rest, args[i])
	}
	if root == "" {
		root = paths.DefaultRoot()
	} else if abs, err := filepath.Abs(root); err == nil {
		root = abs
	}
	return root, rest
}

func hasFlag(args []string, flag string) bool {
	for _, arg := range args {
		if arg == flag {
			return true
		}
	}
	return false
}

func valueFlag(args []string, flag string, fallback string) string {
	prefix := flag + "="
	for i, arg := range args {
		if strings.HasPrefix(arg, prefix) {
			return strings.TrimPrefix(arg, prefix)
		}
		if arg == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return fallback
}

func hasExplicitValue(args []string, flag string) bool {
	prefix := flag + "="
	for i, arg := range args {
		if strings.HasPrefix(arg, prefix) {
			return true
		}
		if arg == flag && i+1 < len(args) {
			return true
		}
	}
	return false
}

func askYesNo(reader *bufio.Reader, stdout io.Writer, prompt string, fallback bool) bool {
	fmt.Fprint(stdout, prompt)
	line, err := reader.ReadString('\n')
	if err != nil {
		return fallback
	}
	switch strings.TrimSpace(strings.ToLower(line)) {
	case "y", "yes", "true", "是", "对", "好", "1", "t", "ok":
		return true
	case "n", "no", "false", "否", "不", "错", "0", "f", "off":
		return false
	default:
		return fallback
	}
}

func fail(stderr io.Writer, format string, args ...any) int {
	fmt.Fprintf(stderr, format+"\n", args...)
	return 1
}

func jsonMarshalIndent(value any) ([]byte, error) {
	return json.MarshalIndent(value, "", "  ")
}

func isTerminal(file *os.File) bool {
	info, err := file.Stat()
	return err == nil && (info.Mode()&os.ModeCharDevice) != 0
}
