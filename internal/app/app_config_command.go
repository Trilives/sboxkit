package app

import (
	"fmt"
	"io"

	"github.com/Trilives/sboxkit/internal/config"
	"github.com/Trilives/sboxkit/internal/paths"
)

func runConfig(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprintln(stdout, "Usage: sboxkit config [show|set] [--root DIR] [--key KEY --value VALUE]")
		return 0
	}
	root, rest := parseRoot(args[1:])
	p := paths.FromRoot(root)
	cfg, err := config.Load(p.CustomizeFile)
	if err != nil {
		return fail(stderr, "load config: %v", err)
	}
	switch args[0] {
	case "show":
		data, _ := jsonMarshalIndent(cfg)
		fmt.Fprintln(stdout, string(data))
	case "set":
		key := valueFlag(rest, "--key", "")
		value := valueFlag(rest, "--value", "")
		if key == "" {
			return fail(stderr, "--key is required")
		}
		if err := config.SetField(&cfg, key, value); err != nil {
			return fail(stderr, "set config: %v", err)
		}
		if err := config.Save(p.CustomizeFile, cfg); err != nil {
			return fail(stderr, "save config: %v", err)
		}
		fmt.Fprintf(stdout, "updated %s\n", key)
	default:
		return fail(stderr, "unknown config command: %s", args[0])
	}
	return 0
}
