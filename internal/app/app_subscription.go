package app

import (
	"fmt"
	"io"

	"github.com/Trilives/sboxkit/internal/config"
	"github.com/Trilives/sboxkit/internal/paths"
	"github.com/Trilives/sboxkit/internal/subscription"
)

func runSub(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprintln(stdout, "Usage: sboxkit sub [add|list|switch|remove|refresh|rebuild] [--url URL|--file PATH] [--proxy URL] [options]")
		return 0
	}
	root, rest := parseRoot(args[1:])
	p := paths.FromRoot(root)
	cfg, err := config.Load(p.CustomizeFile)
	if err != nil {
		return fail(stderr, "load config: %v", err)
	}
	manager := subscription.NewManager(p, cfg)
	switch args[0] {
	case "add":
		return runSubAdd(manager, p, cfg, rest, stdout, stderr)
	case "list":
		return runSubList(manager, stdout, stderr)
	case "switch":
		return runSubSwitch(manager, rest, stdout, stderr)
	case "remove":
		return runSubRemove(manager, rest, stdout, stderr)
	case "refresh":
		return runSubRefresh(manager, p, cfg, rest, stdout, stderr)
	case "rebuild":
		return runSubRebuild(manager, rest, stdout, stderr)
	default:
		return fail(stderr, "unknown sub command: %s", args[0])
	}
}

func runSubAdd(manager *subscription.Manager, p paths.Paths, cfg config.Config, rest []string, stdout io.Writer, stderr io.Writer) int {
	name := valueFlag(rest, "--name", "sub")
	rawURL := valueFlag(rest, "--url", "")
	filePath := valueFlag(rest, "--file", "")
	source := subscription.SourceKind(valueFlag(rest, "--source", string(subscription.SourceClash)))
	if filePath != "" && !hasExplicitValue(rest, "--source") {
		source = ""
	}
	if proxy := valueFlag(rest, "--proxy", ""); proxy != "" {
		cfg.DownloadProxy = proxy
		manager = subscription.NewManager(p, cfg)
	}
	if rawURL == "" && filePath == "" {
		return fail(stderr, "--url or --file is required")
	}
	if rawURL != "" && filePath != "" {
		return fail(stderr, "--url and --file cannot be used together")
	}
	if filePath != "" {
		sub, err := manager.AddFile(name, filePath, source, !hasFlag(rest, "--passthrough"), !hasFlag(rest, "--no-active"))
		if err != nil {
			return fail(stderr, "add config file: %v", err)
		}
		fmt.Fprintf(stdout, "config file %s ready: %d nodes\n", sub.Name, sub.LastNodeCount)
		return 0
	}
	sub, err := manager.Add(name, rawURL, source, !hasFlag(rest, "--passthrough"), !hasFlag(rest, "--no-active"))
	if err != nil {
		return fail(stderr, "add subscription: %v", err)
	}
	fmt.Fprintf(stdout, "subscription %s ready: %d nodes\n", sub.Name, sub.LastNodeCount)
	return 0
}

func runSubList(manager *subscription.Manager, stdout io.Writer, stderr io.Writer) int {
	subs, err := manager.List()
	if err != nil {
		return fail(stderr, "list subscriptions: %v", err)
	}
	for _, sub := range subs {
		fmt.Fprintf(stdout, "%s\t%s\t%d\n", sub.Name, sub.SourceType, sub.LastNodeCount)
	}
	return 0
}

func runSubSwitch(manager *subscription.Manager, rest []string, stdout io.Writer, stderr io.Writer) int {
	name := valueFlag(rest, "--name", "")
	if name == "" {
		return fail(stderr, "--name is required")
	}
	sub, err := manager.Rebuild(name)
	if err != nil {
		return fail(stderr, "rebuild subscription: %v", err)
	}
	if err := manager.Switch(name); err != nil {
		return fail(stderr, "switch subscription: %v", err)
	}
	fmt.Fprintf(stdout, "active subscription rebuilt: %s (%d nodes)\n", name, sub.LastNodeCount)
	return 0
}

func runSubRemove(manager *subscription.Manager, rest []string, stdout io.Writer, stderr io.Writer) int {
	name := valueFlag(rest, "--name", "")
	if name == "" {
		return fail(stderr, "--name is required")
	}
	if err := manager.Remove(name); err != nil {
		return fail(stderr, "remove subscription: %v", err)
	}
	fmt.Fprintf(stdout, "removed subscription: %s\n", name)
	return 0
}

func runSubRefresh(manager *subscription.Manager, p paths.Paths, cfg config.Config, rest []string, stdout io.Writer, stderr io.Writer) int {
	name := valueFlag(rest, "--name", "")
	if name == "" {
		return fail(stderr, "--name is required")
	}
	if proxy := valueFlag(rest, "--proxy", ""); proxy != "" {
		cfg.DownloadProxy = proxy
		manager = subscription.NewManager(p, cfg)
	}
	sub, err := manager.Refresh(name)
	if err != nil {
		return fail(stderr, "refresh subscription: %v", err)
	}
	fmt.Fprintf(stdout, "subscription %s refreshed: %d nodes\n", sub.Name, sub.LastNodeCount)
	return 0
}

func runSubRebuild(manager *subscription.Manager, rest []string, stdout io.Writer, stderr io.Writer) int {
	name := valueFlag(rest, "--name", "")
	if name == "" {
		return fail(stderr, "--name is required")
	}
	sub, err := manager.Rebuild(name)
	if err != nil {
		return fail(stderr, "rebuild subscription: %v", err)
	}
	fmt.Fprintf(stdout, "subscription %s rebuilt: %d nodes\n", sub.Name, sub.LastNodeCount)
	return 0
}
