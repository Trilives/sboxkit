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
	"github.com/Trilives/sboxkit/internal/updater"
	"github.com/Trilives/sboxkit/internal/version"
)

func runUpdate(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Fprintln(stdout, "Usage: sboxkit update [--root DIR] [--force] [--proxy URL] [--core] [--sync-service] [--self --channel stable|preview] [--check]")
		return 0
	}
	root, rest := parseRoot(args)
	if hasFlag(rest, "--self") {
		return runSelfUpdate(root, rest, stdout, stderr)
	}
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

func runSelfUpdate(root string, rest []string, stdout io.Writer, stderr io.Writer) int {
	channel := updater.Channel(valueFlag(rest, "--channel", string(updater.ChannelStable)))
	if channel != updater.ChannelStable && channel != updater.ChannelPreview {
		return fail(stderr, "unknown update channel: %s", channel)
	}
	updatePaths := updater.DefaultPaths()
	if root != "" && root != paths.DefaultRoot() {
		updatePaths = updater.PathsForRoot(root)
	}
	manager := updater.New(updatePaths, nil, nil, nil)
	if hasFlag(rest, "--check") {
		result, err := manager.CheckChannel(context.Background(), version.Version, channel)
		if err != nil {
			return fail(stderr, "check self update: %v", err)
		}
		if result.Available {
			fmt.Fprintf(stdout, "sboxkit %s available on %s\n", result.LatestVersion, channel)
			return 0
		}
		fmt.Fprintf(stdout, "sboxkit is up to date on %s (%s)\n", channel, result.CurrentVersion)
		return 0
	}
	result, err := manager.ApplyChannel(context.Background(), version.Version, channel)
	if err != nil {
		if result.RolledBack {
			return fail(stderr, "self update failed and rolled back: %v", err)
		}
		return fail(stderr, "self update failed: %v", err)
	}
	if result.Version == version.Version {
		fmt.Fprintf(stdout, "sboxkit is already up to date on %s (%s)\n", channel, result.Version)
		return 0
	}
	fmt.Fprintf(stdout, "sboxkit updated to %s on %s\n", result.Version, channel)
	return 0
}
