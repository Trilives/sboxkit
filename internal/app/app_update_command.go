package app

import (
	"context"
	"fmt"
	"io"

	"github.com/Trilives/sboxkit/internal/config"
	"github.com/Trilives/sboxkit/internal/download"
	"github.com/Trilives/sboxkit/internal/paths"
	"github.com/Trilives/sboxkit/internal/subscription"
	"github.com/Trilives/sboxkit/internal/system"
)

func runUpdate(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Fprintln(stdout, "Usage: sboxkit update [--root DIR] [--force] [--proxy URL] [--core] [--sync-service]")
		return 0
	}
	root, rest := parseRoot(args)
	p := paths.FromRoot(root)
	cfg, err := config.Load(p.CustomizeFile)
	if err != nil {
		return fail(stderr, "load config: %v", err)
	}
	force := hasFlag(rest, "--force")
	if proxy := valueFlag(rest, "--proxy", ""); proxy != "" {
		cfg.DownloadProxy = proxy
	}
	if hasFlag(rest, "--core") {
		if err := download.DownloadAll(context.Background(), p, cfg, force); err != nil {
			return fail(stderr, "update assets: %v", err)
		}
	} else if err := download.DownloadRuntimeAssets(context.Background(), p, cfg, force); err != nil {
		return fail(stderr, "update assets: %v", err)
	}
	manager := subscription.NewManager(p, cfg)
	if active, _ := manager.Active(); active != nil {
		if _, err := manager.Rebuild(active.Name); err != nil {
			return fail(stderr, "rebuild active subscription: %v", err)
		}
		fmt.Fprintf(stdout, "active subscription rebuilt: %s\n", active.Name)
	}
	if hasFlag(rest, "--sync-service") {
		printServiceTrafficWarning(stdout)
		if err := system.NewService(p, system.OSRunner{UseSudo: true}).SyncAndRestart(context.Background()); err != nil {
			return fail(stderr, "sync service: %v", err)
		}
		fmt.Fprintln(stdout, "service synced and restarted")
	}
	fmt.Fprintln(stdout, "assets updated")
	return 0
}
