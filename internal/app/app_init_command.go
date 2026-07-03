package app

import (
	"bufio"
	"context"
	"fmt"
	"io"

	"github.com/Trilives/sboxkit/internal/config"
	"github.com/Trilives/sboxkit/internal/download"
	"github.com/Trilives/sboxkit/internal/paths"
	"github.com/Trilives/sboxkit/internal/proxyenv"
)

func runInit(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Fprintln(stdout, "Usage: sboxkit init [--root DIR] [--download] [--proxy URL] [--no-tun] [--write-proxy-env|--no-write-proxy-env] [--proxy-env-file PATH]")
		return 0
	}
	root, rest := parseRoot(args)
	return initState(root, rest, stdout, stderr)
}

func runInitWizard(reader *bufio.Reader, stdout io.Writer, stderr io.Writer) int {
	enableTun := askYesNo(reader, stdout, "Enable TUN mode? [Y/n]: ", true)
	args := []string{}
	if !enableTun {
		args = append(args, "--no-tun")
		if askYesNo(reader, stdout, "TUN is disabled. Write shell proxy variables to ~/.bashrc? [y/N]: ", false) {
			args = append(args, "--write-proxy-env")
		} else {
			args = append(args, "--no-write-proxy-env")
		}
	}
	return initState("", args, stdout, stderr)
}

func initState(root string, rest []string, stdout io.Writer, stderr io.Writer) int {
	p := paths.FromRoot(root)
	cfg, err := config.Load(p.CustomizeFile)
	if err != nil {
		return fail(stderr, "load config: %v", err)
	}
	if hasFlag(rest, "--no-tun") {
		cfg.EnableTun = false
	}
	if err := p.EnsureStateDirs(); err != nil {
		return fail(stderr, "create state directories: %v", err)
	}
	if err := config.Save(p.CustomizeFile, cfg); err != nil {
		return fail(stderr, "save config: %v", err)
	}
	if hasFlag(rest, "--download") {
		if proxy := valueFlag(rest, "--proxy", ""); proxy != "" {
			cfg.DownloadProxy = proxy
		}
		if err := download.DownloadRuntimeAssets(context.Background(), p, cfg, false); err != nil {
			return fail(stderr, "download assets: %v", err)
		}
	}
	if !cfg.EnableTun && hasFlag(rest, "--write-proxy-env") {
		target := valueFlag(rest, "--proxy-env-file", "")
		if err := proxyenv.Write(target); err != nil {
			return fail(stderr, "write proxy environment: %v", err)
		}
		if target == "" {
			target = proxyenv.TargetBashrc()
		}
		fmt.Fprintf(stdout, "proxy environment written to %s\n", target)
	}
	fmt.Fprintf(stdout, "initialized sboxkit state at %s\n", p.StateDir)
	return 0
}
