package app

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/Trilives/sboxkit/internal/paths"
	"github.com/Trilives/sboxkit/internal/system"
)

func runService(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprintln(stdout, "Usage: sboxkit service [install|sync|start|stop|remove|status] [--root DIR] [--no-start]")
		return 0
	}
	root, rest := parseRoot(args[1:])
	svc := system.NewService(paths.FromRoot(root), newSudoRunner())
	ctx := context.Background()
	var err error
	if serviceCommandStartsOrRestarts(args[0], rest) {
		printServiceTrafficWarning(stdout)
	}
	switch args[0] {
	case "install":
		err = svc.Install(ctx, !hasFlag(rest, "--no-start"))
	case "sync":
		err = svc.SyncAndRestart(ctx)
	case "start":
		err = svc.Start(ctx)
	case "stop":
		err = svc.Stop(ctx)
	case "remove":
		err = svc.Remove(ctx, true)
	case "status":
		err = svc.Status(ctx)
	default:
		return fail(stderr, "unknown service command: %s", args[0])
	}
	if err != nil {
		return fail(stderr, "service %s: %v", args[0], err)
	}
	return 0
}

func serviceCommandStartsOrRestarts(cmd string, rest []string) bool {
	switch cmd {
	case "install":
		return !hasFlag(rest, "--no-start")
	case "sync", "start":
		return true
	default:
		return false
	}
}

func serviceTrafficWarning() string {
	return "警告：如果 TUN 或路由变更影响当前会话，启动或重启 sboxkit 可能会中断当前 SSH 连接。"
}

func printServiceTrafficWarning(stdout io.Writer) {
	fmt.Fprintln(stdout, serviceTrafficWarning())
}

func runTimer(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprintln(stdout, "Usage: sboxkit timer [install|remove] [--binary PATH]")
		return 0
	}
	root, rest := parseRoot(args[1:])
	p := paths.FromRoot(root)
	binary := valueFlag(rest, "--binary", "/usr/bin/sboxkit")
	runner := newSudoRunner()
	var err error
	if args[0] == "install" {
		err = system.InstallUpdateTimer(context.Background(), runner, p.StateDir, binary, "", "")
	} else if args[0] == "remove" {
		err = system.RemoveUpdateTimer(context.Background(), runner)
	} else {
		return fail(stderr, "unknown timer command: %s", args[0])
	}
	if err != nil {
		return fail(stderr, "timer %s: %v", args[0], err)
	}
	return 0
}

func runResilience(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprintln(stdout, "Usage: sboxkit resilience [install|remove]")
		return 0
	}
	root, _ := parseRoot(args[1:])
	p := paths.FromRoot(root)
	runner := newSudoRunner()
	var err error
	if args[0] == "install" {
		err = system.InstallResilience(context.Background(), runner, p.StateDir, "2min", 20, "singbox")
	} else if args[0] == "remove" {
		err = system.RemoveResilience(context.Background(), runner)
	} else {
		return fail(stderr, "unknown resilience command: %s", args[0])
	}
	if err != nil {
		return fail(stderr, "resilience %s: %v", args[0], err)
	}
	return 0
}

func runUninstall(args []string, stdout io.Writer, stderr io.Writer) int {
	root, rest := parseRoot(args)
	p := paths.FromRoot(root)
	runner := newSudoRunner()
	var failed bool
	if !hasFlag(rest, "--keep-system") {
		if err := system.RemoveResilience(context.Background(), runner); err != nil {
			fmt.Fprintf(stderr, "remove resilience: %v\n", err)
			failed = true
		}
		if err := system.RemoveUpdateTimer(context.Background(), runner); err != nil {
			fmt.Fprintf(stderr, "remove update timer: %v\n", err)
			failed = true
		}
		if err := system.NewService(p, runner).Remove(context.Background(), true); err != nil {
			fmt.Fprintf(stderr, "remove service/runtime: %v\n", err)
			failed = true
		}
	}
	if hasFlag(rest, "--purge-state") {
		if err := os.RemoveAll(p.StateDir); err != nil {
			return fail(stderr, "remove state: %v", err)
		}
		fmt.Fprintf(stdout, "removed user state: %s\n", p.StateDir)
	}
	if failed {
		printPackageRemovalHint(stdout)
		return 1
	}
	fmt.Fprintln(stdout, "sboxkit-managed service, timers, resilience hooks, and runtime files removed.")
	printPackageRemovalHint(stdout)
	return 0
}

func printPackageRemovalHint(stdout io.Writer) {
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Debian 包本身由 apt 管理。")
	fmt.Fprintln(stdout, "移除已安装的 sboxkit 二进制和包元数据：")
	fmt.Fprintln(stdout, "  sudo apt remove sboxkit")
	fmt.Fprintln(stdout, "如需连同配置文件一起移除：")
	fmt.Fprintln(stdout, "  sudo apt purge sboxkit")
}
