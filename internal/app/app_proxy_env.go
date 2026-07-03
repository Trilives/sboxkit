package app

import (
	"fmt"
	"io"

	"github.com/Trilives/sboxkit/internal/proxyenv"
)

func runProxyEnv(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprintln(stdout, "Usage: sboxkit proxy-env [write|remove] [--file PATH]")
		return 0
	}
	_, rest := parseRoot(args[1:])
	target := valueFlag(rest, "--file", "")
	if target == "" {
		target = valueFlag(rest, "--proxy-env-file", "")
	}
	switch args[0] {
	case "write":
		if err := proxyenv.Write(target); err != nil {
			return fail(stderr, "write proxy environment: %v", err)
		}
		if target == "" {
			target = proxyenv.TargetBashrc()
		}
		fmt.Fprintf(stdout, "proxy environment written to %s\n", target)
	case "remove":
		if err := proxyenv.Remove(target); err != nil {
			return fail(stderr, "remove proxy environment: %v", err)
		}
		if target == "" {
			target = proxyenv.TargetBashrc()
		}
		fmt.Fprintf(stdout, "proxy environment removed from %s\n", target)
	default:
		return fail(stderr, "unknown proxy-env command: %s", args[0])
	}
	return 0
}
