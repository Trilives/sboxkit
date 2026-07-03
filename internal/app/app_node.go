package app

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/Trilives/sboxkit/internal/node"
	"github.com/Trilives/sboxkit/internal/paths"
)

func runNode(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprintln(stdout, "Usage: sboxkit node [list|switch] [--api URL] [--secret TOKEN] [--group Proxy --name NODE] [--reorder] [--sync-service]")
		return 0
	}
	root, rest := parseRoot(args[1:])
	client := node.NewClient(valueFlag(rest, "--api", "http://127.0.0.1:9090"), valueFlag(rest, "--secret", ""))
	switch args[0] {
	case "list":
		groups, err := client.Groups(context.Background())
		if err != nil {
			return fail(stderr, "list nodes: %v", err)
		}
		for _, group := range groups {
			fmt.Fprintf(stdout, "%s\tcurrent=%s\tchoices=%s\n", group.Name, group.Now, strings.Join(group.All, ","))
		}
	case "switch":
		return runNodeSwitch(rest, root, client, stdout, stderr)
	default:
		return fail(stderr, "unknown node command: %s", args[0])
	}
	return 0
}

func runNodeSwitch(rest []string, root string, client nodeSwitcher, stdout io.Writer, stderr io.Writer) int {
	group := valueFlag(rest, "--group", "Proxy")
	name := valueFlag(rest, "--name", "")
	if name == "" {
		return fail(stderr, "--name is required")
	}
	if err := client.Switch(context.Background(), group, name); err != nil {
		return fail(stderr, "switch node: %v", err)
	}
	fmt.Fprintf(stdout, "%s switched to %s\n", group, name)
	if !hasFlag(rest, "--reorder") {
		return 0
	}
	p := paths.FromRoot(root)
	if err := node.ReorderSelectorConfig(p.ConfigFile, group, name); err != nil {
		return fail(stderr, "reorder node: %v", err)
	}
	fmt.Fprintf(stdout, "%s order updated; %s is now first\n", group, name)
	if hasFlag(rest, "--sync-service") {
		return runService(rootArgs(root, "sync"), stdout, stderr)
	}
	return 0
}

type nodeSwitcher interface {
	Switch(ctx context.Context, group string, node string) error
}

func rootArgs(root string, command string) []string {
	args := []string{command}
	if root != "" {
		args = append(args, "--root", root)
	}
	return args
}
